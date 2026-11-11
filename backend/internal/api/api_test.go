package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"glucolens/internal/chat"
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
		dir, err := os.MkdirTemp("", "glucolens-api-test")
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

// fakeAssistant streams a fixed answer, or reports itself unavailable.
type fakeAssistant struct{ ready bool }

func (f *fakeAssistant) Status() chat.Status {
	if !f.ready {
		return chat.Status{Provider: "Groq", Reason: "GROQ_API_KEY is not set."}
	}
	return chat.Status{Ready: true, Provider: "Groq", Model: "test"}
}

func (f *fakeAssistant) Prepare(req *chat.Request) error {
	if !f.ready {
		return chat.ErrNotReady
	}
	if strings.TrimSpace(req.Message) == "" {
		return chat.ErrBadRequest
	}
	if req.SessionID == "" {
		req.SessionID = "session-test"
	}
	return nil
}

func (f *fakeAssistant) Chat(_ context.Context, req chat.Request, emit func(chat.Event) error) error {
	for _, ev := range []chat.Event{
		{Type: "session", Data: map[string]any{"sessionId": req.SessionID}},
		{Type: "delta", Data: map[string]any{"text": "Hello"}},
		{Type: "message", Data: map[string]any{"text": "Hello there"}},
		{Type: "done", Data: map[string]any{}},
	} {
		if err := emit(ev); err != nil {
			return err
		}
	}
	return nil
}

func newTestServer(t *testing.T, ready bool, staticDir string) *httptest.Server {
	t.Helper()
	h := NewRouter(engine(t), &fakeAssistant{ready: ready}, Options{
		AllowedOrigins: []string{"http://localhost:5173"},
		StaticDir:      staticDir,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, method, url, body string, headers ...string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func errorCode(t *testing.T, body string) string {
	t.Helper()
	var e errorBody
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("not an error body: %s", body)
	}
	return e.Error.Code
}

func TestDataEndpoints(t *testing.T) {
	srv := newTestServer(t, false, "")
	base := srv.URL + "/api/v1"

	resp, body := do(t, "GET", base+"/health", "")
	if resp.StatusCode != 200 || !strings.Contains(body, `"rows":100000`) || !strings.Contains(body, `"ready":false`) {
		t.Fatalf("health: %d %s", resp.StatusCode, body)
	}

	resp, body = do(t, "GET", base+"/schema", "")
	var schema query.SchemaResponse
	if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &schema) != nil || len(schema.Columns) != 31 {
		t.Fatalf("schema: %d", resp.StatusCode)
	}

	ok := map[string]string{
		"/query/distribution": `{"column":"bmi","splitBy":"gender"}`,
		"/query/compare":      `{"metric":"hba1c","groupBy":"age"}`,
		"/query/risk":         `{"outcome":{"preset":"diagnosed_diabetes"},"factor":"smoking_status","stratifyBy":"age"}`,
		"/query/risk/rank":    `{"outcome":{"column":"hba1c","min":6.5}}`,
		"/query/profile":      `{"values":{"hba1c":6.1,"gender":"Female"}}`,
		"/query/similar":      `{"values":{"age":50,"bmi":28,"gender":"Male"},"k":100}`,
		"/query/relationship": `{"x":"age","y":"systolic_bp"}`,
		"/records":            `{"filters":[{"column":"gender","equals":["Other"]}],"limit":3}`,
	}
	for path, payload := range ok {
		resp, body := do(t, "POST", base+path, payload)
		if resp.StatusCode != 200 {
			t.Errorf("%s: %d %s", path, resp.StatusCode, body)
		}
	}

	resp, body = do(t, "GET", base+"/records?offset=5&limit=2", "")
	var recs query.RecordsResponse
	if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &recs) != nil || len(recs.Rows) != 2 || recs.Total != 100000 {
		t.Errorf("records: %d %s", resp.StatusCode, body)
	}
	resp, body = do(t, "GET", base+"/records/random", "")
	var rnd struct {
		Values map[string]any `json:"values"`
	}
	if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &rnd) != nil || len(rnd.Values) != 31 {
		t.Errorf("random record: %d %s", resp.StatusCode, body)
	}
	resp, _ = do(t, "GET", base+"/query/correlations", "")
	if resp.StatusCode != 200 {
		t.Errorf("correlations: %d", resp.StatusCode)
	}
}

func TestErrorsAndRouting(t *testing.T) {
	srv := newTestServer(t, false, "")
	base := srv.URL + "/api/v1"
	cases := []struct {
		method, path, body string
		status             int
		code               string
	}{
		{"POST", "/query/risk", `{"outcome":{"preset":"diagnosed_diabetes"},"factor":"nope"}`, 400, "invalid_request"},
		{"POST", "/query/distribution", `{"column":"hba1c","bogus":true}`, 400, "invalid_request"},
		{"POST", "/query/distribution", ``, 400, "invalid_request"},
		{"POST", "/query/distribution", `{"column":"hba1c"} {}`, 400, "invalid_request"},
		{"POST", "/query/profile", `{"values":{}}`, 400, "invalid_request"},
		{"GET", "/records?limit=-1", ``, 400, "invalid_request"},
		{"GET", "/nope", ``, 404, "not_found"},
		{"DELETE", "/schema", ``, 405, "method_not_allowed"},
		{"POST", "/chat", `{"message":"hi"}`, 503, "assistant_unavailable"},
	}
	for _, c := range cases {
		resp, body := do(t, c.method, base+c.path, c.body)
		if resp.StatusCode != c.status || errorCode(t, body) != c.code {
			t.Errorf("%s %s: got %d %s, want %d %s", c.method, c.path, resp.StatusCode, body, c.status, c.code)
		}
	}

	// CORS preflight from the dev server.
	resp, _ := do(t, "OPTIONS", base+"/query/risk", "", "Origin", "http://localhost:5173", "Access-Control-Request-Method", "POST")
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	resp, _ = do(t, "GET", base+"/health", "", "Origin", "http://evil.example")
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("unknown origins must not be allowed")
	}
}

func TestChatStreamsServerSentEvents(t *testing.T) {
	srv := newTestServer(t, true, "")
	resp, body := do(t, "POST", srv.URL+"/api/v1/chat", `{"message":"Hello?","values":{"age":40},"useValues":true}`)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("chat: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	want := "event: session\ndata: {\"sessionId\":\"session-test\"}\n\nevent: delta\ndata: {\"text\":\"Hello\"}\n\n"
	if !strings.HasPrefix(body, want) || !strings.Contains(body, "event: done\n") {
		t.Errorf("stream body = %q", body)
	}
	resp, body = do(t, "POST", srv.URL+"/api/v1/chat", `{"message":"   "}`)
	if resp.StatusCode != 400 {
		t.Errorf("blank message: %d %s", resp.StatusCode, body)
	}
}

func TestServesBuiltFrontend(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>GlucoLens</title>"), 0o644)
	os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644)
	srv := newTestServer(t, false, dir)

	for _, path := range []string{"/", "/insights", "/risk?outcome=x"} {
		resp, body := do(t, "GET", srv.URL+path, "")
		if resp.StatusCode != 200 || !strings.Contains(body, "GlucoLens") {
			t.Errorf("%s: %d %s", path, resp.StatusCode, body)
		}
	}
	resp, body := do(t, "GET", srv.URL+"/assets/app.js", "")
	if resp.StatusCode != 200 || body != "console.log(1)" || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %d %q %v", resp.StatusCode, body, resp.Header)
	}
	resp, _ = do(t, "GET", srv.URL+"/api/v1/nope", "")
	if resp.StatusCode != 404 {
		t.Errorf("unknown API routes must not fall back to the app: %d", resp.StatusCode)
	}
}
