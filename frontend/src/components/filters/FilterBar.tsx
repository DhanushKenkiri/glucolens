import { useState } from 'react'
import { Add01Icon, Cancel01Icon } from '@hugeicons/core-free-icons'
import { Button, Icon } from '@/components/ui/primitives'
import { Segmented, Select } from '@/components/ui/controls'
import { isGroupable, likeMeFilters, useColumns, useSchema } from '@/lib/schema'
import { useSettings, useUi } from '@/lib/store'
import { fmtNumber, fmtValue } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { Column, Filter } from '@/lib/types'

export function describeFilter(f: Filter, col?: Column): string {
  const name = col?.short ?? f.column
  if (f.equals && f.equals.length > 0) return `${name}: ${f.equals.join(' or ')}`
  if (f.min !== undefined && f.max !== undefined) return `${name} ${fmtNumber(f.min, col?.decimals ?? 2)}–${fmtValue(col, f.max)}`
  if (f.min !== undefined) return `${name} ≥ ${fmtValue(col, f.min)}`
  if (f.max !== undefined) return `${name} ≤ ${fmtValue(col, f.max)}`
  return name
}

const same = (a: Filter[], b: Filter[]) => JSON.stringify(a) === JSON.stringify(b)

/** Cohort selector: everyone, people like me, or custom filters. */
export function CohortControls({ filters, onChange }: { filters: Filter[]; onChange: (f: Filter[]) => void }) {
  const values = useSettings((s) => s.values)
  const openValues = useUi((s) => s.openValues)
  const { data: schema } = useSchema()
  const cols = useColumns(schema)
  const [adding, setAdding] = useState(false)
  const likeMe = likeMeFilters(values)
  const mode = filters.length === 0 ? 'Everyone' : likeMe && same(filters, likeMe) ? 'People like me' : 'Custom'
  const items = ['Everyone', 'People like me', ...(mode === 'Custom' ? ['Custom'] : [])]

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      <Segmented
        items={items}
        value={mode}
        ariaLabel="Compare with"
        onChange={(v) => {
          if (v === 'Everyone') onChange([])
          else if (v === 'People like me') {
            if (likeMe) onChange(likeMe)
            else openValues(true)
          }
        }}
      />
      {mode === 'People like me' && <span className="text-xs text-ink-2">Same gender, age ±5 years</span>}
      {mode === 'Custom' &&
        filters.map((f, i) => (
          <span key={i} className="inline-flex h-8 items-center gap-1 rounded-full bg-accent-soft pr-1 pl-3 text-xs font-medium text-ink">
            {describeFilter(f, cols.get(f.column))}
            <button
              type="button"
              aria-label={`Remove filter ${describeFilter(f, cols.get(f.column))}`}
              onClick={() => onChange(filters.filter((_, j) => j !== i))}
              className="grid size-6 place-items-center rounded-full text-ink-2 hover:bg-surface hover:text-ink"
            >
              <Icon icon={Cancel01Icon} size={12} />
            </button>
          </span>
        ))}
      <Button size="sm" variant="ghost" icon={Add01Icon} onClick={() => setAdding((a) => !a)} aria-expanded={adding}>
        Filter
      </Button>
      {adding && schema && (
        <AddFilterPanel
          columns={schema.columns.filter(isGroupable)}
          onCancel={() => setAdding(false)}
          onAdd={(f) => {
            const base = mode === 'Custom' ? filters : mode === 'People like me' ? filters : []
            onChange([...base.filter((x) => x.column !== f.column), f])
            setAdding(false)
          }}
        />
      )}
    </div>
  )
}

function AddFilterPanel({ columns, onAdd, onCancel }: { columns: Column[]; onAdd: (f: Filter) => void; onCancel: () => void }) {
  const [name, setName] = useState('')
  const [picked, setPicked] = useState<string[]>([])
  const [lo, setLo] = useState('')
  const [hi, setHi] = useState('')
  const col = columns.find((c) => c.name === name)
  const levelsMode = col && col.kind !== 'continuous'
  const canAdd = col && (levelsMode ? picked.length > 0 : lo !== '' || hi !== '')

  return (
    <div className="card mt-1 flex w-full flex-wrap items-end gap-3 p-3 sm:w-auto">
      <div>
        <p className="mb-1 text-[11px] font-medium text-muted">Column</p>
        <Select
          options={columns.map((c) => ({ value: c.name, label: c.label }))}
          value={name}
          onChange={(v) => {
            setName(v)
            setPicked([])
            setLo('')
            setHi('')
          }}
          placeholder="Choose…"
          ariaLabel="Filter column"
          size="sm"
          menuWidth={240}
        />
      </div>
      {col && levelsMode && (
        <div className="min-w-0">
          <p className="mb-1 text-[11px] font-medium text-muted">Keep people with</p>
          <div className="flex max-w-[420px] flex-wrap gap-1.5">
            {(col.levels ?? []).map((l) => {
              const on = picked.includes(l)
              return (
                <button
                  key={l}
                  type="button"
                  aria-pressed={on}
                  onClick={() => setPicked(on ? picked.filter((p) => p !== l) : [...picked, l])}
                  className={cn('h-7 rounded-full px-2.5 text-xs ring-1 transition-colors', on ? 'bg-accent text-white ring-accent' : 'bg-surface-2 text-ink ring-line hover:bg-surface-3')}
                >
                  {l}
                </button>
              )
            })}
          </div>
        </div>
      )}
      {col && !levelsMode && (
        <div className="flex items-end gap-2">
          {[
            ['From', lo, setLo],
            ['To', hi, setHi],
          ].map(([label, v, set]) => (
            <label key={label as string} className="block">
              <span className="mb-1 block text-[11px] font-medium text-muted">{label as string}</span>
              <input
                type="number"
                value={v as string}
                onChange={(e) => (set as (s: string) => void)(e.target.value)}
                placeholder={label === 'From' ? fmtNumber(col.stats?.min, col.decimals) : fmtNumber(col.stats?.max, col.decimals)}
                className="tabular h-8 w-24 rounded-xl bg-surface-2 px-2.5 text-sm text-ink ring-1 ring-line outline-none focus:ring-2 focus:ring-accent"
              />
            </label>
          ))}
          {col.unit && <span className="pb-2 text-xs text-muted">{col.unit}</span>}
        </div>
      )}
      <div className="flex gap-1.5">
        <Button
          size="sm"
          variant="primary"
          disabled={!canAdd}
          onClick={() => {
            if (!col) return
            if (levelsMode) onAdd({ column: col.name, equals: picked })
            else onAdd({ column: col.name, min: lo === '' ? undefined : Number(lo), max: hi === '' ? undefined : Number(hi) })
          }}
        >
          Add
        </Button>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  )
}

/** Lays out a row of filters above the content they scope. */
export function FilterRow({ children }: { children: React.ReactNode }) {
  return <div className="flex flex-wrap items-center gap-x-4 gap-y-3 rounded-2xl bg-surface px-3 py-3 ring-1 ring-line">{children}</div>
}
