// Package chat holds the assistant contract shared by the HTTP layer and the
// agent. It has no dependency on ADK, so builds that serve only the data API
// (the lightweight WebAssembly core) stay small.
package chat

import (
	"context"
	"errors"
)

// Status reports whether the assistant can answer questions.
type Status struct {
	Ready    bool   `json:"ready"`
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Request is one user message.
type Request struct {
	SessionID string
	Message   string
	Values    map[string]any
	UseValues bool
}

// Event is one item of the answer stream sent to the browser.
type Event struct {
	Type string
	Data any
}

// Errors returned before any event is streamed.
var (
	ErrNotReady    = errors.New("assistant is not configured")
	ErrSessionBusy = errors.New("this conversation is still answering the previous question")
	ErrBadRequest  = errors.New("invalid chat request")
)

// Assistant is what the chat endpoint needs.
type Assistant interface {
	Status() Status
	Prepare(req *Request) error
	Chat(ctx context.Context, req Request, emit func(Event) error) error
}

// Unavailable is an assistant that explains why it cannot answer.
type Unavailable struct {
	Provider string
	Reason   string
}

func (u Unavailable) Status() Status                                         { return Status{Provider: u.Provider, Reason: u.Reason} }
func (u Unavailable) Prepare(*Request) error                                 { return ErrNotReady }
func (u Unavailable) Chat(context.Context, Request, func(Event) error) error { return ErrNotReady }
