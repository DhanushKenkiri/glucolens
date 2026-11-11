//go:build !(js && wasm)

package assistant

import (
	"context"
	"errors"

	"github.com/openai/openai-go/v3"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/openaimodel"
)

const (
	keyHint   = "Check GROQ_API_KEY in backend/.env and restart the server."
	modelHint = "Change GROQ_MODEL in backend/.env."
)

// newModel uses ADK's OpenAI-compatible adapter on the server.
func newModel(ctx context.Context, cfg Config) (model.LLM, error) {
	return openaimodel.NewModel(ctx, cfg.Model, &openaimodel.ClientConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		// Groq serves the Chat Completions API.
		API: openaimodel.APIChatCompletions,
	})
}

// providerError extracts the HTTP status and message of a provider error.
func providerError(err error) (int, string, bool) {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode, apiErr.Message, true
	}
	var g *groqError
	if errors.As(err, &g) {
		return g.Status, g.Message, true
	}
	return 0, "", false
}
