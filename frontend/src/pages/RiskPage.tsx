import { useEffect, useState } from 'react'
import { useLocation, useSearchParams } from 'react-router'
import { UserCircleIcon } from '@hugeicons/core-free-icons'
import CountUp from '@/components/reactbits/CountUp'
import AnimatedContent from '@/components/reactbits/AnimatedContent'
import { CohortControls, FilterRow } from '@/components/filters/FilterBar'
import { Segmented, Select, Switch } from '@/components/ui/controls'
import { Button, Card, EmptyState, ErrorState, Loading, Note, StrengthBadge } from '@/components/ui/primitives'
import { RankBody, RiskBody, useRank, useRisk } from '@/components/views/RiskViews'
import { ProfileView, SimilarView } from '@/components/views/PersonalViews'
import { PageHeader } from './InsightsPage'
import { decodeFilters, decodeOutcome, encodeFilters, encodeOutcome } from '@/lib/links'
import { isGroupable, useColumns, useSchema } from '@/lib/schema'
import { useSettings, useUi } from '@/lib/store'
import { fmtNumber, fmtPct, lowerFirst } from '@/lib/format'
import type { Column, MatchOn, Outcome, Schema } from '@/lib/types'

const DEFAULT_OUTCOME: Outcome = { preset: 'diagnosed_diabetes' }

export function RiskPage() {
  const { data: schema, isPending, error, refetch } = useSchema()
  const [params, setParams] = useSearchParams()
  const { hash } = useLocation()

  const update = (patch: Record<string, string | undefined>) => {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(patch)) {
      if (v === undefined || v === '') next.delete(k)
      else next.set(k, v)
    }
    setParams(next, { replace: true })
  }

  useEffect(() => {
    if (hash === '#your-picture' && schema) {
      const t = setTimeout(() => document.getElementById('your-picture')?.scrollIntoView({ behavior: 'smooth' }), 300)
      return () => clearTimeout(t)
    }
  }, [hash, schema])

  const outcome = decodeOutcome(params.get('o'), DEFAULT_OUTCOME)
  const filters = decodeFilters(params.get('f'))
  const markers = params.get('markers') === '1'
  // Query keys are compared by value, so a fresh object per render is fine.
  const rankReq = { outcome, filters, includeMarkers: markers }
  const rank = useRank(schema ? rankReq : null)
  const factor = params.get('factor') ?? rank.data?.factors[0]?.column

  if (isPending) return <Loading label="Loading the dataset" className="py-24" />
  if (error) return <div className="mx-auto max-w-[1200px] px-4 py-10"><ErrorState error={error} onRetry={refetch} /></div>

  const top = rank.data?.factors[0]

  return (
    <div className="mx-auto max-w-[1200px] space-y-5 px-4 py-8">
      <PageHeader
        title="Risk Explorer"
        subtitle="Which factors go with an outcome in this dataset, how strongly, and what that looks like for people like you."
      />
      <FilterRow>
        <OutcomePicker schema={schema} outcome={outcome} onChange={(o) => update({ o: encodeOutcome(o), factor: undefined, ref: undefined })} />
        <CohortControls filters={filters} onChange={(f) => update({ f: encodeFilters(f) })} />
        <div className="sm:ml-auto">
          <Switch id="markers" checked={markers} onChange={(on) => update({ markers: on ? '1' : undefined })} label="Include lab tests" />
        </div>
      </FilterRow>

      {rank.error ? (
        <ErrorState error={rank.error} onRetry={rank.refetch} />
      ) : !rank.data ? (
        <Loading label="Ranking factors" />
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-3">
            <StatTile label={`${rank.data.outcome.label} overall`} sub={rank.data.outcome.definition}>
              <CountUp to={Number((rank.data.base.value * 100).toFixed(1))} decimals={1} suffix="%" duration={0.8} />
            </StatTile>
            <StatTile label="People in view" sub={rank.data.cohort === 'Everyone' ? 'Everyone in the dataset' : rank.data.cohort}>
              <CountUp to={rank.data.n} separator="," duration={0.8} />
            </StatTile>
            <StatTile label="Strongest link" sub={top ? top.verdict.split('. ')[0] : '–'}>
              <span className="text-[22px] leading-tight">{top && top.association.strength !== 'none' ? top.label : 'None found'}</span>
            </StatTile>
          </div>

          <div className="grid gap-4 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] [&>*]:min-w-0">
            <Card className="p-4 sm:p-5">
              <RankBody
                d={rank.data}
                isFetching={rank.isFetching}
                selected={factor}
                onSelect={(f) => update({ factor: f, ref: undefined, strat: undefined })}
                initial={10}
                title="What matters most"
                subtitle={`Ranked by Cramér's V: how much ${lowerFirst(rank.data.outcome.label)} rates differ between a factor's groups.${markers ? '' : ' Lab tests are hidden.'}`}
              />
            </Card>
            <Card className="p-4 sm:p-5">
              {factor ? (
                <FactorPanel
                  key={`${factor}-${encodeOutcome(outcome)}`}
                  schema={schema}
                  outcome={outcome}
                  factor={factor}
                  reference={params.get('ref') ?? undefined}
                  stratifyBy={params.get('strat') ?? undefined}
                  filters={filters}
                  onChange={(patch) => update(patch)}
                />
              ) : (
                <EmptyState title="Choose a factor">Pick one from the list to see the rates for each group.</EmptyState>
              )}
            </Card>
          </div>
        </>
      )}

      <YourPicture schema={schema} />
    </div>
  )
}

function StatTile({ label, sub, children }: { label: string; sub?: string; children: React.ReactNode }) {
  return (
    <Card className="px-5 py-4">
      <p className="text-xs font-medium text-ink-2">{label}</p>
      <p className="mt-1 text-[30px] font-semibold tracking-tight text-ink">{children}</p>
      {sub && <p className="mt-0.5 truncate text-xs text-muted" title={sub}>{sub}</p>}
    </Card>
  )
}

function OutcomePicker({ schema, outcome, onChange }: { schema: Schema; outcome: Outcome; onChange: (o: Outcome) => void }) {
  const cols = useColumns(schema)
  const [custom, setCustom] = useState(!outcome.preset)
  const options = [
    ...schema.outcomes.map((p) => ({ value: p.id, label: p.label })),
    { value: '__custom', label: 'Custom outcome…' },
  ]
  const outcomeCols = schema.columns.filter((c) => c.kind !== 'text' && (c.kind !== 'categorical' || (c.levels?.length ?? 0) <= 12))
  const col = outcome.column ? cols.get(outcome.column) : undefined

  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-xs font-medium text-muted">Outcome</span>
      <Select
        options={options}
        value={custom ? '__custom' : (outcome.preset ?? '')}
        onChange={(v) => {
          if (v === '__custom') {
            setCustom(true)
            const first = cols.get('diabetes_stage')
            onChange(first ? { column: first.name, equals: [first.levels![3] ?? first.levels![0]] } : DEFAULT_OUTCOME)
          } else {
            setCustom(false)
            onChange({ preset: v })
          }
        }}
        ariaLabel="Outcome"
        size="sm"
        menuWidth={260}
      />
      {custom && (
        <>
          <Select
            options={outcomeCols.map((c) => ({ value: c.name, label: c.label }))}
            value={outcome.column ?? ''}
            onChange={(name) => {
              const c = cols.get(name)!
              if (c.kind === 'binary') onChange({ column: name, equals: [c.levels![1]] })
              else if (c.kind === 'categorical' || c.kind === 'discrete') onChange({ column: name, equals: [c.levels![c.levels!.length - 1]] })
              else onChange({ column: name, min: Number((c.stats?.p75 ?? 0).toFixed(c.decimals)) })
            }}
            ariaLabel="Outcome column"
            size="sm"
            menuWidth={240}
          />
          {col && (col.kind === 'categorical' || col.kind === 'discrete' || col.kind === 'binary') && (
            <Select
              options={col.levels ?? []}
              value={outcome.equals?.[0] ?? ''}
              onChange={(v) => onChange({ column: col.name, equals: [v] })}
              ariaLabel="Outcome value"
              size="sm"
              menuWidth={200}
            />
          )}
          {col && col.kind === 'continuous' && (
            <label className="inline-flex items-center gap-1.5 text-xs text-ink-2">
              at least
              <input
                type="number"
                defaultValue={outcome.min}
                step={col.input?.step}
                onBlur={(e) => e.target.value !== '' && onChange({ column: col.name, min: Number(e.target.value) })}
                onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLInputElement).blur()}
                className="tabular h-8 w-20 rounded-xl bg-surface-2 px-2.5 text-sm text-ink ring-1 ring-line outline-none focus:ring-2 focus:ring-accent"
                aria-label="Threshold"
              />
              {col.unit}
            </label>
          )}
        </>
      )}
    </div>
  )
}

function FactorPanel({ schema, outcome, factor, reference, stratifyBy, filters, onChange }: {
  schema: Schema
  outcome: Outcome
  factor: string
  reference?: string
  stratifyBy?: string
  filters: ReturnType<typeof decodeFilters>
  onChange: (patch: Record<string, string | undefined>) => void
}) {
  const cols = useColumns(schema)
  const fc = cols.get(factor)
  const req = { outcome, factor, reference, stratifyBy, filters }
  const q = useRisk(req)
  const strataOptions = schema.columns.filter(
    (c: Column) => isGroupable(c) && c.name !== factor && c.name !== (outcome.column ?? '') && c.role !== 'outcome' && (c.kind !== 'categorical' || (c.levels?.length ?? 0) <= 6),
  )

  if (q.error) return <ErrorState error={q.error} onRetry={q.refetch} />
  if (!q.data) return <Loading />
  const d = q.data
  return (
    <AnimatedContent trigger="mount" distance={16} duration={0.45}>
      <div className="mb-4 flex flex-wrap items-center gap-x-3 gap-y-2">
        <div className="min-w-0 flex-1">
          <h3 className="text-[17px] font-semibold text-ink">
            {d.outcome.label} by {lowerFirst(d.factorLabel)}
          </h3>
          {fc?.description && <p className="mt-0.5 text-xs text-ink-2">{fc.description}</p>}
        </div>
        <StrengthBadge strength={d.association.strength} />
      </div>
      <div className="mb-4 flex flex-wrap items-center gap-2 text-xs">
        <span className="font-medium text-muted">Compare against</span>
        <Select
          options={d.levels.map((l) => l.label)}
          value={d.reference}
          onChange={(v) => onChange({ ref: v })}
          ariaLabel="Reference group"
          size="sm"
          menuWidth={200}
        />
        <span className="ml-2 font-medium text-muted">Within groups of</span>
        <Select
          options={[{ value: '', label: 'No split' }, ...strataOptions.map((c) => ({ value: c.name, label: c.label }))]}
          value={stratifyBy ?? ''}
          onChange={(v) => onChange({ strat: v || undefined })}
          ariaLabel="Stratify by"
          size="sm"
          menuWidth={220}
        />
      </div>
      <RiskBody d={d} isFetching={q.isFetching} />
      <Note className="mt-4">
        Dots show the rate in each group; lines show the 95% confidence interval. Relative risk compares each group with “{d.reference}”. A link
        in this dataset does not prove cause.
      </Note>
    </AnimatedContent>
  )
}

function YourPicture({ schema }: { schema: Schema }) {
  const values = useSettings((s) => s.values)
  const openValues = useUi((s) => s.openValues)
  const [matchOn, setMatchOn] = useState<MatchOn>('all')
  const personal = Object.fromEntries(Object.entries(values).filter(([k]) => schema.columns.some((c) => c.name === k && c.role !== 'outcome')))
  const count = Object.keys(personal).length
  const lifestyleCount = Object.keys(personal).filter((k) =>
    ['demographic', 'lifestyle', 'history', 'body'].includes(schema.columns.find((c) => c.name === k)?.role ?? ''),
  ).length

  return (
    <section id="your-picture" className="scroll-mt-24 space-y-4 pt-4">
      <div>
        <h2 className="text-xl font-semibold tracking-tight text-ink">Your picture</h2>
        <p className="mt-1 text-sm text-ink-2">How common each outcome is among the {fmtNumber(500)} people in the dataset most similar to you.</p>
      </div>
      {count < 2 ? (
        <Card>
          <EmptyState
            icon={UserCircleIcon}
            title="Add a few of your values"
            action={<Button variant="primary" icon={UserCircleIcon} onClick={() => openValues(true)}>Add my values</Button>}
          >
            Enter at least two values from your checkup (or load a sample patient) to see outcomes among people like you.
          </EmptyState>
        </Card>
      ) : (
        <div className="grid gap-4 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] [&>*]:min-w-0">
          <Card className="p-4 sm:p-5">
            <SimilarView
              request={{ values: personal, k: 500, matchOn: matchOn === 'lifestyle' && lifestyleCount >= 2 ? 'lifestyle' : 'all' }}
              title="People like you"
              subtitle={matchOn === 'all' ? 'Matched on all your values' : 'Matched on background, lifestyle, history and body only'}
              actions={
                <Segmented
                  items={[
                    { value: 'all', label: 'All values' },
                    { value: 'lifestyle', label: 'Lifestyle' },
                  ]}
                  value={matchOn}
                  onChange={(v) => setMatchOn(v as MatchOn)}
                  ariaLabel="Match on"
                  disabled={lifestyleCount < 2}
                />
              }
            />
          </Card>
          <Card className="p-4 sm:p-5">
            <ProfileView
              request={{ values: personal }}
              onlyFlagged
              title="Values to talk about"
              subtitle="Your values that fall in a watch or alert range of general clinical guides."
            />
            <p className="mt-3 text-xs text-muted">
              Overall in this dataset: {fmtPct((schema.columns.find((c) => c.name === 'diagnosed_diabetes')?.counts?.[1] ?? 0) / schema.dataset.rows)} have
              diagnosed diabetes.
            </p>
          </Card>
        </div>
      )}
    </section>
  )
}
