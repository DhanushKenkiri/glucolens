/* global Go, glucolens */
// Runs one GlucoLens WebAssembly build (the Go API) inside a Web Worker for
// the static site. Messages from the page:
//   {type:'init', wasm, data}            load the binary and dataset.json
//   {type:'configure', id, apiKey, model, baseURL}
//   {type:'fetch', id, method, path, body}
//   {type:'abort', id}
self.importScripts('wasm_exec.js')

let ready = null

async function instantiate(url, go) {
  const res = await fetch(url)
  if (!res.ok) throw new Error(`could not download ${url} (${res.status})`)
  if (WebAssembly.instantiateStreaming && (res.headers.get('Content-Type') || '').includes('application/wasm')) {
    return WebAssembly.instantiateStreaming(res, go.importObject)
  }
  return WebAssembly.instantiate(await res.arrayBuffer(), go.importObject)
}

async function load(wasmURL, dataURL) {
  const go = new Go()
  const [wasm, data] = await Promise.all([
    instantiate(wasmURL, go),
    fetch(dataURL).then((r) => {
      if (!r.ok) throw new Error(`could not download the dataset (${r.status})`)
      return r.arrayBuffer()
    }),
  ])
  go.run(wasm.instance)
  return glucolens.load(new Uint8Array(data))
}

self.onmessage = async (event) => {
  const m = event.data
  if (m.type === 'init') {
    ready = load(m.wasm, m.data)
    ready.then(
      (info) => self.postMessage({ type: 'ready', info }),
      (err) => self.postMessage({ type: 'failed', error: String((err && err.message) || err) }),
    )
    return
  }
  try {
    await ready
  } catch (err) {
    const error = `GlucoLens could not load in this browser: ${(err && err.message) || err}`
    if (m.type === 'configure') self.postMessage({ type: 'configured', id: m.id, error })
    else if (m.type === 'fetch') self.postMessage({ type: 'end', id: m.id, error })
    return
  }
  switch (m.type) {
    case 'configure':
      try {
        const result = await glucolens.configure({ apiKey: m.apiKey || '', model: m.model || '', baseURL: m.baseURL || '' })
        self.postMessage({ type: 'configured', id: m.id, result })
      } catch (err) {
        self.postMessage({ type: 'configured', id: m.id, error: String((err && err.message) || err) })
      }
      break
    case 'fetch':
      glucolens.fetch(
        m.id,
        m.method,
        m.path,
        m.body || '',
        (status, contentType) => self.postMessage({ type: 'head', id: m.id, status, contentType }),
        (text) => self.postMessage({ type: 'chunk', id: m.id, text }),
        (error) => self.postMessage({ type: 'end', id: m.id, error: error || null }),
      )
      break
    case 'abort':
      glucolens.abort(m.id)
      break
  }
}
