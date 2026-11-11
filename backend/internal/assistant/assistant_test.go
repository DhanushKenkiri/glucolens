package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"glucolens/internal/dataset"
	"glucolens/internal/query"
)

const realCSV = "../../data/diabetes_dataset.csv"

var (
	engOnce sync.Once
	eng     *query.Engine
	engErr  error
)

func engine(t *testing.T) *query.Engine {
	t.Helper()
	if _, err := os.Stat(realCSV); err != nil {
		t.Skipf("real dataset not available: %v", err)
	}
	engOnce.Do(func() {
		dir, err := os.MkdirTemp("", "glucolens-assistant-test")
		if err != nil {
			engErr = err
			return
		}
		ds, _, err := dataset.Open(realCSV, filepath.Join(dir, "d.json"), false)
		if err != nil {
			engErr = err
			return
		}
		eng = query.New(ds)
	})
	if engErr != nil {
		t.Fatal(engErr)
	}
	return eng
}

// turn is one scripted model reply: either tool calls or text.
type turn struct {
	calls []fakeCall
	text  []string // streamed in pieces
}

type fakeCall struct {
	id, name string
	args     map[string]any
}

// fakeGroq is an OpenAI-compatible Chat Completions server that replays a
// script and records every request body.
type fakeGroq struct {
	mu       sync.Mutex
	script   []turn
	requests []map[string]any
	status   int // when non-zero, every request fails with this status
	// streamErrors makes the first N requests fail the way Groq does when a
	// tool call does not match its schema: an error event inside the stream.
	streamErrors int
}

func (f *fakeGroq) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/chat/completions" {
		http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	n := len(f.requests)
	status := f.status
	streamErr := n <= f.streamErrors
	var t turn
	if n <= len(f.script) {
		t = f.script[n-1]
	} else {
		t = turn{text: []string{"(script exhausted)"}}
	}
	f.mu.Unlock()

	if status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		b, _ := json.Marshal(map[string]any{"error": map[string]any{
			"message": "Rate limit reached for model", "type": "tokens", "code": "rate_limit_exceeded",
		}})
		w.Write(b)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	if streamErr {
		b, _ := json.Marshal(map[string]any{"error": map[string]any{
			"message": "Tool call validation failed: parameters for tool get_distribution did not match schema: errors: [`/filters/0`: expected object, but got array]",
			"type":    "invalid_request_error", "code": "tool_use_failed",
		}})
		fmt.Fprintf(w, "data: %s%s", b, sseEnd)
		return
	}
	chunk := func(delta map[string]any, finish any) {
		b, _ := json.Marshal(map[string]any{
			"id": fmt.Sprintf("chatcmpl-%d", n), "object": "chat.completion.chunk", "created": 1700000000,
			"model":   "fake-model",
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		})
		fmt.Fprintf(w, "data: %s\n\n", b)
		w.(http.Flusher).Flush()
	}
	if len(t.calls) > 0 {
		for i, c := range t.calls {
			args, _ := json.Marshal(c.args)
			half := len(args) / 2
			chunk(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
				"index": i, "id": c.id, "type": "function",
				"function": map[string]any{"name": c.name, "arguments": string(args[:half])},
			}}}, nil)
			chunk(map[string]any{"tool_calls": []any{map[string]any{
				"index": i, "function": map[string]any{"arguments": string(args[half:])},
			}}}, nil)
		}
		chunk(map[string]any{}, "tool_calls")
	} else {
		for i, piece := range t.text {
			d := map[string]any{"content": piece}
			if i == 0 {
				d["role"] = "assistant"
			}
			chunk(d, nil)
		}
		chunk(map[string]any{}, "stop")
	}
	b, _ := json.Marshal(map[string]any{
		"id": fmt.Sprintf("chatcmpl-%d", n), "object": "chat.completion.chunk", "created": 1700000000,
		"model": "fake-model", "choices": []any{},
		"usage": map[string]any{"prompt_tokens": 900, "completion_tokens": 40, "total_tokens": 940},
	})
	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", b)
}

func newService(t *testing.T, fake *fakeGroq) *Service {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	svc, err := New(context.Background(), Config{
		APIKey: "gsk_test", Model: "openai/gpt-oss-120b", BaseURL: srv.URL, ReasoningEffort: "low",
	}, engine(t))
	if err != nil {
		t.Fatal(err)
	}
	if !svc.Status().Ready {
		t.Fatalf("service not ready: %+v", svc.Status())
	}
	return svc
}

func collect(t *testing.T, svc *Service, req ChatRequest) []Event {
	t.Helper()
	if err := svc.Prepare(&req); err != nil {
		t.Fatal(err)
	}
	var events []Event
	err := svc.Chat(context.Background(), req, func(e Event) error {
		// Round-trip through JSON exactly as the HTTP layer does.
		b, err := json.Marshal(e.Data)
		if err != nil {
			return fmt.Errorf("event %s is not JSON-encodable: %w", e.Type, err)
		}
		var data any
		json.Unmarshal(b, &data)
		events = append(events, Event{Type: e.Type, Data: data})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func find(events []Event, typ string) []map[string]any {
	var out []map[string]any
	for _, e := range events {
		if e.Type == typ {
			out = append(out, e.Data.(map[string]any))
		}
	}
	return out
}

func TestAgentRunsToolsAndStreamsAnswer(t *testing.T) {
	fake := &fakeGroq{script: []turn{
		{calls: []fakeCall{{id: "call_1", name: "compare_groups", args: map[string]any{
			"metric": "hba1c", "group_by": "family_history_diabetes",
		}}}},
		{calls: []fakeCall{{id: "call_2", name: "get_percentile", args: map[string]any{"columns": []string{"hba1c"}}}}},
		{text: []string{"HbA1c is **higher** ", "with a family history. ", "Yours sits near the middle."}},
	}}
	svc := newService(t, fake)
	events := collect(t, svc, ChatRequest{
		SessionID: "test-session-1",
		Message:   "Is HbA1c higher with a family history, and where is mine?",
		Values:    map[string]any{"hba1c": 6.1, "gender": "Female", "age": 52, "bogus": 1},
		UseValues: true,
	})

	var types []string
	for _, e := range events {
		types = append(types, e.Type)
	}
	t.Logf("events: %v", types)
	if types[0] != "session" || types[len(types)-1] != "done" {
		t.Fatalf("stream must start with session and end with done: %v", types)
	}

	calls := find(events, "tool_call")
	results := find(events, "tool_result")
	if len(calls) != 2 || len(results) != 2 {
		t.Fatalf("tool calls=%d results=%d", len(calls), len(results))
	}
	if calls[0]["name"] != "compare_groups" || calls[0]["label"] != "Comparing HbA1c by family history of diabetes" {
		t.Errorf("first call = %v", calls[0])
	}
	if results[0]["ok"] != true {
		t.Fatalf("compare result failed: %v", results[0])
	}
	view := results[0]["view"].(map[string]any)
	req := view["request"].(map[string]any)
	if view["kind"] != "compare" || req["metric"] != "hba1c" || req["groupBy"] != "family_history_diabetes" {
		t.Errorf("compare view = %v", view)
	}
	summary := results[0]["result"].(map[string]any)["summary"].(string)
	if !strings.Contains(summary, "with family history of diabetes") {
		t.Errorf("compare summary = %q", summary)
	}

	// get_percentile read the user's values from session state.
	if results[1]["ok"] != true {
		t.Fatalf("percentile result failed: %v", results[1])
	}
	pv := results[1]["view"].(map[string]any)
	if pv["kind"] != "profile" {
		t.Errorf("percentile view = %v", pv)
	}
	vals := results[1]["result"].(map[string]any)["values"].([]any)
	if len(vals) != 1 || vals[0].(map[string]any)["value"] != "6.1%" {
		t.Errorf("percentile values = %v", vals)
	}

	var streamed strings.Builder
	for _, d := range find(events, "delta") {
		streamed.WriteString(d["text"].(string))
	}
	msgs := find(events, "message")
	want := "HbA1c is **higher** with a family history. Yours sits near the middle."
	if streamed.String() != want || len(msgs) != 1 || msgs[0]["text"] != want {
		t.Errorf("streamed %q, messages %v", streamed.String(), msgs)
	}

	// What Groq would have received.
	if len(fake.requests) != 3 {
		t.Fatalf("model calls = %d, want 3", len(fake.requests))
	}
	first := fake.requests[0]
	if first["model"] != "openai/gpt-oss-120b" || first["stream"] != true || first["reasoning_effort"] != "low" {
		t.Errorf("request basics: model=%v stream=%v reasoning_effort=%v", first["model"], first["stream"], first["reasoning_effort"])
	}
	tools := first["tools"].([]any)
	var names []string
	for _, tl := range tools {
		names = append(names, tl.(map[string]any)["function"].(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "get_distribution,compare_groups,analyze_risk,rank_risk_factors,get_percentile,find_similar_people" {
		t.Errorf("tools = %v", names)
	}
	msgs0 := first["messages"].([]any)
	sys := msgs0[0].(map[string]any)
	if sys["role"] != "system" {
		t.Fatalf("first message role = %v", sys["role"])
	}
	sysText := fmt.Sprint(sys["content"])
	for _, needle := range []string{"GlucoLens", "100000", "- hba1c: 6.1%", "- gender: Female", "diagnosed_diabetes: Diagnosed diabetes"} {
		if !strings.Contains(sysText, needle) {
			t.Errorf("system prompt lacks %q", needle)
		}
	}
	if strings.Contains(sysText, "bogus") {
		t.Errorf("unknown values must not reach the prompt")
	}
	// Groq rejects messages[].name; make sure no message carries one.
	for i, r := range fake.requests {
		for _, m := range r["messages"].([]any) {
			if _, has := m.(map[string]any)["name"]; has {
				t.Errorf("request %d has a message with a name field: %v", i, m)
			}
		}
	}
	// The second request carries the tool result back to the model.
	var sawToolMsg bool
	for _, m := range fake.requests[1]["messages"].([]any) {
		mm := m.(map[string]any)
		if mm["role"] == "tool" && mm["tool_call_id"] == "call_1" {
			sawToolMsg = true
			if !strings.Contains(fmt.Sprint(mm["content"]), "family_history_diabetes") {
				t.Errorf("tool message content = %v", mm["content"])
			}
		}
	}
	if !sawToolMsg {
		t.Errorf("second request has no tool message for call_1")
	}
}

func TestToolErrorsGoBackToTheModel(t *testing.T) {
	fake := &fakeGroq{script: []turn{
		{calls: []fakeCall{{id: "call_a", name: "analyze_risk", args: map[string]any{
			"outcome": "diagnosed_diabetes", "factor": "smoking",
		}}}},
		{calls: []fakeCall{{id: "call_b", name: "analyze_risk", args: map[string]any{
			"outcome": "diagnosed_diabetes", "factor": "smoking_status",
		}}}},
		{calls: []fakeCall{{id: "call_c", name: "find_similar_people", args: map[string]any{}}}},
		{text: []string{"Smoking shows no meaningful difference in this dataset."}},
	}}
	svc := newService(t, fake)
	events := collect(t, svc, ChatRequest{Message: "Does smoking raise diabetes risk?", UseValues: false})

	results := find(events, "tool_result")
	if len(results) != 3 {
		t.Fatalf("results = %d", len(results))
	}
	if results[0]["ok"] != false || !strings.Contains(results[0]["error"].(string), "unknown factor") {
		t.Errorf("bad column should fail with a helpful error: %v", results[0])
	}
	if results[0]["view"] != nil {
		t.Errorf("failed calls must not carry a view")
	}
	if results[1]["ok"] != true || results[1]["view"].(map[string]any)["kind"] != "risk" {
		t.Errorf("second risk call = %v", results[1])
	}
	verdict := results[1]["result"].(map[string]any)["verdict"].(string)
	if !strings.HasPrefix(verdict, "No meaningful difference") {
		t.Errorf("smoking verdict = %q", verdict)
	}
	// No values shared: the similar-people tool explains what is missing.
	if results[2]["ok"] != false || !strings.Contains(results[2]["error"].(string), "My values") {
		t.Errorf("similar without values = %v", results[2])
	}
	if !strings.Contains(fmt.Sprint(fake.requests[0]["messages"].([]any)[0]), "has not shared") {
		t.Errorf("prompt should say no values were shared")
	}
	// The model saw the error text and could correct itself.
	var sawError bool
	for _, m := range fake.requests[1]["messages"].([]any) {
		mm := m.(map[string]any)
		if mm["role"] == "tool" && strings.Contains(fmt.Sprint(mm["content"]), "unknown factor") {
			sawError = true
		}
	}
	if !sawError {
		t.Errorf("the tool error was not sent back to the model")
	}
}

func TestProviderErrorsBecomeFriendlyEvents(t *testing.T) {
	fake := &fakeGroq{status: http.StatusTooManyRequests}
	svc := newService(t, fake)
	events := collect(t, svc, ChatRequest{Message: "hello"})
	errs := find(events, "error")
	if len(errs) != 1 || errs[0]["code"] != "rate_limited" {
		t.Fatalf("errors = %v", errs)
	}
	if events[len(events)-1].Type != "done" {
		t.Errorf("stream should still end with done")
	}
}

func TestSessionsAndValidation(t *testing.T) {
	svc, err := New(context.Background(), Config{Model: "m"}, engine(t))
	if err != nil {
		t.Fatal(err)
	}
	if svc.Status().Ready || !strings.Contains(svc.Status().Reason, "GROQ_API_KEY") {
		t.Errorf("no key should mean not ready: %+v", svc.Status())
	}
	if err := svc.Prepare(&ChatRequest{Message: "hi"}); err != ErrNotReady {
		t.Errorf("prepare without key = %v", err)
	}

	ready := newService(t, &fakeGroq{})
	bad := []ChatRequest{
		{Message: "  "},
		{Message: strings.Repeat("a", maxMessageLength+1)},
		{Message: "hi", SessionID: "bad id with spaces"},
	}
	for i, r := range bad {
		if err := ready.Prepare(&r); err == nil {
			t.Errorf("case %d should be rejected", i)
		}
	}
	r := ChatRequest{Message: "hi"}
	if err := ready.Prepare(&r); err != nil || len(r.SessionID) < 8 {
		t.Errorf("prepare should assign a session id: %v %q", err, r.SessionID)
	}

	// A busy session rejects a second concurrent question.
	lock := ready.sessionLock(r.SessionID)
	lock.Lock()
	err = ready.Chat(context.Background(), r, func(Event) error { return nil })
	lock.Unlock()
	if err != ErrSessionBusy {
		t.Errorf("concurrent chat = %v, want ErrSessionBusy", err)
	}
}

func TestRetriesWhenGroqRejectsAToolCall(t *testing.T) {
	fake := &fakeGroq{streamErrors: 1, script: []turn{
		{}, // consumed by the rejected attempt
		{calls: []fakeCall{{id: "call_1", name: "get_distribution", args: map[string]any{
			"column": "hba1c", "filters": []string{"gender = Female", "age >= 50"},
		}}}},
		{text: []string{"Here is the spread of HbA1c."}},
	}}
	svc := newService(t, fake)
	events := collect(t, svc, ChatRequest{Message: "Show HbA1c for women over 50"})
	if errs := find(events, "error"); len(errs) != 0 {
		t.Fatalf("the rejected tool call should be retried, got errors %v", errs)
	}
	results := find(events, "tool_result")
	if len(results) != 1 || results[0]["ok"] != true {
		t.Fatalf("results = %v", results)
	}
	req := results[0]["view"].(map[string]any)["request"].(map[string]any)
	filters := req["filters"].([]any)
	if len(filters) != 2 || filters[0].(map[string]any)["column"] != "gender" || filters[1].(map[string]any)["min"] != 50.0 {
		t.Errorf("parsed filters = %v", filters)
	}
	if got := results[0]["result"].(map[string]any)["group"]; got != "Gender: Female · Age ≥ 50 years" {
		t.Errorf("cohort = %v", got)
	}
	// The retry carried a note explaining the rejection.
	var sawNote bool
	for _, m := range fake.requests[1]["messages"].([]any) {
		if strings.Contains(fmt.Sprint(m.(map[string]any)["content"]), "Automatic note") {
			sawNote = true
		}
	}
	if !sawNote {
		t.Errorf("retry request lacks the correction note")
	}

	// Rejected every time: a clear error, not a raw provider message.
	always := newService(t, &fakeGroq{streamErrors: 99})
	errs := find(collect(t, always, ChatRequest{Message: "hi"}), "error")
	if len(errs) != 1 || errs[0]["code"] != "tool_call_rejected" {
		t.Errorf("errors = %v", errs)
	}
}

func TestParseFilters(t *testing.T) {
	ds := engine(t).Dataset()
	cases := map[string]string{
		"gender = Female":                 `{"column":"gender","equals":["Female"]}`,
		"age >= 50":                       `{"column":"age","min":50}`,
		"age <= 65":                       `{"column":"age","max":65}`,
		"smoking_status in Never, Former": `{"column":"smoking_status","equals":["Never","Former"]}`,
		"family_history_diabetes: Yes":    `{"column":"family_history_diabetes","equals":["Yes"]}`,
		"age = 40-50":                     `{"column":"age","min":40,"max":50}`,
		"Smoking status = 'Current'":      `{"column":"smoking_status","equals":["Current"]}`,
		"diabetes_stage in [\"Type 2\"]":  `{"column":"diabetes_stage","equals":["Type 2"]}`,
	}
	for in, want := range cases {
		fs, err := parseFilters(ds, []string{in})
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		b, _ := json.Marshal(fs[0])
		if string(b) != want {
			t.Errorf("%q = %s, want %s", in, b, want)
		}
	}
	for _, bad := range []string{"nonsense", "weight >= 3", "age >= old", "gender != Male"} {
		if _, err := parseFilters(ds, []string{bad}); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if f, _ := parseFilters(ds, []string{"age > 50"}); *f[0].Min <= 50 {
		t.Errorf("age > 50 must exclude 50")
	}
}

// sseEnd terminates a server-sent event.
const sseEnd = "" + string(rune(10)) + string(rune(10))

// newLiteService wires the agent to the lightweight Groq client that the
// WebAssembly build uses.
func newLiteService(t *testing.T, fake *fakeGroq) *Service {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	cfg := Config{APIKey: "gsk_test", Model: "openai/gpt-oss-120b", BaseURL: srv.URL, ReasoningEffort: "low"}
	svc, err := NewWithModel(newGroqLite(cfg.Model, cfg.APIKey, cfg.BaseURL), cfg, engine(t))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestGroqLiteClient(t *testing.T) {
	fake := &fakeGroq{script: []turn{
		{calls: []fakeCall{{id: "call_1", name: "compare_groups", args: map[string]any{
			"metric": "hba1c", "group_by": "family_history_diabetes", "filters": []string{"age >= 40"},
		}}}},
		{calls: []fakeCall{{id: "call_2", name: "get_percentile", args: map[string]any{"columns": []string{"hba1c"}}}}},
		{text: []string{"HbA1c is **higher** ", "with a family history."}},
	}}
	svc := newLiteService(t, fake)
	events := collect(t, svc, ChatRequest{
		Message: "Is HbA1c higher with a family history?", Values: map[string]any{"hba1c": 6.1}, UseValues: true,
	})
	results := find(events, "tool_result")
	if len(results) != 2 || results[0]["ok"] != true || results[1]["ok"] != true {
		t.Fatalf("tool results = %v", results)
	}
	msgs := find(events, "message")
	if len(msgs) != 1 || msgs[0]["text"] != "HbA1c is **higher** with a family history." || len(find(events, "delta")) != 2 {
		t.Errorf("messages = %v", msgs)
	}

	first := fake.requests[0]
	if first["stream"] != true || first["reasoning_effort"] != "low" || first["max_completion_tokens"] != 2048.0 {
		t.Errorf("request basics = %v %v %v", first["stream"], first["reasoning_effort"], first["max_completion_tokens"])
	}
	if n := len(first["tools"].([]any)); n != 6 {
		t.Errorf("tools = %d", n)
	}
	sys := first["messages"].([]any)[0].(map[string]any)
	if sys["role"] != "system" || !strings.Contains(fmt.Sprint(sys["content"]), "- hba1c: 6.1%") {
		t.Errorf("system message = %v", sys["role"])
	}
	// The follow-up request replays the call and its result in OpenAI shape.
	var sawCall, sawResult bool
	for _, m := range fake.requests[1]["messages"].([]any) {
		mm := m.(map[string]any)
		if _, has := mm["name"]; has {
			t.Errorf("messages must not carry a name field: %v", mm)
		}
		if tc, ok := mm["tool_calls"].([]any); ok && mm["role"] == "assistant" {
			fn := tc[0].(map[string]any)["function"].(map[string]any)
			sawCall = tc[0].(map[string]any)["id"] == "call_1" && fn["name"] == "compare_groups"
		}
		if mm["role"] == "tool" && mm["tool_call_id"] == "call_1" {
			sawResult = strings.Contains(fmt.Sprint(mm["content"]), "family_history_diabetes")
		}
	}
	if !sawCall || !sawResult {
		t.Errorf("history replay: call=%v result=%v", sawCall, sawResult)
	}

	// Schema rejections inside the stream are retried.
	retry := newLiteService(t, &fakeGroq{streamErrors: 1, script: []turn{{}, {text: []string{"Recovered."}}}})
	ev := collect(t, retry, ChatRequest{Message: "hi"})
	if len(find(ev, "error")) != 0 || find(ev, "message")[0]["text"] != "Recovered." {
		t.Errorf("retry events = %v", ev)
	}
	// Rate limits become a friendly error after the client's own retries.
	limited := newLiteService(t, &fakeGroq{status: http.StatusTooManyRequests})
	errs := find(collect(t, limited, ChatRequest{Message: "hi"}), "error")
	if len(errs) != 1 || errs[0]["code"] != "rate_limited" {
		t.Errorf("rate limit errors = %v", errs)
	}
}
