import { useQuery } from '@tanstack/react-query'
import CountUp from '@/components/reactbits/CountUp'
import { Histogram } from '@/components/charts/Histogram'
import { LevelBars, MeterList, StackedShareBars } from '@/components/charts/Bars'
import { DataTable } from '@/components/charts/core'
import { ViewFrame, QueryGate } from './Frame'
import { api } from '@/lib/api'
import { fmtNumber, fmtPct, fmtValue, lowerFirst } from '@/lib/format'
import { keepPreviousData, useColumns, useSchema } from '@/lib/schema'
import { STATUS } from '@/lib/palette'
import type { Column, DistributionRequest, DistributionResponse } from '@/lib/types'

/** How a column's values are spread, chosen by the column's kind. */
export function DistributionView({ request, you, title, subtitle, actions, compact }: {
  request: DistributionRequest
  you?: number | string
  title?: React.ReactNode
  subtitle?: React.ReactNode
  actions?: React.ReactNode
  compact?: boolean
}) {
  const { data: schema } = useSchema()
  const cols = useColumns(schema)
  const q = useQuery({
    queryKey: ['distribution', request],
    queryFn: () => api.distribution(request),
    placeholderData: keepPreviousData,
  })
  const column = cols.get(request.column)

  return (
    <QueryGate isPending={q.isPending || !column} error={q.error} refetch={q.refetch} minHeight={compact ? 120 : 180}>
      {() => {
        const d = q.data!
        const col = column!
        return (
          <ViewFrame
            title={title}
            subtitle={subtitle}
            actions={actions}
            isFetching={q.isFetching}
            table={<DistributionTable d={d} column={col} />}
            footer={<DistributionFooter d={d} column={col} />}
          >
            <DistributionChart d={d} column={col} you={you} compact={compact} />
          </ViewFrame>
        )
      }}
    </QueryGate>
  )
}

function youLevel(column: Column, you: number | string | undefined): string | undefined {
  if (you === undefined) return undefined
  if (typeof you === 'string') return you
  if (column.kind === 'binary') return column.levels?.[you >= 0.5 ? 1 : 0]
  if (column.kind === 'discrete') return String(you)
  return undefined
}

export function DistributionChart({ d, column, you, compact }: { d: DistributionResponse; column: Column; you?: number | string; compact?: boolean }) {
  if (d.mode === 'histogram') {
    return (
      <Histogram
        column={column}
        bins={d.bins!}
        overall={d.overall}
        groups={d.groups}
        bands={column.bands}
        you={typeof you === 'number' ? you : undefined}
        height={compact ? 180 : 220}
      />
    )
  }
  const levels = d.levels ?? []
  const mine = youLevel(column, you)
  if (column.kind === 'binary') {
    const yes = levels[1]
    if (d.groups && d.groups.length > 0) {
      return (
        <div>
          <p className="mb-3 text-xs text-ink-2">
            Share with {lowerFirst(column.label)}: {yes}
          </p>
          <MeterList
            rows={d.groups.map((g) => ({ label: g.label, value: g.shares[1] ?? 0, n: g.n, lowN: g.lowN }))}
            scaleMax={1}
          />
        </div>
      )
    }
    const share = d.overall.shares[1] ?? 0
    return (
      <div className="flex flex-col gap-3 py-2">
        <div className="flex items-baseline gap-2">
          <span className="text-4xl font-semibold tracking-tight text-ink">
            <CountUp to={Number((share * 100).toFixed(1))} decimals={1} duration={0.9} suffix="%" />
          </span>
          <span className="text-sm text-ink-2">answered “{yes}”</span>
        </div>
        <MeterList
          rows={[
            { label: levels[1], value: share, n: d.overall.counts[1] },
            { label: levels[0], value: d.overall.shares[0] ?? 0, n: d.overall.counts[0] },
          ]}
          scaleMax={1}
          highlight={mine}
        />
      </div>
    )
  }
  if (d.groups && d.groups.length > 0) {
    return <StackedShareBars groups={d.groups.map((g) => ({ label: g.label, n: g.n, shares: g.shares, lowN: g.lowN }))} levels={levels} />
  }
  return <LevelBars levels={levels} shares={d.overall.shares} counts={d.overall.counts} highlight={mine} />
}

function DistributionFooter({ d, column }: { d: DistributionResponse; column: Column }) {
  const s = d.overall.summary
  return (
    <div className="space-y-1.5 text-xs text-ink-2">
      <p>
        {d.cohort === 'Everyone' ? 'Everyone' : d.cohort} · <span className="tabular">{fmtNumber(d.n)}</span> people
        {s && column.kind !== 'binary' && (
          <>
            {' '}
            · median <span className="tabular font-medium text-ink">{fmtValue(column, s.median)}</span> · middle half{' '}
            <span className="tabular">
              {fmtValue(column, s.p25)}–{fmtValue(column, s.p75)}
            </span>
          </>
        )}
      </p>
      {d.bands && d.bands.length > 0 && (
        <p className="flex flex-wrap gap-x-3 gap-y-1">
          {d.bands.map((b) => (
            <span key={b.label} className="inline-flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ background: STATUS[b.tone] }} aria-hidden="true" />
              {b.label} <span className="tabular font-medium text-ink">{fmtPct(b.share)}</span>
            </span>
          ))}
        </p>
      )}
    </div>
  )
}

function DistributionTable({ d, column }: { d: DistributionResponse; column: Column }) {
  const groups = d.groups ?? []
  const labels =
    d.mode === 'histogram'
      ? d.bins!.map((b) => `${fmtValue(column, b.start)} – ${fmtValue(column, b.end)}`)
      : (d.levels ?? [])
  const head = [{ label: column.label }, { label: 'People', align: 'right' as const }, { label: 'Share', align: 'right' as const }, ...groups.map((g) => ({ label: g.label, align: 'right' as const }))]
  const rows = labels.map((l, i) => [
    l,
    fmtNumber(d.overall.counts[i]),
    fmtPct(d.overall.shares[i]),
    ...groups.map((g) => fmtPct(g.shares[i] ?? 0)),
  ])
  return <DataTable caption={`Distribution of ${column.label}`} columns={head} rows={rows} />
}
