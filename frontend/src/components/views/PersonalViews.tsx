import { useQuery } from '@tanstack/react-query'
import { Dumbbells, PercentileStrips } from '@/components/charts/Rows'
import { DataTable } from '@/components/charts/core'
import { Note } from '@/components/ui/primitives'
import { ViewFrame, QueryGate } from './Frame'
import { api } from '@/lib/api'
import { fmtAny, fmtNumber, fmtPct, ordinal } from '@/lib/format'
import { keepPreviousData, useColumns, useSchema } from '@/lib/schema'
import type { ProfileRequest, SimilarRequest } from '@/lib/types'

/** Each of the person's values placed among a comparison group. */
export function ProfileView({ request, title, subtitle, onlyFlagged }: {
  request: ProfileRequest
  title?: React.ReactNode
  subtitle?: React.ReactNode
  onlyFlagged?: boolean
}) {
  const q = useQuery({ queryKey: ['profile', request], queryFn: () => api.profile(request), placeholderData: keepPreviousData })
  return (
    <QueryGate isPending={q.isPending} error={q.error} refetch={q.refetch} minHeight={80}>
      {() => {
        const d = q.data!
        const items = onlyFlagged ? d.items.filter((i) => i.tone === 'watch' || i.tone === 'alert') : d.items
        return (
          <ViewFrame
            title={title}
            subtitle={subtitle}
            isFetching={q.isFetching}
            table={
              <DataTable
                caption="Your values compared with the group"
                columns={[{ label: 'Value' }, { label: 'Yours', align: 'right' }, { label: 'Percentile', align: 'right' }, { label: 'Group median', align: 'right' }, { label: 'Clinical range' }]}
                rows={d.items.map((i) => [
                  i.label,
                  i.display,
                  i.percentile !== undefined ? ordinal(i.percentile) : i.share !== undefined ? `${fmtPct(i.share)} share it` : '–',
                  i.median !== undefined ? fmtNumber(i.median, 2) : '–',
                  i.band?.label ?? '–',
                ])}
              />
            }
            footer={<p className="text-xs text-ink-2">Compared with {d.cohort === 'Everyone' ? 'everyone' : d.cohort} · {fmtNumber(d.n)} people</p>}
          >
            {items.length > 0 ? (
              <PercentileStrips items={items} />
            ) : (
              <p className="py-3 text-sm text-ink-2">None of your values fall in a watch or alert range.</p>
            )}
          </ViewFrame>
        )
      }}
    </QueryGate>
  )
}

/** Outcomes among the people most similar to the person, vs everyone. */
export function SimilarView({ request, title, subtitle, actions }: {
  request: SimilarRequest
  title?: React.ReactNode
  subtitle?: React.ReactNode
  actions?: React.ReactNode
}) {
  const { data: schema } = useSchema()
  const cols = useColumns(schema)
  const q = useQuery({ queryKey: ['similar', request], queryFn: () => api.similar(request), placeholderData: keepPreviousData })
  return (
    <QueryGate isPending={q.isPending} error={q.error} refetch={q.refetch}>
      {() => {
        const d = q.data!
        const notable = d.values
          .filter((v) => !v.matched && v.median !== undefined && cols.get(v.column)?.role !== 'outcome')
          .map((v) => {
            const c = cols.get(v.column)!
            const z = c.stats && c.stats.std > 0 ? (v.median! - c.stats.median) / c.stats.std : 0
            return { v, c, z }
          })
          .filter((x) => Math.abs(x.z) >= 0.15)
          .sort((a, b) => Math.abs(b.z) - Math.abs(a.z))
          .slice(0, 4)
        return (
          <ViewFrame
            title={title}
            subtitle={subtitle}
            actions={actions}
            isFetching={q.isFetching}
            table={
              <DataTable
                caption="Outcomes among similar people"
                columns={[{ label: 'Outcome' }, { label: 'People like you', align: 'right' }, { label: 'Everyone', align: 'right' }, { label: 'Ratio', align: 'right' }]}
                rows={d.outcomes.map((o) => [o.outcome.label, fmtPct(o.similar.value), fmtPct(o.overall.value), `${o.ratio.toFixed(2)}×`])}
              />
            }
            footer={
              <div className="space-y-2">
                {notable.length > 0 && (
                  <div className="text-xs text-ink-2">
                    <p className="mb-1 font-medium text-ink">How these people differ from everyone</p>
                    <ul className="space-y-0.5">
                      {notable.map(({ v, c }) => (
                        <li key={v.column}>
                          {c.label}: median <span className="tabular font-medium text-ink">{fmtAny(c, v.median)}</span> vs{' '}
                          <span className="tabular">{fmtAny(c, c.stats?.median)}</span> overall
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                <Note>
                  Matched on {d.features.length} of your values ({d.features.map((f) => cols.get(f)?.short ?? f).join(', ')}). The{' '}
                  {fmtNumber(d.k)} closest people in this dataset, not a prediction for you.
                </Note>
              </div>
            }
          >
            <Dumbbells rows={d.outcomes.map((o) => ({ label: o.outcome.label, overall: o.overall.value, similar: o.similar.value, n: o.similar.n }))} />
          </ViewFrame>
        )
      }}
    </QueryGate>
  )
}
