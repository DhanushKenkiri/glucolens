// Command server runs the GlucoLens API: it converts the checkup CSV to JSON
// once, loads it, and serves the query API, the assistant and (when built)
// the frontend.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"glucolens/internal/api"
	"glucolens/internal/assistant"
	"glucolens/internal/config"
	"glucolens/internal/dataset"
	"glucolens/internal/query"
)

func main() {
	envFiles := flag.String("env", ".env,../.env", "comma-separated .env files, highest priority first")
	reconvert := flag.Bool("reconvert", false, "regenerate the JSON dataset from the CSV")
	convertOnly := flag.Bool("convert-only", false, "convert the CSV to JSON and exit")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	cfg := config.Load(strings.Split(*envFiles, ",")...)

	start := time.Now()
	ds, res, err := dataset.Open(cfg.DatasetCSV, cfg.DatasetJSON, *reconvert)
	if err != nil {
		logger.Error("could not load the dataset", "csv", cfg.DatasetCSV, "json", cfg.DatasetJSON, "error", err)
		os.Exit(1)
	}
	if res.Converted {
		logger.Info("converted CSV to JSON", "reason", res.Reason, "csv", cfg.DatasetCSV, "json", cfg.DatasetJSON)
	}
	logger.Info("dataset loaded", "rows", ds.Rows, "columns", len(ds.Columns),
		"source", ds.Source.File, "ms", time.Since(start).Milliseconds())
	if *convertOnly {
		return
	}

	engine := query.New(ds)
	go engine.Correlations() // warm the cache

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	asst, err := assistant.New(ctx, assistant.Config{
		APIKey:          cfg.GroqAPIKey,
		Model:           cfg.GroqModel,
		BaseURL:         cfg.GroqBaseURL,
		ReasoningEffort: cfg.GroqReasoningEffort,
	}, engine)
	if err != nil {
		logger.Error("could not create the assistant", "error", err)
		os.Exit(1)
	}
	if st := asst.Status(); st.Ready {
		logger.Info("assistant ready", "provider", st.Provider, "model", st.Model, "reasoning_effort", cfg.GroqReasoningEffort)
	} else {
		logger.Warn("assistant disabled", "reason", st.Reason)
	}

	handler := api.NewRouter(engine, asst, api.Options{
		AllowedOrigins: cfg.AllowedOrigins,
		StaticDir:      cfg.StaticDir,
		Logger:         logger,
	})
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No write timeout: answers stream for as long as the assistant runs
		// (it has its own time limit).
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "url", "http://localhost:"+cfg.Port)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}
}
