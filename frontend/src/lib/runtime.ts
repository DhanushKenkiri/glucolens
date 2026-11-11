import { useSyncExternalStore } from 'react'
import { useSettings } from './store'

/**
 * Where the API runs.
 *
 * - Server mode (default): requests go to the Go server at /api/v1.
 * - Static mode (VITE_STATIC=1, the GitHub Pages build): the same Go code runs
 *   in this browser as WebAssembly inside Web Workers. glucolens-core.wasm
 *   serves the data API; glucolens-agent.wasm adds the ADK agent and loads
 *   only when someone asks a question with their own Groq key. Responses
 *   stream back from the worker and are wrapped in ordinary fetch Responses,
 *   so the rest of the app cannot tell the difference.
 */
export const STATIC = import.meta.env.VITE_STATIC === '1'
export const API_BASE = '/api/v1'
const BASE = import.meta.env.BASE_URL

export const GROQ_MODELS = [
  { value: 'openai/gpt-oss-120b', label: 'GPT-OSS 120B (best answers)' },
  { value: 'openai/gpt-oss-20b', label: 'GPT-OSS 20B (faster, higher limits)' },
  { value: 'llama-3.3-70b-versatile', label: 'Llama 3.3 70B' },
]

type Phase = 'idle' | 'loading' | 'ready' | 'failed'
export interface RuntimeStatus {
  data: Phase
  agent: Phase
  error?: string
}

let status: RuntimeStatus = { data: STATIC ? 'loading' : 'ready', agent: 'idle' }
const listeners = new Set<() => void>()
function setStatus(patch: Partial<RuntimeStatus>) {
  status = { ...status, ...patch }
  listeners.forEach((l) => l())
}

/** Loading state of the in-browser runtime (static mode). */
export function useRuntimeStatus(): RuntimeStatus {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    () => status,
  )
}

interface Pending {
  head: (status: number, contentType: string) => void
  chunk: (text: string) => void
  end: (error: string | null) => void
}

class WasmWorker {
  private worker: Worker
  private pending = new Map<number, Pending>()
  private configured = new Map<number, { resolve: (v: unknown) => void; reject: (e: Error) => void }>()
  private seq = 0
  readonly ready: Promise<void>

  constructor(wasmFile: string) {
    this.worker = new Worker(`${BASE}glucolens-worker.js`)
    this.ready = new Promise((resolve, reject) => {
      this.worker.onmessage = (e: MessageEvent) => {
        const m = e.data
        switch (m.type) {
          case 'ready':
            resolve()
            break
          case 'failed':
            reject(new Error(m.error))
            break
          case 'head':
            this.pending.get(m.id)?.head(m.status, m.contentType)
            break
          case 'chunk':
            this.pending.get(m.id)?.chunk(m.text)
            break
          case 'end':
            this.pending.get(m.id)?.end(m.error ?? null)
            break
          case 'configured': {
            const c = this.configured.get(m.id)
            this.configured.delete(m.id)
            if (m.error) c?.reject(new Error(m.error))
            else c?.resolve(m.result)
            break
          }
        }
      }
      this.worker.onerror = (e) => reject(new Error(e.message || 'The WebAssembly worker failed to start.'))
    })
    this.worker.postMessage({ type: 'init', wasm: `${BASE}${wasmFile}`, data: `${BASE}dataset.json` })
  }

  configure(apiKey: string, model: string, baseURL: string): Promise<unknown> {
    const id = ++this.seq
    return new Promise((resolve, reject) => {
      this.configured.set(id, { resolve, reject })
      this.worker.postMessage({ type: 'configure', id, apiKey, model, baseURL })
    })
  }

  fetch(method: string, path: string, body: string, signal?: AbortSignal | null): Promise<Response> {
    const id = ++this.seq
    const encoder = new TextEncoder()
    return new Promise<Response>((resolve, reject) => {
      let controller!: ReadableStreamDefaultController<Uint8Array>
      let headed = false
      let done = false
      const stream = new ReadableStream<Uint8Array>({
        start: (c) => {
          controller = c
        },
        cancel: () => this.worker.postMessage({ type: 'abort', id }),
      })
      const finish = () => {
        done = true
        this.pending.delete(id)
        if (this.closing && this.pending.size === 0) this.worker.terminate()
      }
      this.pending.set(id, {
        head: (code, contentType) => {
          headed = true
          resolve(new Response(stream, { status: code, headers: { 'Content-Type': contentType || 'application/json' } }))
        },
        chunk: (text) => {
          try {
            controller.enqueue(encoder.encode(text))
          } catch {
            /* the reader went away */
          }
        },
        end: (error) => {
          finish()
          if (error) {
            if (!headed) reject(new TypeError(error))
            else controller.error(new Error(error))
            return
          }
          if (!headed) resolve(new Response(null, { status: 204 }))
          try {
            controller.close()
          } catch {
            /* already closed */
          }
        },
      })
      signal?.addEventListener('abort', () => {
        if (done) return
        this.worker.postMessage({ type: 'abort', id })
        const err = new DOMException('The request was stopped.', 'AbortError')
        finish()
        if (!headed) reject(err)
        else
          try {
            controller.error(err)
          } catch {
            /* closed */
          }
      })
      this.ready.then(
        () => this.worker.postMessage({ type: 'fetch', id, method, path, body }),
        (err) => {
          finish()
          reject(new TypeError(String(err?.message ?? err)))
        },
      )
    })
  }

  private closing = false

  /** Terminates once no request is in flight. */
  terminateWhenIdle() {
    this.closing = true
    if (this.pending.size === 0) this.worker.terminate()
  }
}

let core: WasmWorker | null = null
let agent: WasmWorker | null = null
let agentConfigured: Promise<unknown> | null = null
let agentReady = false

function startCore(): WasmWorker {
  if (!core) {
    core = new WasmWorker('glucolens-core.wasm')
    core.ready.then(
      () => setStatus({ data: 'ready' }),
      (err) => setStatus({ data: 'failed', error: String(err?.message ?? err) }),
    )
  }
  return core
}

function settings() {
  const s = useSettings.getState()
  return { key: s.groqKey.trim(), model: s.groqModel || GROQ_MODELS[0].value, baseURL: s.groqBaseURL.trim() }
}

/** Loads the agent build and gives it the visitor's key. */
function startAgent(): Promise<unknown> {
  if (!agent) {
    setStatus({ agent: 'loading' })
    agent = new WasmWorker('glucolens-agent.wasm')
    const a = agent
    agentConfigured = a.ready
      .then(() => {
        const { key, model, baseURL } = settings()
        return a.configure(key, model, baseURL)
      })
      .then((r) => {
        // The agent build also serves the data API, so the core can go.
        agentReady = true
        setStatus({ agent: 'ready', data: 'ready' })
        core?.terminateWhenIdle()
        core = null
        return r
      })
    agentConfigured.catch((err) => setStatus({ agent: 'failed', error: String(err?.message ?? err) }))
  }
  return agentConfigured!
}

/** Applies a new key or model to a running agent (static mode). */
export async function applyGroqSettings(): Promise<void> {
  if (!STATIC) return
  const { key, model, baseURL } = settings()
  if (!key) {
    if (agent && agentReady) await agent.configure('', model, baseURL)
    return
  }
  if (!agent) {
    await startAgent()
    return
  }
  await agentConfigured
  await agent.configure(key, model, baseURL)
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

const NO_KEY = 'Add your Groq API key to use the assistant on this site. It is kept in this browser and sent only to Groq.'

/** fetch() for API paths such as "/schema" or "/chat", in either mode. */
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  if (!STATIC) return fetch(API_BASE + path, init)
  const method = init?.method ?? 'GET'
  const body = typeof init?.body === 'string' ? init.body : ''

  if (path === '/chat') {
    if (!settings().key) {
      return json(503, { error: { code: 'assistant_unavailable', message: NO_KEY } })
    }
    try {
      await startAgent()
    } catch (err) {
      throw new TypeError(`The assistant could not load in this browser: ${String((err as Error)?.message ?? err)}`, { cause: err })
    }
    return agent!.fetch(method, API_BASE + path, body, init?.signal)
  }

  const target = agentReady && agent ? agent : startCore()
  const res = await target.fetch(method, API_BASE + path, body, init?.signal)
  if (path !== '/health' || !res.ok) return res
  // The assistant's readiness on the static site depends on the visitor's key.
  const health = await res.json()
  const { key, model } = settings()
  health.assistant = key ? { ready: true, provider: 'Groq', model } : { ready: false, provider: 'Groq', reason: NO_KEY }
  return json(200, health)
}

// Static mode: start loading the data as soon as the app starts; if a key is
// already saved, warm up the agent once the data API is ready.
if (STATIC && typeof window !== 'undefined') {
  startCore()
    .ready.then(() => {
      if (settings().key) startAgent().catch(() => {})
    })
    .catch(() => {})
}
