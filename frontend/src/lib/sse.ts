import { ApiError, errorFromResponse, UNREACHABLE } from './api'
import { apiFetch } from './runtime'
import type { ChatEvent, Values } from './types'

export interface RawEvent {
  type: string
  data: string
}

/**
 * Splits a server-sent-events buffer into complete events. Returns the
 * parsed events and the unfinished tail to keep for the next chunk.
 */
export function parseSSE(buffer: string): { events: RawEvent[]; rest: string } {
  const events: RawEvent[] = []
  const normalized = buffer.replace(/\r\n/g, '\n')
  const blocks = normalized.split('\n\n')
  const rest = blocks.pop() ?? ''
  for (const block of blocks) {
    let type = 'message'
    const data: string[] = []
    for (const line of block.split('\n')) {
      if (line === '' || line.startsWith(':')) continue
      const colon = line.indexOf(':')
      const field = colon === -1 ? line : line.slice(0, colon)
      let value = colon === -1 ? '' : line.slice(colon + 1)
      if (value.startsWith(' ')) value = value.slice(1)
      if (field === 'event') type = value
      else if (field === 'data') data.push(value)
    }
    if (data.length > 0) events.push({ type, data: data.join('\n') })
  }
  return { events, rest }
}

export interface ChatRequestBody {
  sessionId?: string
  message: string
  values?: Values
  useValues?: boolean
}

/** Posts a question and calls onEvent for every streamed event. */
export async function streamChat(
  body: ChatRequestBody,
  onEvent: (event: ChatEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  let res: Response
  try {
    res = await apiFetch('/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
      body: JSON.stringify(body),
      signal,
    })
  } catch (err) {
    if ((err as Error).name === 'AbortError') throw err
    const detail = err instanceof Error && err.message ? ` (${err.message})` : ''
    throw new ApiError(0, 'api_unreachable', UNREACHABLE + detail)
  }
  if (!res.ok) throw await errorFromResponse(res)
  if (!res.body) throw new ApiError(0, 'no_stream', 'The server returned no answer stream.')

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const { events, rest } = parseSSE(buffer)
    buffer = rest
    for (const ev of events) {
      try {
        onEvent({ type: ev.type, data: JSON.parse(ev.data) } as ChatEvent)
      } catch {
        // Ignore a malformed event rather than abort the whole answer.
      }
    }
  }
  buffer += decoder.decode()
  const { events } = parseSSE(buffer + '\n\n')
  for (const ev of events) {
    try {
      onEvent({ type: ev.type, data: JSON.parse(ev.data) } as ChatEvent)
    } catch {
      /* ignore */
    }
  }
}
