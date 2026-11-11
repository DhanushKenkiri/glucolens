//go:build js && wasm

// Command wasm-core serves the GlucoLens data API inside the browser for the
// static GitHub Pages build: the same dataset loader, query engine and
// gorilla/mux router as the server, without the agent (which loads separately
// as wasm-agent when someone uses the Ask tab).
package main

import (
	"errors"
	"net/http"
	"sync"

	"glucolens/internal/api"
	"glucolens/internal/chat"
	"glucolens/internal/dataset"
	"glucolens/internal/query"
	"glucolens/internal/wasmbridge"
)

type core struct {
	mu      sync.Mutex
	handler http.Handler
}

func (c *core) Load(data []byte) (map[string]any, error) {
	ds, err := dataset.LoadBytes(data)
	if err != nil {
		return nil, err
	}
	h := api.NewRouter(query.New(ds), chat.Unavailable{
		Provider: "Groq",
		Reason:   "Add your Groq API key to use the assistant on this site. It is kept in this browser and sent only to Groq.",
	}, api.Options{})
	c.mu.Lock()
	c.handler = h
	c.mu.Unlock()
	return map[string]any{"rows": ds.Rows, "columns": len(ds.Columns)}, nil
}

func (c *core) Configure(string, string, string) (map[string]any, error) {
	return nil, errors.New("the assistant runs in the agent build")
}

func (c *core) Handler() http.Handler {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handler
}

func main() { wasmbridge.Run(&core{}) }
