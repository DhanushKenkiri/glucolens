// Package assistant runs the GlucoLens question-answering agent, built with
// Google's Agent Development Kit (ADK) for Go. The agent reaches the dataset
// only through typed tools over the query engine; the model is served by
// Groq through its OpenAI-compatible API.
package assistant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"

	"glucolens/internal/chat"
	"glucolens/internal/query"
)

const (
	appName = "glucolens"
	userID  = "patient"

	defaultMaxLLMCalls = 8
	chatTimeout        = 120 * time.Second
	maxMessageLength   = 2000
)

// Config configures the model behind the assistant.
type Config struct {
	APIKey          string
	Model           string
	BaseURL         string
	ReasoningEffort string // low | medium | high; empty for non-reasoning models
	MaxLLMCalls     int
}

// The contract types live in package chat so the HTTP layer does not depend
// on ADK; these aliases keep the assistant's own code readable.
type (
	Status      = chat.Status
	ChatRequest = chat.Request
	Event       = chat.Event
)

// Errors returned before any event is streamed.
var (
	ErrNotReady    = chat.ErrNotReady
	ErrSessionBusy = chat.ErrSessionBusy
	ErrBadRequest  = chat.ErrBadRequest
)

// Service answers questions with an ADK agent.
type Service struct {
	engine   *query.Engine
	runner   *runner.Runner
	status   Status
	maxCalls int

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// New creates the assistant. Without an API key it returns a service that
// reports itself as not ready instead of failing, so the rest of the app
// keeps working.
func New(ctx context.Context, cfg Config, engine *query.Engine) (*Service, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return &Service{engine: engine, status: Status{
			Provider: "Groq", Model: cfg.Model,
			Reason: "GROQ_API_KEY is not set. Add your Groq key to backend/.env (or the .env in the project root) and restart the server.",
		}}, nil
	}
	llm, err := newModel(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create model: %w", err)
	}
	return NewWithModel(llm, cfg, engine)
}

// NewWithModel creates the assistant around any ADK model (used by tests).
func NewWithModel(llm model.LLM, cfg Config, engine *query.Engine) (*Service, error) {
	tools, err := buildTools(engine)
	if err != nil {
		return nil, fmt.Errorf("build tools: %w", err)
	}
	ds := engine.Dataset()
	schema := schemaSummary(ds)

	genCfg := &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.3),
		MaxOutputTokens: 2048,
	}
	if level, ok := thinkingLevels[strings.ToLower(cfg.ReasoningEffort)]; ok {
		genCfg.ThinkingConfig = &genai.ThinkingConfig{ThinkingLevel: level}
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "glucolens_assistant",
		Description: "Answers questions about the checkup dataset and the user's own values.",
		Model:       llm,
		InstructionProvider: func(ctx agent.ReadonlyContext) (string, error) {
			return buildInstruction(ds, schema, patientFromState(ctx.ReadonlyState())), nil
		},
		Tools:                 tools,
		GenerateContentConfig: genCfg,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	r, err := runner.New(runner.Config{
		AppName:           appName,
		Agent:             a,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create runner: %w", err)
	}
	maxCalls := cfg.MaxLLMCalls
	if maxCalls <= 0 {
		maxCalls = defaultMaxLLMCalls
	}
	return &Service{
		engine:   engine,
		runner:   r,
		status:   Status{Ready: true, Provider: "Groq", Model: cfg.Model},
		maxCalls: maxCalls,
		locks:    map[string]*sync.Mutex{},
	}, nil
}

var thinkingLevels = map[string]genai.ThinkingLevel{
	"minimal": genai.ThinkingLevelMinimal,
	"low":     genai.ThinkingLevelLow,
	"medium":  genai.ThinkingLevelMedium,
	"high":    genai.ThinkingLevelHigh,
}

// Status reports readiness.
func (s *Service) Status() Status { return s.status }

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// Prepare validates a request and fills in a session id when missing.
func (s *Service) Prepare(req *ChatRequest) error {
	if !s.status.Ready {
		return ErrNotReady
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		return fmt.Errorf("%w: message is empty", ErrBadRequest)
	}
	if len([]rune(req.Message)) > maxMessageLength {
		return fmt.Errorf("%w: message is longer than %d characters", ErrBadRequest, maxMessageLength)
	}
	if req.SessionID == "" {
		req.SessionID = newSessionID()
	}
	if !sessionIDPattern.MatchString(req.SessionID) {
		return fmt.Errorf("%w: sessionId must be 8-64 letters, digits, - or _", ErrBadRequest)
	}
	return nil
}

func newSessionID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) sessionLock(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.locks[id]
	if !ok {
		l = &sync.Mutex{}
		s.locks[id] = l
	}
	return l
}

// Chat runs one turn and streams events through emit. Call Prepare first.
// It returns ErrSessionBusy before emitting anything when the session is
// already answering.
func (s *Service) Chat(ctx context.Context, req ChatRequest, emit func(Event) error) error {
	if !s.status.Ready {
		return ErrNotReady
	}
	lock := s.sessionLock(req.SessionID)
	if !lock.TryLock() {
		return ErrSessionBusy
	}
	defer lock.Unlock()

	ctx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()

	ds := s.engine.Dataset()
	patient := map[string]any{}
	if req.UseValues {
		vals, _ := ds.NormalizeAll(req.Values)
		for name, v := range vals {
			patient[name] = v.JSON()
		}
	}

	if err := emit(Event{Type: "session", Data: map[string]any{"sessionId": req.SessionID}}); err != nil {
		return err
	}

	st := &streamState{ds: ds, patient: patient, emit: emit, calls: map[string]map[string]any{}}
	msg := genai.NewContentFromText(req.Message, genai.RoleUser)
	runCfg := agent.RunConfig{StreamingMode: agent.StreamingModeSSE, MaxLLMCalls: s.maxCalls}
	started := time.Now()

	// Groq checks tool-call arguments against the schema and rejects the whole
	// reply when a model writes them in the wrong shape. That is usually a
	// one-off slip, so the turn is retried with a note telling the model what
	// went wrong.
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		runErr := s.runOnce(ctx, req.SessionID, msg, runCfg, patient, st)
		if runErr == nil {
			break
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err() // the browser went away
		}
		var emitErr *emitError
		if errors.As(runErr, &emitErr) {
			return emitErr.err
		}
		if isToolValidationError(runErr) && attempt < maxAttempts {
			slog.Warn("assistant: model sent invalid tool arguments, retrying",
				"session", req.SessionID, "attempt", attempt, "error", truncate(runErr.Error(), 400))
			msg = genai.NewContentFromText(fmt.Sprintf(
				"(Automatic note, not from the user: your last tool call was rejected because its arguments did not match the tool's schema: %s. "+
					"Call the tool again with valid arguments. Filters are a list of strings such as \"age >= 50\" or \"gender = Female\". "+
					"Then answer the user's question: %q)", truncate(runErr.Error(), 300), req.Message), genai.RoleUser)
			continue
		}
		code, message := classifyError(runErr, s.status.Model)
		slog.Error("assistant: answer failed", "session", req.SessionID, "code", code, "error", runErr)
		if e := emit(Event{Type: "error", Data: map[string]any{"code": code, "message": message}}); e != nil {
			return e
		}
		break
	}
	return emit(Event{Type: "done", Data: map[string]any{
		"elapsedMs": time.Since(started).Milliseconds(),
		"toolCalls": st.toolCalls,
	}})
}

// emitError marks a failure to write to the browser, which ends the turn.
type emitError struct{ err error }

func (e *emitError) Error() string { return e.err.Error() }

// runOnce runs the agent for one message and streams its events.
func (s *Service) runOnce(ctx context.Context, sessionID string, msg *genai.Content, runCfg agent.RunConfig, patient map[string]any, st *streamState) error {
	for ev, err := range s.runner.Run(ctx, userID, sessionID, msg, runCfg,
		runner.WithStateDelta(map[string]any{stateKeyPatient: patient})) {
		if err != nil {
			return err
		}
		if err := st.handle(ev); err != nil {
			return &emitError{err: err}
		}
	}
	return nil
}

// isToolValidationError reports Groq's rejection of a tool call whose
// arguments did not match the declared schema (code tool_use_failed).
func isToolValidationError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tool_use_failed") ||
		strings.Contains(msg, "tool call validation failed") ||
		strings.Contains(msg, "failed to call a function")
}
