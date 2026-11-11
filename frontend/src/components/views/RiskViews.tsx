import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { RankBars, RateRows, rateDomain } from '@/components/charts/Rows'
import { DataTable } from '@/components/charts/core'
import { StrengthBadge } from '@/components/ui/primitives'
import { ViewFrame, QueryGate } from './Frame'
import { api } from '@/lib/api'
import { fmtNumber, fmtPValue, fmtPct } from '@/lib/format'
import { keepPreviousData } from '@/lib/schema'
import type { RankRequest, RankResponse, RiskRequest, RiskResponse } from '@/lib/types'

export function useRisk(request: RiskRequest | null) {
  return useQuery({
    queryKey: ['risk', request],
    queryFn: () => api.risk(request!),
    enabled: !!request,
    placeholderData: keepPreviousData,
  })
}

export function useRank(request: RankRequest | null) {
  return useQuery({
    queryKey: ['rank', request],
    queryFn: () => api.rank(request!),
    enabled: !!request,
    placeholderData: keepPreviousData,
  })
}

/** Outcome rate by factor group with CIs, relative risk and a verdict. */
export function RiskView({ request, title, subtitle, showStrata = true }: {
  request: RiskRequest
  title?: React.ReactNode
  subtitle?: React.ReactNode
  showStrata?: boolean
}) {
  const q = useRisk(request)
  return (
    <QueryGate isPending={q.isPending} error={q.error} refetch={q.refetch}>
      {() => <RiskBody d={q.data!} isFetching={q.isFetching} title={title} subtitle={subtitle} showStrata={showStrata} />}
    </QueryGate>
  )
}

export function RiskBody({ d, isFetching, title, subtitle, showStrata = true }: {
  d: RiskResponse
  isFetching?: boolean
  title?: React.ReactNode
  subtitle?: React.ReactNode
  showStrata?: boolean
}) {
  // One shared axis so the strata compare with each other.
  const strataDomain = d.strata && d.strata.length > 0 ? rateDomain(d.strata.flatMap((s) => s.levels)) : undefined
  return (
    <ViewFrame
      title={title}
      subtitle={subtitle}
      isFetching={isFetching}
      table={<RiskTable d={d} />}
      footer={
        <div className="space-y-2 text-xs text-ink-2">
          <p className="text-[13px] leading-relaxed text-ink">{d.verdict}</p>
          <p className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <StrengthBadge strength={d.association.strength} />
            <span>Chi-square {fmtPValue(d.association.pValue)}</span>
            <span>Cramér's V {d.association.cramersV.toFixed(2)}</span>
            <span className="tabular">{fmtNumber(d.n)} people</span>
          </p>
        </div>
      }
    >
      <RateRows levels={d.levels} base={d.base.value} />
      {showStrata && d.strata && d.strata.length > 0 && (
        <div className="mt-5">
          <p className="mb-2 text-xs font-medium text-ink-2">
            Within each {d.stratifyByLabel} group, to check whether {d.stratifyByLabel?.toLowerCase()} explains the difference
          </p>
          <div className="grid gap-3 sm:grid-cols-2">
            {d.strata.map((s) => (
              <div key={s.key} className="rounded-2xl bg-surface-2 p-3 ring-1 ring-line">
                <div className="mb-1.5 flex items-center justify-between gap-2 text-xs">
                  <span className="font-semibold text-ink">{s.label}</span>
                  <StrengthBadge strength={s.association.strength} />
                </div>
                <RateRows levels={s.levels} base={s.base.value} compact domain={strataDomain} />
              </div>
            ))}
          </div>
        </div>
      )}
    </ViewFrame>
  )
}

function RiskTable({ d }: { d: RiskResponse }) {
  return (
    <DataTable
      caption={`${d.outcome.label} by ${d.factorLabel}`}
      columns={[
        { label: d.factorLabel },
        { label: 'People', align: 'right' },
        { label: 'With outcome', align: 'right' },
        { label: 'Rate', align: 'right' },
        { label: '95% CI', align: 'right' },
        { label: 'Relative risk', align: 'right' },
      ]}
      rows={d.levels.map((l) => [
        l.label + (l.isReference ? ' (reference)' : ''),
        fmtNumber(l.rate.n),
        fmtNumber(l.rate.events),
        fmtPct(l.rate.value),
        `${fmtPct(l.rate.low)}–${fmtPct(l.rate.high)}`,
        l.relativeRisk ? `${l.relativeRisk.value.toFixed(2)} (${l.relativeRisk.low.toFixed(2)}–${l.relativeRisk.high.toFixed(2)})` : l.isReference ? '1.00' : '–',
      ])}
    />
  )
}

/** Factors ranked by how strongly they relate to the outcome. */
export function RankView({ request, selected, onSelect, limit, title, subtitle }: {
  request: RankRequest
  selected?: string
  onSelect?: (column: string) => void
  limit?: number
  title?: React.ReactNode
  subtitle?: React.ReactNode
}) {
  const q = useRank(request)
  return (
    <QueryGate isPending={q.isPending} error={q.error} refetch={q.refetch}>
      {() => <RankBody d={q.data!} isFetching={q.isFetching} selected={selected} onSelect={onSelect} limit={limit} title={title} subtitle={subtitle} />}
    </QueryGate>
  )
}

export function RankBody({ d, isFetching, selected, onSelect, limit, initial, title, subtitle }: {
  d: RankResponse
  isFetching?: boolean
  selected?: string
  onSelect?: (column: string) => void
  limit?: number
  initial?: number
  title?: React.ReactNode
  subtitle?: React.ReactNode
}) {
  const [all, setAll] = useState(false)
  const cap = limit ?? (initial && !all ? initial : undefined)
  const visible = cap ? d.factors.filter((f, i) => i < cap || f.column === selected) : d.factors
  const factors = visible.map((f) => ({
    column: f.column,
    label: f.label,
    v: f.association.cramersV,
    strength: f.association.strength,
    detail:
      f.association.strength === 'none'
        ? `No meaningful difference (${fmtPct(f.lowest.rate)}–${fmtPct(f.highest.rate)})`
        : `${f.highest.label}: ${fmtPct(f.highest.rate)} · ${f.lowest.label}: ${fmtPct(f.lowest.rate)}`,
  }))
  return (
    <ViewFrame
      title={title}
      subtitle={subtitle}
      isFetching={isFetching}
      table={
        <DataTable
          caption={`Factors linked to ${d.outcome.label}`}
          columns={[{ label: 'Factor' }, { label: "Cramér's V", align: 'right' }, { label: 'Strength' }, { label: 'Highest group' }, { label: 'Lowest group' }]}
          rows={d.factors.map((f) => [
            f.label,
            f.association.cramersV.toFixed(3),
            f.association.strength,
            `${f.highest.label} ${fmtPct(f.highest.rate)}`,
            `${f.lowest.label} ${fmtPct(f.lowest.rate)}`,
          ])}
        />
      }
    >
      <RankBars factors={factors} selected={selected} onSelect={onSelect} />
      {!limit && initial && d.factors.length > initial && (
        <button type="button" onClick={() => setAll(!all)} className="mt-2 w-full rounded-xl py-2 text-sm font-medium text-accent hover:bg-surface-2">
          {all ? 'Show fewer' : `Show all ${d.factors.length} factors`}
        </button>
      )}
    </ViewFrame>
  )
}
