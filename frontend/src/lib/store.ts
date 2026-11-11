import { create } from 'zustand'
import { createJSONStorage, persist, type StateStorage } from 'zustand/middleware'
import type { ChatEvent, Values, View } from './types'

// Browser storage can be missing or throw (private mode, blocked site data).
const safeStorage: StateStorage = {
  getItem: (name) => {
    try {
      return window.localStorage.getItem(name)
    } catch {
      return null
    }
  },
  setItem: (name, value) => {
    try {
      window.localStorage.setItem(name, value)
    } catch {
      /* not persisted */
    }
  },
  removeItem: (name) => {
    try {
      window.localStorage.removeItem(name)
    } catch {
      /* ignore */
    }
  },
}

export type ThemePref = 'system' | 'light' | 'dark'

interface SettingsState {
  values: Values
  useValues: boolean
  theme: ThemePref
  /** Static site only: the visitor's own Groq key, kept in this browser. */
  groqKey: string
  groqModel: string
  /** Static site only: an alternative OpenAI-compatible endpoint (tests). */
  groqBaseURL: string
  setGroq: (key: string, model: string) => void
  setValue: (column: string, value: number | string | undefined) => void
  setValues: (values: Values) => void
  clearValues: () => void
  setUseValues: (on: boolean) => void
  setTheme: (theme: ThemePref) => void
}

/** The person's own checkup values and preferences, kept in this browser only. */
export const useSettings = create<SettingsState>()(
  persist(
    (set) => ({
      values: {},
      useValues: true,
      theme: 'system',
      groqKey: '',
      groqModel: 'openai/gpt-oss-120b',
      groqBaseURL: '',
      setGroq: (groqKey, groqModel) => set({ groqKey, groqModel }),
      setValue: (column, value) =>
        set((s) => {
          const next = { ...s.values }
          if (value === undefined || value === '' || (typeof value === 'number' && !Number.isFinite(value))) {
            delete next[column]
          } else {
            next[column] = value
          }
          return { values: next }
        }),
      setValues: (values) => set({ values }),
      clearValues: () => set({ values: {} }),
      setUseValues: (useValues) => set({ useValues }),
      setTheme: (theme) => set({ theme }),
    }),
    { name: 'glucolens-settings', storage: createJSONStorage(() => safeStorage), version: 1 },
  ),
)

interface UiState {
  valuesOpen: boolean
  openValues: (open: boolean) => void
}

export const useUi = create<UiState>()((set) => ({
  valuesOpen: false,
  openValues: (valuesOpen) => set({ valuesOpen }),
}))

// --- Conversation (kept in memory so it survives switching tabs) -----------

export interface ToolStep {
  id: string
  name: string
  label: string
  status: 'running' | 'done' | 'failed'
  error?: string
  view?: View
  result?: Record<string, unknown>
}

export interface Turn {
  id: string
  question: string
  segments: string[]
  segmentOpen: boolean
  steps: ToolStep[]
  status: 'streaming' | 'done' | 'error'
  error?: { code: string; message: string }
  startedAt: number
  elapsedMs?: number
}

interface ChatState {
  sessionId?: string
  turns: Turn[]
  addTurn: (turn: Turn) => void
  apply: (turnId: string, event: ChatEvent) => void
  fail: (turnId: string, error: { code: string; message: string }) => void
  reset: () => void
}

function updateTurn(turns: Turn[], id: string, fn: (t: Turn) => Turn): Turn[] {
  return turns.map((t) => (t.id === id ? fn(t) : t))
}

/** Folds one stream event into a turn. Exported for tests. */
export function reduceTurn(t: Turn, ev: ChatEvent): Turn {
  switch (ev.type) {
    case 'delta': {
      const segments = [...t.segments]
      if (t.segmentOpen && segments.length > 0) {
        segments[segments.length - 1] += ev.data.text
      } else {
        segments.push(ev.data.text)
      }
      return { ...t, segments, segmentOpen: true }
    }
    case 'message': {
      // The complete text replaces what was streamed for this part.
      const segments = [...t.segments]
      if (t.segmentOpen && segments.length > 0) segments[segments.length - 1] = ev.data.text
      else segments.push(ev.data.text)
      return { ...t, segments, segmentOpen: false }
    }
    case 'tool_call':
      return {
        ...t,
        segmentOpen: false,
        steps: [...t.steps, { id: ev.data.id, name: ev.data.name, label: ev.data.label, status: 'running' }],
      }
    case 'tool_result':
      return {
        ...t,
        steps: t.steps.map((s) =>
          s.id === ev.data.id
            ? {
                ...s,
                status: ev.data.ok ? 'done' : 'failed',
                error: ev.data.error,
                view: ev.data.view,
                result: ev.data.result,
              }
            : s,
        ),
      }
    case 'error':
      return { ...t, status: 'error', error: ev.data }
    case 'done':
      return {
        ...t,
        status: t.status === 'error' ? 'error' : 'done',
        segmentOpen: false,
        elapsedMs: ev.data.elapsedMs ?? Date.now() - t.startedAt,
        steps: t.steps.map((s) => (s.status === 'running' ? { ...s, status: 'failed' } : s)),
      }
    default:
      return t
  }
}

export const useChat = create<ChatState>()((set) => ({
  sessionId: undefined,
  turns: [],
  addTurn: (turn) => set((s) => ({ turns: [...s.turns, turn] })),
  apply: (turnId, event) =>
    set((s) => {
      if (event.type === 'session') return { sessionId: event.data.sessionId }
      return { turns: updateTurn(s.turns, turnId, (t) => reduceTurn(t, event)) }
    }),
  fail: (turnId, error) =>
    set((s) => ({
      turns: updateTurn(s.turns, turnId, (t) => ({
        ...t,
        status: 'error',
        error,
        segmentOpen: false,
        steps: t.steps.map((st) => (st.status === 'running' ? { ...st, status: 'failed' } : st)),
      })),
    })),
  reset: () => set({ sessionId: undefined, turns: [] }),
}))
