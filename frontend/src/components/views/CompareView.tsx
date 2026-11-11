import { useQuery } from '@tanstack/react-query'
import { StackedShareBars } from '@/components/charts/Bars'
import { BoxPlots, RateRows } from '@/components/charts/Rows'
import { DataTable } from '@/components/charts/core'
import { StrengthBadge } from '@/components/ui/primitives'
import { ViewFrame, QueryGate } from './Frame'
import { api } from '@/lib/api'
import { fmtNumber, fmtPValue, fmtPct, fmtValue } from '@/lib/format'
import { keepPreviousData, useColumns, useSchema } from '@/lib/schema'
import type { CompareRequest, CompareResponse, Column } from '@/lib/types'

/** A metric across the groups of another column. */
export function CompareView({ request, you, title, subtitle }: {
  request: CompareRequest
  you?: number
  title?: React.ReactNode
  subtitle?: React.ReactNode
}) {
  const { data: schema } = useSchema()
  const cols = useColumns(schema)
  const q = useQuery({ queryKey: ['compare', request], queryFn: () => api.compare(request), placeholderData: keepPreviousData })
  const metric = cols.get(request.metric)
  return (
    <QueryGate isPending={q.isPending || !metric} error={q.error} refetch={q.refetch}>
      {() => (
        <ViewFrame
          title={title}
          subtitle={subtitle}
          isFetching={q.isFetching}
          table={<CompareTable d={q.data!} metric={metric!} />}
          footer={<CompareFooter d={q.data!} />}
        >
          <CompareChart d={q.data!} metric={metric!} you={you} />
        </ViewFrame>
      )}
    </QueryGate>
  )
}

export function CompareChart({ d, metric, you }: { d: CompareResponse; metric: Column; you?: number }) {
  if (d.mode === 'numeric') {
    return <BoxPlots rows={d.groups.map((g) => ({ label: g.label, summary: g.summary, lowN: g.lowN }))} column={metric} you={you} />
  }
  if (d.mode === 'rate') {
    return (
      <RateRows
        levels={d.groups.map((g) => ({ key: g.key, label: g.label, rate: g.rate!, difference: 0, lowN: g.lowN }))}
        base={d.overall.rate?.value}
        showRR={false}
      />
    )
  }
  return <StackedShareBars groups={d.groups.map((g) => ({ label: g.label, n: g.n, shares: g.shares ?? [], lowN: g.lowN }))} levels={d.levels ?? []} />
}

function CompareFooter({ d }: { d: CompareResponse }) {
  return (
    <div className="space-y-1.5 text-xs text-ink-2">
      <p className="text-[13px] leading-relaxed text-ink">{d.summary}</p>
      <p className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <StrengthBadge strength={d.test.strength} />
        <span>
          {d.test.name}, {fmtPValue(d.test.pValue)}
        </span>
        <span>
          {d.test.effectName} {d.test.effect.toFixed(2)}
        </span>
        <span className="tabular">{fmtNumber(d.n)} people</span>
      </p>
    </div>
  )
}

function CompareTable({ d, metric }: { d: CompareResponse; metric: Column }) {
  if (d.mode === 'numeric') {
    return (
      <DataTable
        caption={`${d.metricLabel} by ${d.groupByLabel}`}
        columns={[{ label: d.groupByLabel }, { label: 'People', align: 'right' }, { label: 'Median', align: 'right' }, { label: 'Mean', align: 'right' }, { label: '25th–75th', align: 'right' }, { label: '5th–95th', align: 'right' }]}
        rows={d.groups.map((g) => [
          g.label,
          fmtNumber(g.n),
          fmtValue(metric, g.summary?.median),
          fmtValue(metric, g.summary?.mean),
          g.summary ? `${fmtValue(metric, g.summary.p25)}–${fmtValue(metric, g.summary.p75)}` : '–',
          g.summary ? `${fmtValue(metric, g.summary.p5)}–${fmtValue(metric, g.summary.p95)}` : '–',
        ])}
      />
    )
  }
  if (d.mode === 'rate') {
    return (
      <DataTable
        caption={`${d.metricLabel} by ${d.groupByLabel}`}
        columns={[{ label: d.groupByLabel }, { label: 'People', align: 'right' }, { label: 'Rate', align: 'right' }, { label: '95% CI', align: 'right' }]}
        rows={d.groups.map((g) => [g.label, fmtNumber(g.n), fmtPct(g.rate?.value), g.rate ? `${fmtPct(g.rate.low)}–${fmtPct(g.rate.high)}` : '–'])}
      />
    )
  }
  return (
    <DataTable
      caption={`${d.metricLabel} by ${d.groupByLabel}`}
      columns={[{ label: d.groupByLabel }, { label: 'People', align: 'right' }, ...(d.levels ?? []).map((l) => ({ label: l, align: 'right' as const }))]}
      rows={d.groups.map((g) => [g.label, fmtNumber(g.n), ...(g.shares ?? []).map((s) => fmtPct(s))])}
    />
  )
}
