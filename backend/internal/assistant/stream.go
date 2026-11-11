package assistant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"

	"glucolens/internal/dataset"
)

// streamState turns ADK events into the small protocol the UI reads:
//
//	session     {sessionId}
//	delta       {text}                 a streamed piece of the answer
//	message     {text}                 the complete text of that answer part
//	tool_call   {id, name, label, args}
//	tool_result {id, name, ok, error?, view?, result}
//	error       {code, message}
//	done        {elapsedMs, toolCalls}
type streamState struct {
	ds        *dataset.Dataset
	patient   map[string]any
	emit      func(Event) error
	calls     map[string]map[string]any // tool call id -> args
	toolCalls int
}

func (s *streamState) handle(ev *session.Event) error {
	if ev == nil {
		return nil
	}
	if ev.ErrorCode != "" || ev.ErrorMessage != "" {
		msg := ev.ErrorMessage
		if msg == "" {
			msg = "The model stopped with an error (" + ev.ErrorCode + ")."
		}
		slog.Error("assistant: model error", "code", ev.ErrorCode, "message", msg)
		return s.emit(Event{Type: "error", Data: map[string]any{"code": "model_error", "message": msg}})
	}
	if ev.Content == nil {
		return nil
	}
	var text strings.Builder
	for _, p := range ev.Content.Parts {
		if p == nil || p.Thought {
			continue
		}
		switch {
		case p.Text != "":
			if ev.Partial {
				if err := s.emit(Event{Type: "delta", Data: map[string]any{"text": p.Text}}); err != nil {
					return err
				}
			} else {
				text.WriteString(p.Text)
			}
		case p.FunctionCall != nil && !ev.Partial:
			fc := p.FunctionCall
			s.calls[fc.ID] = fc.Args
			s.toolCalls++
			if err := s.emit(Event{Type: "tool_call", Data: map[string]any{
				"id": fc.ID, "name": fc.Name, "label": labelFor(s.ds, fc.Name, fc.Args), "args": fc.Args,
			}}); err != nil {
				return err
			}
		case p.FunctionResponse != nil:
			fr := p.FunctionResponse
			data := map[string]any{"id": fr.ID, "name": fr.Name, "ok": true, "result": fr.Response}
			if e, ok := fr.Response["error"]; ok {
				data["ok"] = false
				data["error"] = fmt.Sprint(e)
				slog.Warn("assistant: tool returned an error to the model", "tool", fr.Name, "args", s.calls[fr.ID], "error", e)
			} else if v := viewFor(s.ds, fr.Name, s.calls[fr.ID], s.patient); v != nil {
				data["view"] = v
			}
			if err := s.emit(Event{Type: "tool_result", Data: data}); err != nil {
				return err
			}
		}
	}
	if text.Len() > 0 {
		return s.emit(Event{Type: "message", Data: map[string]any{"text": text.String()}})
	}
	return nil
}

// classifyError turns a run error into a code and a message a person can
// act on.
func classifyError(err error, modelName string) (string, string) {
	if status, providerMsg, ok := providerError(err); ok {
		detail := strings.TrimSpace(providerMsg)
		switch status {
		case 401, 403:
			return "invalid_api_key", "Groq rejected the API key. " + keyHint
		case 404:
			return "model_not_found", fmt.Sprintf("Groq does not offer the model %q to this key. %s", modelName, modelHint)
		case 413:
			return "request_too_large", "The conversation got too long for the model. Start a new chat."
		case 429:
			return "rate_limited", "Groq's rate limit was reached. Wait a minute and try again, or set GROQ_MODEL to a lighter model such as openai/gpt-oss-20b."
		case 400, 422:
			return "provider_rejected", "Groq rejected the request: " + truncate(detail, 300)
		default:
			return "provider_error", fmt.Sprintf("Groq returned an error (status %d). Try again shortly.", status)
		}
	}
	switch {
	case isToolValidationError(err):
		return "tool_call_rejected", "The model kept sending a data tool request in the wrong format. Try rephrasing the question."
	case errors.Is(err, agent.ErrLLMCallsLimitExceeded):
		return "too_many_steps", "That question needed too many steps. Try asking something more specific."
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout", "The answer took too long. Try again, or ask something more specific."
	}
	return "assistant_error", "The assistant hit a problem: " + truncate(err.Error(), 300)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
