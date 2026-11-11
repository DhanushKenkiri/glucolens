//go:build js && wasm

// Command wasm-agent is the full GlucoLens API in the browser: the data API
// plus the ADK agent, which calls Groq directly with a key the visitor
// supplies. The static site loads it when someone uses the Ask tab.
package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"glucolens/internal/api"
	"glucolens/internal/assistant"
	"glucolens/internal/dataset"
	"glucolens/internal/query"
	"glucolens/internal/wasmbridge"
)

const groqBaseURL = "https://api.groq.com/openai/v1"

// browserAssistant explains how to enable the assistant on the static site.
type browserAssistant struct{ *assistant.Service }

func (b browserAssistant) Status() assistant.Status {
	st := b.Service.Status()
	if !st.Ready {
		st.Reason = "Add your Groq API key to use the assistant on this site. It is kept in this browser and sent only to Groq."
	}
	return st
}

type agentApp struct {
	mu      sync.Mutex
	engine  *query.Engine
	handler http.Handler
}

func (a *agentApp) Load(data []byte) (map[string]any, error) {
	ds, err := dataset.LoadBytes(data)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.engine = query.New(ds)
	a.mu.Unlock()
	if _, err := a.Configure("", "", ""); err != nil {
		return nil, err
	}
	return map[string]any{"rows": ds.Rows, "columns": len(ds.Columns)}, nil
}

func (a *agentApp) Configure(key, model, baseURL string) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.engine == nil {
		return nil, errors.New("the dataset is not loaded yet")
	}
	if model == "" {
		model = "openai/gpt-oss-120b"
	}
	if baseURL == "" {
		baseURL = groqBaseURL
	}
	effort := ""
	if strings.Contains(model, "gpt-oss") || strings.Contains(model, "qwen3") {
		effort = "low"
	}
	svc, err := assistant.New(context.Background(), assistant.Config{
		APIKey: key, Model: model, BaseURL: baseURL, ReasoningEffort: effort,
	}, a.engine)
	if err != nil {
		return nil, err
	}
	a.handler = api.NewRouter(a.engine, browserAssistant{svc}, api.Options{})
	return map[string]any{"ready": svc.Status().Ready, "model": model}, nil
}

func (a *agentApp) Handler() http.Handler {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.handler
}

func main() { wasmbridge.Run(&agentApp{}) }
