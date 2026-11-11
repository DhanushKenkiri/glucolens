// Package api exposes the query engine and the assistant over HTTP using
// gorilla/mux.
package api

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"

	"glucolens/internal/chat"
	"glucolens/internal/query"
)

// Assistant is what the chat endpoint needs from the assistant service.
type Assistant = chat.Assistant

// Options configures the router.
type Options struct {
	AllowedOrigins []string
	StaticDir      string // built frontend; served at / when it exists
	Logger         *slog.Logger
}

// Server holds the handlers' dependencies.
type Server struct {
	engine    *query.Engine
	assistant Assistant
	log       *slog.Logger
}

// NewRouter builds the HTTP handler: the API under /api/v1 and, when the
// frontend has been built, the single-page app at /.
func NewRouter(engine *query.Engine, asst Assistant, opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{engine: engine, assistant: asst, log: logger}

	r := mux.NewRouter()
	// Routes are registered with full paths on the root router: gorilla/mux
	// subrouters lose "method not allowed" errors once a later route's prefix
	// matches.
	route := func(path string, h http.HandlerFunc, methods ...string) {
		r.HandleFunc("/api/v1"+path, h).Methods(methods...)
	}
	route("/health", s.health, http.MethodGet)
	route("/schema", s.schema, http.MethodGet)
	route("/query/distribution", s.distribution, http.MethodPost)
	route("/query/compare", s.compare, http.MethodPost)
	route("/query/risk", s.risk, http.MethodPost)
	route("/query/risk/rank", s.rank, http.MethodPost)
	route("/query/profile", s.profile, http.MethodPost)
	route("/query/similar", s.similar, http.MethodPost)
	route("/query/correlations", s.correlations, http.MethodGet)
	route("/query/relationship", s.relationship, http.MethodPost)
	route("/records", s.records, http.MethodGet, http.MethodPost)
	route("/records/random", s.randomRecord, http.MethodGet)
	route("/chat", s.chat, http.MethodPost)
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no route "+r.Method+" "+r.URL.Path)
	})
	r.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", r.Method+" is not allowed on "+r.URL.Path)
	})

	if dir := opts.StaticDir; dir != "" {
		if info, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !info.IsDir() {
			r.PathPrefix("/").Methods(http.MethodGet, http.MethodHead).Handler(spaHandler(dir))
			logger.Info("serving the built frontend", "dir", dir)
		}
	}
	// The middleware wraps the router so it also covers 404s and CORS
	// preflight requests, which match no route.
	return recoverer(logger)(requestLogger(logger)(cors(opts.AllowedOrigins)(r)))
}

// spaHandler serves static files and falls back to index.html so client-side
// routes such as /insights work on reload.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "not_found", "no API route "+r.URL.Path)
			return
		}
		clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if clean != "." && !strings.HasPrefix(clean, "..") {
			if info, err := os.Stat(filepath.Join(dir, clean)); err == nil && !info.IsDir() {
				if strings.HasPrefix(clean, "assets"+string(filepath.Separator)) {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
