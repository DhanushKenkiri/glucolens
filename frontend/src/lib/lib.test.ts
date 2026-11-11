import { describe, expect, it } from 'vitest'
import { parseSSE } from './sse'
import { reduceTurn, type Turn } from './store'
import { fmtPct, fmtValue, lowerFirst, niceTicks, ordinal } from './format'
import { decodeFilters, decodeOutcome, encodeFilters, encodeOutcome, linkForView } from './links'
import { diverging, mix, sequential } from './palette'
import type { ChatEvent } from './types'

describe('parseSSE', () => {
  it('splits complete events and keeps the unfinished tail', () => {
    const buf = 'event: session\ndata: {"sessionId":"abc12345"}\n\nevent: delta\ndata: {"text":"Hel'
    const { events, rest } = parseSSE(buf)
    expect(events).toEqual([{ type: 'session', data: '{"sessionId":"abc12345"}' }])
    expect(rest).toBe('event: delta\ndata: {"text":"Hel')
  })

  it('handles CRLF, comments and multi-line data', () => {
    const { events, rest } = parseSSE(': ping\r\n\r\nevent: message\r\ndata: line1\r\ndata: line2\r\n\r\n')
    expect(events).toEqual([{ type: 'message', data: 'line1\nline2' }])
    expect(rest).toBe('')
  })

  it('reassembles an event split across chunks', () => {
    let buffer = ''
    const out: string[] = []
    for (const chunk of ['event: del', 'ta\ndata: {"te', 'xt":"Hi"}\n', '\n']) {
      buffer += chunk
      const { events, rest } = parseSSE(buffer)
      buffer = rest
      out.push(...events.map((e) => `${e.type}:${e.data}`))
    }
    expect(out).toEqual(['delta:{"text":"Hi"}'])
  })
})

describe('reduceTurn', () => {
  const base: Turn = { id: 't', question: 'q', segments: [], segmentOpen: false, steps: [], status: 'streaming', startedAt: 0 }
  const run = (events: ChatEvent[]) => events.reduce(reduceTurn, base)

  it('streams deltas, then replaces them with the final message', () => {
    const t = run([
      { type: 'delta', data: { text: 'Hb' } },
      { type: 'delta', data: { text: 'A1c is' } },
      { type: 'message', data: { text: 'HbA1c is higher.' } },
      { type: 'done', data: { elapsedMs: 1200 } },
    ])
    expect(t.segments).toEqual(['HbA1c is higher.'])
    expect(t.status).toBe('done')
    expect(t.elapsedMs).toBe(1200)
  })

  it('tracks tool calls and keeps text before and after them as separate parts', () => {
    const t = run([
      { type: 'message', data: { text: 'Let me check.' } },
      { type: 'tool_call', data: { id: 'c1', name: 'compare_groups', label: 'Comparing', args: {} } },
      { type: 'tool_result', data: { id: 'c1', name: 'compare_groups', ok: true, view: { kind: 'compare', request: { metric: 'hba1c', groupBy: 'age' } } } },
      { type: 'tool_call', data: { id: 'c2', name: 'analyze_risk', label: 'Checking', args: {} } },
      { type: 'tool_result', data: { id: 'c2', name: 'analyze_risk', ok: false, error: 'unknown factor' } },
      { type: 'delta', data: { text: 'Done.' } },
      { type: 'done', data: {} },
    ])
    expect(t.segments).toEqual(['Let me check.', 'Done.'])
    expect(t.steps.map((s) => s.status)).toEqual(['done', 'failed'])
    expect(t.steps[0].view?.kind).toBe('compare')
    expect(t.steps[1].error).toBe('unknown factor')
  })

  it('keeps the error status through done and closes running steps', () => {
    const t = run([
      { type: 'tool_call', data: { id: 'c1', name: 'x', label: 'x', args: {} } },
      { type: 'error', data: { code: 'rate_limited', message: 'slow down' } },
      { type: 'done', data: {} },
    ])
    expect(t.status).toBe('error')
    expect(t.error?.code).toBe('rate_limited')
    expect(t.steps[0].status).toBe('failed')
  })
})

describe('format', () => {
  it('formats values with units', () => {
    expect(fmtValue({ decimals: 2, unit: '%' }, 6.1)).toBe('6.1%')
    expect(fmtValue({ decimals: 0, unit: 'mmHg' }, 128)).toBe('128 mmHg')
    expect(fmtValue({ decimals: 1, unit: '/10' }, 7.25)).toBe('7.3/10')
    expect(fmtPct(0.6012)).toBe('60.1%')
  })
  it('orders, lowercases and ticks', () => {
    expect([1, 2, 3, 4, 11, 12, 13, 21, 62].map(ordinal)).toEqual(['1st', '2nd', '3rd', '4th', '11th', '12th', '13th', '21st', '62nd'])
    expect(lowerFirst('Diagnosed diabetes')).toBe('diagnosed diabetes')
    expect(lowerFirst('HbA1c')).toBe('HbA1c')
    expect(lowerFirst('BMI')).toBe('BMI')
    expect(niceTicks(4, 9.8, 5)).toEqual([4, 6, 8])
    expect(niceTicks(0, 0.62, 4)).toEqual([0, 0.2, 0.4, 0.6])
  })
})

describe('links', () => {
  it('round-trips filters and outcomes', () => {
    const f = [{ column: 'gender', equals: ['Female'] }, { column: 'age', min: 40, max: 50 }]
    expect(decodeFilters(encodeFilters(f)!)).toEqual(f)
    expect(decodeFilters('not json')).toEqual([])
    expect(decodeOutcome(encodeOutcome({ preset: 'obesity' }), { preset: 'x' })).toEqual({ preset: 'obesity' })
    expect(decodeOutcome(encodeOutcome({ column: 'hba1c', min: 6.5 }), { preset: 'x' })).toMatchObject({ column: 'hba1c', min: 6.5 })
    expect(decodeOutcome('{broken', { preset: 'x' })).toEqual({ preset: 'x' })
  })
  it('links evidence to the matching tab', () => {
    expect(linkForView({ kind: 'risk', request: { outcome: { preset: 'diagnosed_diabetes' }, factor: 'smoking_status' } }).to).toBe(
      '/risk?o=diagnosed_diabetes&factor=smoking_status',
    )
    expect(linkForView({ kind: 'compare', request: { metric: 'hba1c', groupBy: 'age' } }).to).toBe('/insights?col=hba1c&split=age')
  })
})

describe('palette', () => {
  it('interpolates in OKLab and anchors the scales', () => {
    expect(mix('#000000', '#ffffff', 0)).toBe('#000000')
    expect(mix('#000000', '#ffffff', 1)).toBe('#ffffff')
    expect(sequential('light', 0)).toBe('#cde2fb')
    expect(sequential('light', 1)).toBe('#0d366b')
    expect(sequential('dark', 0)).toBe('#0d366b')
    expect(diverging('light', 0)).toBe('#f0efec')
    expect(diverging('light', 1)).toBe('#e34948')
    expect(diverging('light', -1)).toBe('#2a78d6')
  })
})
