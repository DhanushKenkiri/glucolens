import type { Filter, Outcome, Schema, View } from './types'

export function encodeFilters(filters: Filter[] | undefined): string | undefined {
  if (!filters || filters.length === 0) return undefined
  return JSON.stringify(filters)
}

export function decodeFilters(raw: string | null): Filter[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((f) => f && typeof f.column === 'string')
  } catch {
    return []
  }
}

/** Outcomes are a preset id in the URL, or JSON for a custom definition. */
export function encodeOutcome(o: Outcome): string {
  if (o.preset) return o.preset
  return JSON.stringify({ column: o.column, equals: o.equals, min: o.min, max: o.max })
}

export function decodeOutcome(raw: string | null, fallback: Outcome): Outcome {
  if (!raw) return fallback
  if (raw.startsWith('{')) {
    try {
      const o = JSON.parse(raw)
      if (typeof o.column === 'string') return o
    } catch {
      /* fall through */
    }
    return fallback
  }
  return { preset: raw }
}

function qs(params: Record<string, string | undefined>) {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '') sp.set(k, v)
  const s = sp.toString()
  return s ? `?${s}` : ''
}

export function insightsLink(p: { column?: string; split?: string; filters?: Filter[]; schema?: Schema }) {
  const section = p.column ? p.schema?.columns.find((c) => c.name === p.column)?.section : undefined
  return `/insights${qs({ section, col: p.column, split: p.split, f: encodeFilters(p.filters) })}`
}

export function riskLink(p: { outcome: Outcome; factor?: string; reference?: string; stratifyBy?: string; filters?: Filter[]; markers?: boolean }) {
  return `/risk${qs({
    o: encodeOutcome(p.outcome),
    factor: p.factor,
    ref: p.reference,
    strat: p.stratifyBy,
    f: encodeFilters(p.filters),
    markers: p.markers ? '1' : undefined,
  })}`
}

/** Where an assistant evidence card should lead. */
export function linkForView(view: View, schema?: Schema): { to: string; label: string } {
  switch (view.kind) {
    case 'distribution':
      return { to: insightsLink({ column: view.request.column, split: view.request.splitBy, filters: view.request.filters, schema }), label: 'Open in Insights' }
    case 'compare':
      return { to: insightsLink({ column: view.request.metric, split: view.request.groupBy, filters: view.request.filters, schema }), label: 'Open in Insights' }
    case 'risk':
      return {
        to: riskLink({
          outcome: view.request.outcome,
          factor: view.request.factor,
          reference: view.request.reference,
          stratifyBy: view.request.stratifyBy,
          filters: view.request.filters,
        }),
        label: 'Open in Risk Explorer',
      }
    case 'rank':
      return { to: riskLink({ outcome: view.request.outcome, filters: view.request.filters, markers: view.request.includeMarkers }), label: 'Open in Risk Explorer' }
    case 'profile':
      return { to: '/insights', label: 'Open in Insights' }
    case 'similar':
      return { to: '/risk#your-picture', label: 'Open in Risk Explorer' }
  }
}
