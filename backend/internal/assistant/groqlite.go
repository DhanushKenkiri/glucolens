package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// groqLite is a small ADK model.LLM for Groq's OpenAI-compatible Chat
// Completions API, written with net/http and encoding/json only. The
// WebAssembly build uses it instead of ADK's openaimodel adapter, whose
// openai-go dependency would make the browser download several times larger.
// It supports what this app needs: a system instruction, text turns,
// function tools, streaming and errors.
type groqLite struct {
	name    string
	apiKey  string
	baseURL string
	client  *http.Client
}

func newGroqLite(name, apiKey, baseURL string) *groqLite {
	if baseURL == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}
	return &groqLite{name: name, apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), client: &http.Client{}}
}

func (g *groqLite) Name() string { return g.name }

// groqError is an error reported by the API, with its HTTP status.
type groqError struct {
	Status  int
	Code    string
	Message string
}

func (e *groqError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("groq: %s (status %d, %s)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("groq: %s (status %d)", e.Message, e.Status)
}

type glMessage struct {
	Role       string       `json:"role"`
	Content    *string      `json:"content,omitempty"`
	ToolCalls  []glToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type glToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type glTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

type glRequest struct {
	Model               string      `json:"model"`
	Messages            []glMessage `json:"messages"`
	Tools               []glTool    `json:"tools,omitempty"`
	Stream              bool        `json:"stream"`
	Temperature         *float32    `json:"temperature,omitempty"`
	MaxCompletionTokens int32       `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string      `json:"reasoning_effort,omitempty"`
}

type glChoice struct {
	Delta        glMessage `json:"delta"`
	Message      glMessage `json:"message"`
	FinishReason string    `json:"finish_reason"`
}

type glChunk struct {
	Choices []glChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

func strPtr(s string) *string { return &s }

// buildRequest converts ADK's request into a Chat Completions body.
func (g *groqLite) buildRequest(req *model.LLMRequest, stream bool) (*glRequest, error) {
	out := &glRequest{Model: g.name, Stream: stream}
	if req.Model != "" {
		out.Model = req.Model
	}
	if cfg := req.Config; cfg != nil {
		out.Temperature = cfg.Temperature
		out.MaxCompletionTokens = cfg.MaxOutputTokens
		if tc := cfg.ThinkingConfig; tc != nil && tc.ThinkingLevel != "" && tc.ThinkingLevel != genai.ThinkingLevelUnspecified {
			out.ReasoningEffort = strings.ToLower(string(tc.ThinkingLevel))
			if out.ReasoningEffort == "minimal" {
				out.ReasoningEffort = "low"
			}
		}
		if si := cfg.SystemInstruction; si != nil {
			var sb strings.Builder
			for _, p := range si.Parts {
				if p != nil && p.Text != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n\n")
					}
					sb.WriteString(p.Text)
				}
			}
			if sb.Len() > 0 {
				out.Messages = append(out.Messages, glMessage{Role: "system", Content: strPtr(sb.String())})
			}
		}
		for _, t := range cfg.Tools {
			if t == nil {
				continue
			}
			for _, fd := range t.FunctionDeclarations {
				if fd == nil {
					continue
				}
				var tool glTool
				tool.Type = "function"
				tool.Function.Name = fd.Name
				tool.Function.Description = fd.Description
				switch {
				case fd.ParametersJsonSchema != nil:
					tool.Function.Parameters = fd.ParametersJsonSchema
				case fd.Parameters != nil:
					tool.Function.Parameters = fd.Parameters
				default:
					tool.Function.Parameters = map[string]any{"type": "object", "properties": map[string]any{}}
				}
				out.Tools = append(out.Tools, tool)
			}
		}
	}

	for _, c := range req.Contents {
		if c == nil {
			continue
		}
		var text strings.Builder
		var calls []glToolCall
		for _, p := range c.Parts {
			if p == nil || p.Thought {
				continue
			}
			switch {
			case p.FunctionResponse != nil:
				b, err := json.Marshal(p.FunctionResponse.Response)
				if err != nil {
					return nil, err
				}
				out.Messages = append(out.Messages, glMessage{Role: "tool", ToolCallID: p.FunctionResponse.ID, Content: strPtr(string(b))})
			case p.FunctionCall != nil:
				b, err := json.Marshal(p.FunctionCall.Args)
				if err != nil {
					return nil, err
				}
				var tc glToolCall
				tc.ID, tc.Type = p.FunctionCall.ID, "function"
				tc.Function.Name, tc.Function.Arguments = p.FunctionCall.Name, string(b)
				calls = append(calls, tc)
			case p.Text != "":
				text.WriteString(p.Text)
			}
		}
		role := "user"
		if c.Role == genai.RoleModel {
			role = "assistant"
		}
		if text.Len() == 0 && len(calls) == 0 {
			continue
		}
		m := glMessage{Role: role, ToolCalls: calls}
		if text.Len() > 0 {
			m.Content = strPtr(text.String())
		}
		out.Messages = append(out.Messages, m)
	}
	if len(out.Messages) == 0 {
		return nil, errors.New("groq: nothing to send")
	}
	return out, nil
}

// GenerateContent implements model.LLM.
func (g *groqLite) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		body, err := g.buildRequest(req, stream)
		if err != nil {
			yield(nil, err)
			return
		}
		resp, err := g.post(ctx, body)
		if err != nil {
			yield(nil, err)
			return
		}
		defer resp.Body.Close()
		if !stream {
			var full glChunk
			if err := json.NewDecoder(resp.Body).Decode(&full); err != nil {
				yield(nil, fmt.Errorf("groq: read response: %w", err))
				return
			}
			if len(full.Choices) == 0 {
				yield(nil, errors.New("groq: empty response"))
				return
			}
			m := full.Choices[0].Message
			text := ""
			if m.Content != nil {
				text = *m.Content
			}
			final, err := finalResponse(text, m.ToolCalls, full.Choices[0].FinishReason)
			yield(final, err)
			return
		}

		var text strings.Builder
		var calls []glToolCall
		finish := ""
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			var chunk glChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if chunk.Error != nil {
				// Groq reports some failures inside the stream, for example a
				// tool call whose arguments do not match the schema.
				yield(nil, &groqError{Status: http.StatusBadRequest, Code: chunk.Error.Code, Message: chunk.Error.Message})
				return
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			ch := chunk.Choices[0]
			if ch.FinishReason != "" {
				finish = ch.FinishReason
			}
			if ch.Delta.Content != nil && *ch.Delta.Content != "" {
				text.WriteString(*ch.Delta.Content)
				partial := &model.LLMResponse{
					Content: &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{{Text: *ch.Delta.Content}}},
					Partial: true,
				}
				if !yield(partial, nil) {
					return
				}
			}
			for _, d := range ch.Delta.ToolCalls {
				i := len(calls)
				if d.Index != nil {
					i = *d.Index
				}
				for len(calls) <= i {
					calls = append(calls, glToolCall{})
				}
				if d.ID != "" {
					calls[i].ID = d.ID
				}
				if d.Function.Name != "" {
					calls[i].Function.Name += d.Function.Name
				}
				calls[i].Function.Arguments += d.Function.Arguments
			}
		}
		if err := sc.Err(); err != nil {
			yield(nil, fmt.Errorf("groq: read stream: %w", err))
			return
		}
		final, err := finalResponse(text.String(), calls, finish)
		yield(final, err)
	}
}

// post sends the request, retrying briefly on rate limits and server errors.
func (g *groqLite) post(ctx context.Context, body *glRequest) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/chat/completions", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)
		if body.Stream {
			httpReq.Header.Set("Accept", "text/event-stream")
		}
		resp, err := g.client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("groq: request failed: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		lastErr = readGroqError(resp)
		resp.Body.Close()
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if !retryable || attempt == 2 {
			return nil, lastErr
		}
		wait := time.Duration(attempt+1) * time.Second
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 && s <= 10 {
			wait = time.Duration(s) * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

func readGroqError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var body glChunk
	if json.Unmarshal(b, &body) == nil && body.Error != nil {
		return &groqError{Status: resp.StatusCode, Code: body.Error.Code, Message: body.Error.Message}
	}
	msg := strings.TrimSpace(string(b))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return &groqError{Status: resp.StatusCode, Message: truncate(msg, 300)}
}

func finalResponse(text string, calls []glToolCall, finish string) (*model.LLMResponse, error) {
	content := &genai.Content{Role: genai.RoleModel}
	if text != "" {
		content.Parts = append(content.Parts, &genai.Part{Text: text})
	}
	for i, c := range calls {
		if c.Function.Name == "" {
			continue
		}
		args := map[string]any{}
		if strings.TrimSpace(c.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(c.Function.Arguments), &args); err != nil {
				return nil, &groqError{Status: http.StatusBadRequest, Code: "tool_use_failed",
					Message: fmt.Sprintf("tool call validation failed: arguments for %s are not valid JSON", c.Function.Name)}
			}
		}
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		content.Parts = append(content.Parts, &genai.Part{FunctionCall: &genai.FunctionCall{ID: id, Name: c.Function.Name, Args: args}})
	}
	reason := genai.FinishReasonStop
	if finish == "length" {
		reason = genai.FinishReasonMaxTokens
	}
	return &model.LLMResponse{Content: content, TurnComplete: true, FinishReason: reason}, nil
}
