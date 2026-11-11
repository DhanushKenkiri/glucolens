import { useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowUpRight01Icon, ChartBarLineIcon, InformationCircleIcon, UserCircleIcon } from '@hugeicons/core-free-icons'
import SpotlightCard from '@/components/reactbits/SpotlightCard'
import FadeContent from '@/components/reactbits/FadeContent'
import { CohortControls, FilterRow } from '@/components/filters/FilterBar'
import { Segmented, Select, Tip } from '@/components/ui/controls'
import { Button, Card, Drawer, EmptyState, ErrorState, Icon, Loading, Note, ToneBadge } from '@/components/ui/primitives'
import { DistributionView } from '@/components/views/DistributionView'
import { CompareView } from '@/components/views/CompareView'
import { ProfileView } from '@/components/views/PersonalViews'
import { CorrHeatmap, DensityPlot } from '@/components/charts/Matrix'
import { ViewFrame } from '@/components/views/Frame'
import { DataTable } from '@/components/charts/core'
import { api } from '@/lib/api'
import { decodeFilters, encodeFilters, riskLink } from '@/lib/links'
import { isGroupable, keepPreviousData, useColumns, useSchema } from '@/lib/schema'
import { useSettings, useUi } from '@/lib/store'
import { fmtAny, fmtNumber, lowerFirst, ordinal } from '@/lib/format'
import { useResolvedTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'
import type { Column, Filter, Schema } from '@/lib/types'

export function PageHeader({ title, subtitle, children }: { title: string; subtitle: string; children?: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className="text-[28px] font-semibold tracking-tight text-ink sm:text-[32px]">{title}</h1>
        <p className="mt-1 max-w-2xl text-[15px] text-ink-2">{subtitle}</p>
      </div>
      {children}
    </div>
  )
}

export function InsightsPage() {
  const { data: schema, isPending, error, refetch } = useSchema()
  const [params, setParams] = useSearchParams()

  const update = (patch: Record<string, string | undefined>) => {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(patch)) {
      if (v === undefined || v === '') next.delete(k)
      else next.set(k, v)
    }
    setParams(next, { replace: true })
  }

  if (isPending) return <Loading label="Loading the dataset" className="py-24" />
  if (error) return <div className="mx-auto max-w-[1200px] px-4 py-10"><ErrorState error={error} onRetry={refetch} /></div>

  const filters = decodeFilters(params.get('f'))
  const view = params.get('view') === 'relationships' ? 'relationships' : 'sections'
  const section = schema.sections.find((s) => s.id === params.get('section'))?.id ?? schema.sections[0].id
  const split = params.get('split') ?? ''
  const focus = params.get('col') ?? undefined

  return (
    <div className="mx-auto max-w-[1200px] space-y-5 px-4 py-8">
      <PageHeader
        title="Insights"
        subtitle={`See how each checkup value is spread across ${fmtNumber(schema.dataset.rows)} people, and where yours sits.`}
      />
      <FilterRow>
        <CohortControls filters={filters} onChange={(f) => update({ f: encodeFilters(f) })} />
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium text-muted">Split by</span>
          <Select
            options={[{ value: '', label: 'Nothing' }, ...schema.columns.filter((c) => isGroupable(c) && c.kind !== 'continuous').map((c) => ({ value: c.name, label: c.label })),
              ...schema.columns.filter((c) => c.kind === 'continuous' && (c.groups?.length || c.bands?.some((b) => !b.when))).map((c) => ({ value: c.name, label: `${c.label} groups` }))]}
            value={split}
            onChange={(v) => update({ split: v || undefined })}
            ariaLabel="Split by"
            size="sm"
            menuWidth={250}
          />
        </div>
        <div className="sm:ml-auto">
          <Segmented
            items={[
              { value: 'sections', label: 'By section' },
              { value: 'relationships', label: 'Relationships' },
            ]}
            value={view}
            onChange={(v) => update({ view: v === 'sections' ? undefined : v })}
            ariaLabel="View"
          />
        </div>
      </FilterRow>

      {view === 'sections' ? (
        <SectionsView schema={schema} section={section} filters={filters} split={split} focus={focus} onSection={(id) => update({ section: id, col: undefined })} />
      ) : (
        <RelationshipsView x={params.get('x') ?? undefined} y={params.get('y') ?? undefined} filters={filters} onPair={(x, y) => update({ x, y })} />
      )}
    </div>
  )
}

function SectionsView({ schema, section, filters, split, focus, onSection }: {
  schema: Schema
  section: string
  filters: Filter[]
  split: string
  focus?: string
  onSection: (id: string) => void
}) {
  const cols = useColumns(schema)
  const values = useSettings((s) => s.values)
  const openValues = useUi((s) => s.openValues)
  const current = schema.sections.find((s) => s.id === section)!
  const columns = current.columns.map((n) => cols.get(n)!).filter((c) => c && c.kind !== 'text')
  const mine = Object.fromEntries(Object.entries(values).filter(([k]) => current.columns.includes(k)))
  const [detail, setDetail] = useState<Column | null>(null)

  useEffect(() => {
    if (!focus) return
    const t = setTimeout(() => document.getElementById(`col-${focus}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' }), 250)
    return () => clearTimeout(t)
  }, [focus, section])

  return (
    <div className="space-y-5">
      <nav aria-label="Sections" className="no-scrollbar -mx-4 flex gap-2 overflow-x-auto px-4 pb-1 lg:mx-0 lg:flex-wrap lg:overflow-visible lg:px-0">
        {schema.sections.map((s) => {
          const on = s.id === section
          const filled = s.columns.filter((c) => values[c] !== undefined).length
          return (
            <button
              key={s.id}
              type="button"
              onClick={() => onSection(s.id)}
              aria-current={on ? 'page' : undefined}
              className={cn(
                'inline-flex h-9 shrink-0 items-center gap-2 rounded-full px-4 text-sm font-medium transition-colors',
                on ? 'bg-ink text-page' : 'bg-surface text-ink-2 ring-1 ring-line hover:text-ink',
              )}
            >
              {s.label}
              {filled > 0 && (
                <span className={cn('tabular rounded-full px-1.5 text-[11px]', on ? 'bg-page/20' : 'bg-accent-soft text-accent')}>{filled}</span>
              )}
            </button>
          )
        })}
      </nav>

      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p className="text-sm text-ink-2">{current.description}</p>
        {Object.keys(values).length === 0 && (
          <button type="button" onClick={() => openValues(true)} className="inline-flex items-center gap-1.5 text-sm font-medium text-accent">
            <Icon icon={UserCircleIcon} size={16} /> Add your values to see where you sit
          </button>
        )}
      </div>

      {Object.keys(mine).length > 0 && (
        <Card className="p-5">
          <ProfileView
            request={{ values: mine, filters }}
            title={`Your ${current.label.toLowerCase()} values`}
            subtitle="Percentile among the people in view. The shaded part of each track is the middle half."
          />
        </Card>
      )}

      <div className="grid gap-4 md:grid-cols-2 [&>*]:min-w-0">
        {columns.map((c, i) => (
          <FadeContent key={`${section}-${c.name}`} trigger="mount" duration={0.5} delay={Math.min(i, 6) * 0.05} blur={false}>
            <ColumnCard column={c} filters={filters} split={split !== c.name ? split : ''} focused={focus === c.name} onDetail={() => setDetail(c)} />
          </FadeContent>
        ))}
      </div>

      <Drawer open={!!detail} onClose={() => setDetail(null)} title={detail ? detail.label : ''} width={720}>
        {detail && <ColumnDetail column={detail} filters={filters} split={split} schema={schema} />}
      </Drawer>
    </div>
  )
}

function ColumnCard({ column, filters, split, focused, onDetail }: { column: Column; filters: Filter[]; split: string; focused: boolean; onDetail: () => void }) {
  const mode = useResolvedTheme()
  const you = useSettings((s) => s.values[column.name])
  const profileQ = useQuery({
    queryKey: ['profile', { values: { [column.name]: you }, filters }],
    queryFn: () => api.profile({ values: { [column.name]: you! }, filters }),
    enabled: you !== undefined,
    placeholderData: keepPreviousData,
  })
  const item = you !== undefined ? profileQ.data?.items[0] : undefined

  return (
    <SpotlightCard
      id={`col-${column.name}`}
      theme={mode}
      spotlightColor="var(--accent)"
      intensity={mode === 'dark' ? 0.18 : 0.1}
      spotlightSize={420}
      className={cn('h-full p-0!', focused && 'ring-2 ring-accent')}
      style={{ '--spotlight-card-surface': 'var(--surface)', '--spotlight-card-border': 'var(--line)', '--spotlight-card-shadow': 'var(--shadow-card)', borderRadius: 18 } as React.CSSProperties}
    >
      <div className="relative z-[2] p-5">
        <DistributionView
          request={{ column: column.name, filters, splitBy: split || undefined }}
          you={you}
          compact
          title={
            <span className="flex items-center gap-1.5">
              <span className="truncate">{column.label}</span>
              {column.unit && <span className="text-xs font-normal text-muted">{column.unit}</span>}
              {column.description && (
                <Tip content={<span className="block max-w-[260px] text-left leading-snug">{column.description}</span>}>
                  <button type="button" aria-label={`About ${column.label}`} className="grid size-6 place-items-center rounded-full text-muted hover:text-ink">
                    <Icon icon={InformationCircleIcon} size={15} />
                  </button>
                </Tip>
              )}
            </span>
          }
          subtitle={
            you !== undefined ? (
              <span className="flex flex-wrap items-center gap-2">
                <span>
                  You: <span className="font-semibold text-ink">{fmtAny(column, you)}</span>
                </span>
                {item?.percentile !== undefined && <span className="tabular">{ordinal(item.percentile)} percentile</span>}
                {item?.band && <ToneBadge tone={item.band.tone}>{item.band.label}</ToneBadge>}
              </span>
            ) : undefined
          }
          actions={
            <button
              type="button"
              onClick={onDetail}
              aria-label={`Compare ${column.label} across groups`}
              className="inline-flex h-8 items-center gap-1 rounded-full px-2.5 text-xs font-medium text-ink-2 transition-colors hover:bg-surface-2 hover:text-ink"
            >
              <Icon icon={ChartBarLineIcon} size={15} />
              <span className="hidden sm:inline">Compare</span>
            </button>
          }
        />
      </div>
    </SpotlightCard>
  )
}

function ColumnDetail({ column, filters, split, schema }: { column: Column; filters: Filter[]; split: string; schema: Schema }) {
  const groupable = schema.columns.filter((c) => isGroupable(c) && c.name !== column.name)
  const [groupBy, setGroupBy] = useState(split && split !== column.name ? split : column.name === 'gender' ? 'age' : 'gender')
  const you = useSettings((s) => s.values[column.name])
  const outcome = column.kind === 'binary' ? { column: column.name } : undefined
  return (
    <div className="space-y-6 px-5 py-5">
      {column.description && <p className="text-sm text-ink-2">{column.description}</p>}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs font-medium text-muted">Compare across</span>
        <Select
          options={groupable.map((c) => ({ value: c.name, label: c.label }))}
          value={groupBy}
          onChange={setGroupBy}
          ariaLabel="Compare across"
          size="sm"
          menuWidth={250}
        />
      </div>
      <CompareView
        request={{ metric: column.name, groupBy, filters }}
        you={typeof you === 'number' ? you : undefined}
        title={`${column.label} by ${lowerFirst(schema.columns.find((c) => c.name === groupBy)?.label ?? '')}`}
      />
      <div className="border-t border-line pt-5">
        <DistributionView request={{ column: column.name, filters }} you={you} title="Whole distribution" />
      </div>
      {outcome && (
        <Link to={riskLink({ outcome, filters })} className="inline-flex items-center gap-1.5 text-sm font-medium text-accent">
          Explore what's linked to {column.label.toLowerCase()} <Icon icon={ArrowUpRight01Icon} size={15} />
        </Link>
      )}
    </div>
  )
}

function RelationshipsView({ x, y, filters, onPair }: { x?: string; y?: string; filters: Filter[]; onPair: (x: string, y: string) => void }) {
  const corr = useQuery({ queryKey: ['correlations'], queryFn: api.correlations, staleTime: Infinity })
  const pair = useMemo<[string, string] | undefined>(() => {
    if (x && y) return [x, y]
    const top = corr.data?.top[0]
    return top ? [top.b, top.a] : undefined
  }, [x, y, corr.data])
  const rel = useQuery({
    queryKey: ['relationship', pair, filters],
    queryFn: () => api.relationship({ x: pair![0], y: pair![1], filters }),
    enabled: !!pair,
    placeholderData: keepPreviousData,
  })
  const labelOf = (n: string) => corr.data?.columns.find((c) => c.name === n)?.label ?? n

  if (corr.isPending) return <Loading label="Computing correlations" />
  if (corr.error) return <ErrorState error={corr.error} onRetry={corr.refetch} />

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)] [&>*]:min-w-0">
      <Card className="p-5">
        <ViewFrame
          title="How values move together"
          subtitle="Correlation (r) between every pair of numeric values across everyone. Click a cell to explore that pair."
          table={
            <DataTable
              caption="Strongest correlations"
              columns={[{ label: 'Pair' }, { label: 'r', align: 'right' }]}
              rows={corr.data.top.map((p) => [`${labelOf(p.a)} × ${labelOf(p.b)}`, p.r.toFixed(2)])}
            />
          }
        >
          <CorrHeatmap data={corr.data} selected={pair} onSelect={(a, b) => onPair(b, a)} />
        </ViewFrame>
        <div className="mt-5">
          <p className="mb-2 text-xs font-semibold text-ink-2">Strongest links</p>
          <div className="flex flex-wrap gap-1.5">
            {corr.data.top.slice(0, 8).map((p) => (
              <button
                key={`${p.a}-${p.b}`}
                type="button"
                onClick={() => onPair(p.b, p.a)}
                className="tabular inline-flex h-7 items-center gap-1.5 rounded-full bg-surface-2 px-2.5 text-xs text-ink ring-1 ring-line hover:bg-surface-3"
              >
                {labelOf(p.a)} × {labelOf(p.b)} <span className="text-muted">r {p.r.toFixed(2)}</span>
              </button>
            ))}
          </div>
        </div>
      </Card>
      <Card className="p-5">
        {!pair ? (
          <EmptyState title="Pick a pair" icon={ChartBarLineIcon}>Click a cell in the matrix.</EmptyState>
        ) : rel.isPending ? (
          <Loading />
        ) : rel.error ? (
          <ErrorState error={rel.error} onRetry={rel.refetch} />
        ) : (
          <ViewFrame
            title={`${rel.data.y.label} by ${lowerFirst(rel.data.x.label)}`}
            subtitle={rel.data.cohort === 'Everyone' ? `${fmtNumber(rel.data.n)} people` : `${rel.data.cohort} · ${fmtNumber(rel.data.n)} people`}
            isFetching={rel.isFetching}
            actions={
              <Button size="sm" variant="ghost" onClick={() => onPair(pair[1], pair[0])}>
                Swap axes
              </Button>
            }
            table={
              <DataTable
                caption="Median by bin"
                columns={[{ label: rel.data.x.label }, { label: 'People', align: 'right' }, { label: `Median ${rel.data.y.label}`, align: 'right' }, { label: 'Middle half', align: 'right' }]}
                rows={rel.data.trend.map((t) => [
                  `${fmtNumber(rel.data.xBins[t.bin].start, rel.data.x.decimals)}–${fmtNumber(rel.data.xBins[t.bin].end, rel.data.x.decimals)}`,
                  fmtNumber(t.n),
                  fmtNumber(t.median, rel.data.y.decimals),
                  `${fmtNumber(t.p25, rel.data.y.decimals)}–${fmtNumber(t.p75, rel.data.y.decimals)}`,
                ])}
              />
            }
            footer={
              <div className="space-y-2">
                <p className="text-[13px] text-ink">{rel.data.summary}</p>
                <Note>Correlation shows that two values move together in this dataset, not that one causes the other.</Note>
              </div>
            }
          >
            <DensityPlot data={rel.data} />
          </ViewFrame>
        )}
      </Card>
    </div>
  )
}
