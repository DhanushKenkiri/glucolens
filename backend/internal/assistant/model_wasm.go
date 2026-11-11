//go:build js && wasm

package assistant

import (
	"context"
	"errors"

	"google.golang.org/adk/v2/model"
)

const (
	keyHint   = "Check the key you entered on the Ask tab."
	modelHint = "Choose another model on the Ask tab."
)

// newModel uses the lightweight Groq client in the browser build, keeping the
// WebAssembly download small (ADK's openaimodel pulls in openai-go).
func newModel(_ context.Context, cfg Config) (model.LLM, error) {
	return newGroqLite(cfg.Model, cfg.APIKey, cfg.BaseURL), nil
}

// providerError extracts the HTTP status and message of a provider error.
func providerError(err error) (int, string, bool) {
	var g *groqError
	if errors.As(err, &g) {
		return g.Status, g.Message, true
	}
	return 0, "", false
}
