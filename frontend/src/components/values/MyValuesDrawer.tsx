import { useMemo, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { ArrowDown01Icon, Cancel01Icon, Delete02Icon, ShuffleIcon } from '@hugeicons/core-free-icons'
import { Button, Drawer, Icon, Note } from '@/components/ui/primitives'
import { Select, Segmented, Switch } from '@/components/ui/controls'
import { api } from '@/lib/api'
import { isPersonalColumn, useSchema } from '@/lib/schema'
import { useSettings, useUi } from '@/lib/store'
import { fmtNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { Column, Values } from '@/lib/types'

export function MyValuesDrawer() {
  const open = useUi((s) => s.valuesOpen)
  const openValues = useUi((s) => s.openValues)
  const { data: schema } = useSchema()
  const values = useSettings((s) => s.values)
  const setValues = useSettings((s) => s.setValues)
  const clearValues = useSettings((s) => s.clearValues)
  const useValues = useSettings((s) => s.useValues)
  const setUseValues = useSettings((s) => s.setUseValues)

  const sample = useMutation({
    mutationFn: api.randomRecord,
    onSuccess: (rec) => {
      if (!schema) return
      const next: Values = {}
      for (const c of schema.columns) {
        const v = rec.values[c.name]
        if (isPersonalColumn(c) && v !== null && v !== undefined) next[c.name] = v
      }
      setValues(next)
    },
  })

  const sections = useMemo(() => {
    if (!schema) return []
    const byName = new Map(schema.columns.map((c) => [c.name, c]))
    return schema.sections
      .map((s) => ({ ...s, cols: s.columns.map((n) => byName.get(n)!).filter((c) => c && isPersonalColumn(c)) }))
      .filter((s) => s.cols.length > 0)
  }, [schema])

  const count = Object.keys(values).length
  const total = sections.reduce((n, s) => n + s.cols.length, 0)

  return (
    <Drawer
      open={open}
      onClose={() => openValues(false)}
      title={
        <span className="flex items-baseline gap-2">
          My values
          <span className="tabular text-xs font-normal text-muted">
            {count} of {total} entered
          </span>
        </span>
      }
      footer={
        <div className="space-y-3">
          <Switch id="use-values" checked={useValues} onChange={setUseValues} label="Use my values in assistant answers" />
          <div className="flex flex-wrap gap-2">
            <Button icon={ShuffleIcon} onClick={() => sample.mutate()} disabled={sample.isPending || !schema}>
              {sample.isPending ? 'Loading…' : 'Load sample patient'}
            </Button>
            <Button icon={Delete02Icon} variant="ghost" onClick={clearValues} disabled={count === 0}>
              Clear all
            </Button>
          </div>
          {sample.isError && <p className="text-xs text-alert-text">Couldn't load a sample: {(sample.error as Error).message}</p>}
        </div>
      }
    >
      <div className="space-y-1 px-5 py-4">
        <Note className="mb-3">
          Enter what you know from your checkup; every field is optional. Values are saved only in this browser. They are
          sent to the GlucoLens server to compare with the dataset and, when the switch below is on, to Groq to answer your
          questions.
        </Note>
        {sections.map((s, i) => (
          <SectionFields key={s.id} title={s.label} columns={s.cols} defaultOpen={i < 3} />
        ))}
      </div>
    </Drawer>
  )
}

function SectionFields({ title, columns, defaultOpen }: { title: string; columns: Column[]; defaultOpen: boolean }) {
  const [open, setOpen] = useState(defaultOpen)
  const values = useSettings((s) => s.values)
  const filled = columns.filter((c) => values[c.name] !== undefined).length
  return (
    <section className="border-b border-line py-2 last:border-0">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="flex w-full items-center justify-between gap-2 rounded-lg py-2 text-left"
      >
        <span className="text-sm font-semibold text-ink">{title}</span>
        <span className="flex items-center gap-2 text-xs text-muted">
          <span className="tabular">
            {filled}/{columns.length}
          </span>
          <Icon icon={ArrowDown01Icon} size={14} className={cn('transition-transform', open && 'rotate-180')} />
        </span>
      </button>
      {open && (
        <div className="space-y-3 pb-3 pt-1">
          {columns.map((c) => (
            <ValueField key={c.name} column={c} />
          ))}
        </div>
      )}
    </section>
  )
}

/** One input, chosen by column kind. */
export function ValueField({ column }: { column: Column }) {
  const value = useSettings((s) => s.values[column.name])
  const setValue = useSettings((s) => s.setValue)
  const id = `value-${column.name}`
  const levels = column.levels ?? []

  let control: React.ReactNode
  if (column.kind === 'binary') {
    const current = typeof value === 'number' ? (value >= 0.5 ? levels[1] : levels[0]) : ''
    control = (
      <div className="flex items-center gap-2">
        <Segmented
          items={['Not set', levels[0], levels[1]]}
          value={current || 'Not set'}
          onChange={(v) => setValue(column.name, v === 'Not set' ? undefined : v === levels[1] ? 1 : 0)}
          ariaLabel={column.label}
        />
      </div>
    )
  } else if (column.kind === 'categorical') {
    control = (
      <div className="flex items-center gap-1.5">
        <Select
          options={levels}
          value={typeof value === 'string' ? value : ''}
          onChange={(v) => setValue(column.name, v)}
          placeholder="Not set"
          ariaLabel={column.label}
          size="sm"
          menuWidth={200}
        />
        {value !== undefined && <ClearButton onClick={() => setValue(column.name, undefined)} label={`Clear ${column.label}`} />}
      </div>
    )
  } else {
    control = <NumberField column={column} value={typeof value === 'number' ? value : undefined} onChange={(v) => setValue(column.name, v)} id={id} />
  }

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3">
      <label htmlFor={id} className="min-w-0">
        <span className="block truncate text-sm text-ink">{column.label}</span>
        {column.stats && column.kind !== 'binary' && (
          <span className="block truncate text-[11px] text-muted">
            Dataset range {fmtNumber(column.stats.min, column.decimals)}–{fmtNumber(column.stats.max, column.decimals)}
            {column.unit ? ` ${column.unit}` : ''}
          </span>
        )}
      </label>
      {control}
    </div>
  )
}

function NumberField({ column, value, onChange, id }: { column: Column; value?: number; onChange: (v: number | undefined) => void; id: string }) {
  const [draft, setDraft] = useState<string | null>(null)
  const shown = draft ?? (value === undefined ? '' : String(value))
  const outside = value !== undefined && column.stats && (value < column.stats.min || value > column.stats.max)
  return (
    <div className="flex flex-col items-end">
      <div className="flex items-center gap-1.5">
        <div className="flex h-8 items-center rounded-xl bg-surface-2 ring-1 ring-line focus-within:ring-2 focus-within:ring-accent">
          <input
            id={id}
            type="number"
            inputMode="decimal"
            step={column.input?.step ?? 'any'}
            value={shown}
            placeholder="–"
            onChange={(e) => {
              setDraft(e.target.value)
              const raw = e.target.value.trim()
              if (raw === '') onChange(undefined)
              else {
                const n = Number(raw)
                if (Number.isFinite(n)) onChange(n)
              }
            }}
            onBlur={() => setDraft(null)}
            className="tabular w-[76px] bg-transparent px-2.5 text-right text-sm text-ink outline-none placeholder:text-muted"
          />
          {column.unit && <span className="pr-2.5 text-xs text-muted">{column.unit}</span>}
        </div>
        {value !== undefined && <ClearButton onClick={() => onChange(undefined)} label={`Clear ${column.label}`} />}
      </div>
      {outside && <span className="mt-1 text-[11px] text-muted">Outside the range seen in the dataset</span>}
    </div>
  )
}

function ClearButton({ onClick, label }: { onClick: () => void; label: string }) {
  return (
    <button type="button" onClick={onClick} aria-label={label} className="grid size-6 place-items-center rounded-full text-muted hover:bg-surface-3 hover:text-ink">
      <Icon icon={Cancel01Icon} size={13} />
    </button>
  )
}
