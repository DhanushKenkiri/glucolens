package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"glucolens/internal/chat"
	"glucolens/internal/query"
)

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	ds := s.engine.Dataset()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"rows":      ds.Rows,
		"columns":   len(ds.Columns),
		"dataset":   map[string]any{"name": ds.Name, "source": ds.Source},
		"assistant": s.assistant.Status(),
	})
}

func (s *Server) schema(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.engine.Schema())
}

// handle decodes a JSON request body, runs the query and writes the result.
func handle[Req any, Resp any](s *Server, run func(Req) (Resp, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req
		if err := decodeJSON(w, r, &req); err != nil {
			s.respond(w, r, nil, err)
			return
		}
		resp, err := run(req)
		s.respond(w, r, resp, err)
	}
}

func (s *Server) distribution(w http.ResponseWriter, r *http.Request) {
	handle(s, s.engine.Distribution)(w, r)
}

func (s *Server) compare(w http.ResponseWriter, r *http.Request) {
	handle(s, s.engine.Compare)(w, r)
}

func (s *Server) risk(w http.ResponseWriter, r *http.Request) {
	handle(s, s.engine.Risk)(w, r)
}

func (s *Server) rank(w http.ResponseWriter, r *http.Request) {
	handle(s, s.engine.Rank)(w, r)
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	handle(s, s.engine.Profile)(w, r)
}

func (s *Server) similar(w http.ResponseWriter, r *http.Request) {
	handle(s, s.engine.Similar)(w, r)
}

func (s *Server) relationship(w http.ResponseWriter, r *http.Request) {
	handle(s, s.engine.Relationship)(w, r)
}

func (s *Server) correlations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.engine.Correlations())
}

type recordsRequest struct {
	Filters []query.Filter `json:"filters,omitempty"`
	Offset  int            `json:"offset,omitempty"`
	Limit   int            `json:"limit,omitempty"`
}

func (s *Server) records(w http.ResponseWriter, r *http.Request) {
	var req recordsRequest
	if r.Method == http.MethodPost {
		if err := decodeJSON(w, r, &req); err != nil {
			s.respond(w, r, nil, err)
			return
		}
	} else {
		var err error
		if req.Offset, err = intParam(r, "offset"); err == nil {
			req.Limit, err = intParam(r, "limit")
		}
		if err != nil {
			s.respond(w, r, nil, err)
			return
		}
	}
	resp, err := s.engine.Records(req.Filters, req.Offset, req.Limit)
	s.respond(w, r, resp, err)
}

func intParam(r *http.Request, name string) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, &query.ValidationError{Message: fmt.Sprintf("%s must be a non-negative whole number", name)}
	}
	return n, nil
}

func (s *Server) randomRecord(w http.ResponseWriter, _ *http.Request) {
	idx, row := s.engine.RandomRecord()
	writeJSON(w, http.StatusOK, map[string]any{"index": idx, "values": row})
}

type chatBody struct {
	SessionID string         `json:"sessionId,omitempty"`
	Message   string         `json:"message"`
	Values    map[string]any `json:"values,omitempty"`
	UseValues bool           `json:"useValues,omitempty"`
}

// chat streams the assistant's answer as server-sent events.
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var body chatBody
	if err := decodeJSON(w, r, &body); err != nil {
		s.respond(w, r, nil, err)
		return
	}
	req := chat.Request{
		SessionID: body.SessionID, Message: body.Message, Values: body.Values, UseValues: body.UseValues,
	}
	if err := s.assistant.Prepare(&req); err != nil {
		switch {
		case errors.Is(err, chat.ErrNotReady):
			st := s.assistant.Status()
			writeError(w, http.StatusServiceUnavailable, "assistant_unavailable", st.Reason)
		case errors.Is(err, chat.ErrBadRequest):
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}

	rc := http.NewResponseController(w)
	started := false
	emit := func(ev chat.Event) error {
		if !started {
			h := w.Header()
			h.Set("Content-Type", "text/event-stream; charset=utf-8")
			h.Set("Cache-Control", "no-cache, no-transform")
			h.Set("Connection", "keep-alive")
			h.Set("X-Accel-Buffering", "no")
			w.WriteHeader(http.StatusOK)
			started = true
		}
		data, err := json.Marshal(ev.Data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data); err != nil {
			return err
		}
		return rc.Flush()
	}

	err := s.assistant.Chat(r.Context(), req, emit)
	switch {
	case err == nil:
	case errors.Is(err, chat.ErrSessionBusy) && !started:
		writeError(w, http.StatusConflict, "session_busy", err.Error())
	case r.Context().Err() != nil:
		// The browser disconnected; nothing left to send.
	case !started:
		s.log.Error("chat failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	default:
		s.log.Error("chat stream failed", "error", err)
		_ = emit(chat.Event{Type: "error", Data: map[string]any{"code": "stream_error", "message": err.Error()}})
	}
}
