//go:build js && wasm

// Package wasmbridge exposes an http.Handler to JavaScript in a WebAssembly
// build. The browser calls glucolens.fetch(...) and receives the status,
// content type and body chunks through callbacks, so the frontend can wrap a
// request in an ordinary fetch Response, including streamed answers.
package wasmbridge

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"syscall/js"
)

// App is what a WebAssembly build provides.
type App interface {
	// Load receives the converted dataset JSON.
	Load(data []byte) (map[string]any, error)
	// Configure receives the visitor's Groq key, model and (for tests) an
	// alternative OpenAI-compatible base URL.
	Configure(apiKey, model, baseURL string) (map[string]any, error)
	// Handler serves the API once loaded.
	Handler() http.Handler
}

var (
	mu      sync.Mutex
	cancels = map[int]context.CancelFunc{}
)

// Run registers the glucolens global and blocks forever.
func Run(app App) {
	js.Global().Set("glucolens", js.ValueOf(map[string]any{
		"load": js.FuncOf(func(_ js.Value, args []js.Value) any {
			data := make([]byte, args[0].Get("length").Int())
			js.CopyBytesToGo(data, args[0])
			return promise(func() (any, error) { return app.Load(data) })
		}),
		"configure": js.FuncOf(func(_ js.Value, args []js.Value) any {
			key := strings.TrimSpace(args[0].Get("apiKey").String())
			str := func(name string) string {
				if v := args[0].Get(name); v.Type() == js.TypeString {
					return strings.TrimSpace(v.String())
				}
				return ""
			}
			model, baseURL := str("model"), str("baseURL")
			return promise(func() (any, error) { return app.Configure(key, model, baseURL) })
		}),
		"fetch": js.FuncOf(func(_ js.Value, args []js.Value) any { return fetch(app, args) }),
		"abort": js.FuncOf(abort),
	}))
	select {}
}

// promise runs fn on a goroutine and settles a JS promise with its result.
func promise(fn func() (any, error)) js.Value {
	executor := js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve, reject := args[0], args[1]
		go func() {
			v, err := fn()
			if err != nil {
				reject.Invoke(js.Global().Get("Error").New(err.Error()))
				return
			}
			resolve.Invoke(js.ValueOf(v))
		}()
		return nil
	})
	return js.Global().Get("Promise").New(executor)
}

// fetch(id, method, path, body, onHead(status, contentType), onChunk(text), onEnd(error|null))
func fetch(app App, args []js.Value) any {
	id := args[0].Int()
	method, path, body := args[1].String(), args[2].String(), args[3].String()
	onHead, onChunk, onEnd := args[4], args[5], args[6]

	h := app.Handler()
	mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	cancels[id] = cancel
	mu.Unlock()

	go func() {
		defer func() {
			mu.Lock()
			delete(cancels, id)
			mu.Unlock()
			cancel()
			if r := recover(); r != nil {
				onEnd.Invoke(fmt.Sprint(r))
			}
		}()
		if h == nil {
			onEnd.Invoke("the dataset is not loaded yet")
			return
		}
		req, err := http.NewRequestWithContext(ctx, method, "http://glucolens.local"+path, strings.NewReader(body))
		if err != nil {
			onEnd.Invoke(err.Error())
			return
		}
		req.Header.Set("Content-Type", "application/json")
		w := &streamWriter{header: http.Header{}, onHead: onHead, onChunk: onChunk}
		h.ServeHTTP(w, req)
		w.finish()
		onEnd.Invoke(js.Null())
	}()
	return nil
}

// abort(id) cancels a running request (for example a chat the user stopped).
func abort(_ js.Value, args []js.Value) any {
	mu.Lock()
	cancel := cancels[args[0].Int()]
	mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// streamWriter forwards a handler's response to JavaScript as it is written.
type streamWriter struct {
	header  http.Header
	status  int
	sent    bool
	onHead  js.Value
	onChunk js.Value
	buf     bytes.Buffer
}

func (w *streamWriter) Header() http.Header { return w.header }

func (w *streamWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *streamWriter) flushHead() {
	if w.sent {
		return
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.sent = true
	w.onHead.Invoke(w.status, w.header.Get("Content-Type"))
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.flushHead()
	w.buf.Write(p)
	return len(p), nil
}

// Flush sends what has been written so far (the chat flushes per event).
func (w *streamWriter) Flush() {
	w.flushHead()
	if w.buf.Len() > 0 {
		w.onChunk.Invoke(w.buf.String())
		w.buf.Reset()
	}
}

// The final flush happens when the handler returns.
func (w *streamWriter) finish() { w.Flush() }
