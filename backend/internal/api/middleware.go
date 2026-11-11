package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"glucolens/internal/query"
)

const maxBodyBytes = 1 << 20

// statusRecorder captures the status code. Unwrap lets
// http.ResponseController reach the real writer to flush SSE events.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				return
			}
			log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status,
				"ms", time.Since(start).Milliseconds())
		})
	}
}

func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler {
						panic(v)
					}
					log.Error("panic", "path", r.URL.Path, "error", v, "stack", string(debug.Stack()))
					writeError(w, http.StatusInternalServerError, "internal_error", "something went wrong on the server")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// cors allows the listed browser origins (the Vite dev server) to call the API.
func cors(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (slices.Contains(allowed, origin) || slices.Contains(allowed, "*")) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type")
				h.Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// errorBody is the JSON shape of every error response.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode_error", "could not encode the response: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	b, _ := json.Marshal(errorBody{Error: errorDetail{Code: code, Message: message}})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// decodeJSON reads a JSON body strictly: unknown fields and trailing data
// are rejected so client mistakes show up as clear 400s.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			return &query.ValidationError{Message: "request body is empty; send a JSON object"}
		case errors.As(err, &maxErr):
			return &query.ValidationError{Message: "request body is too large"}
		default:
			return &query.ValidationError{Message: "invalid JSON body: " + err.Error()}
		}
	}
	if dec.More() {
		return &query.ValidationError{Message: "request body has extra data after the JSON object"}
	}
	return nil
}

// respond writes a query result or maps its error to a status code.
func (s *Server) respond(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err != nil {
		if query.IsValidation(err) {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		s.log.Error("query failed", "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("query failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, v)
}
