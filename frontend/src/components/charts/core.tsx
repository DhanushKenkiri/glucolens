import { useCallback, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from 'react'
import { useResolvedTheme } from '@/lib/theme'
import { CHART, type Mode } from '@/lib/palette'

/** Tracks an element's content width. */
export function useMeasure<T extends HTMLElement>(): [RefObject<T | null>, number] {
  const ref = useRef<T>(null)
  const [width, setWidth] = useState(0)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    setWidth(el.getBoundingClientRect().width)
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width ?? 0
      setWidth((prev) => (Math.abs(prev - w) > 0.5 ? w : prev))
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return [ref, width]
}

export function useChartColors(): { mode: Mode } & (typeof CHART)['light'] {
  const mode = useResolvedTheme()
  return { mode, ...CHART[mode] }
}

export function linear(d0: number, d1: number, r0: number, r1: number) {
  const span = d1 - d0 || 1
  const f = (v: number) => r0 + ((v - d0) / span) * (r1 - r0)
  return f
}

/**
 * A bar with 4px rounded corners at the data end and square corners at the
 * baseline. Vertical bars grow up from y0; horizontal bars grow right from x0.
 */
export function barPath(x: number, y: number, w: number, h: number, orient: 'up' | 'right', radius = 4): string {
  if (w <= 0 || h <= 0) return ''
  if (orient === 'up') {
    const r = Math.min(radius, w / 2, h)
    return `M${x},${y + h}V${y + r}Q${x},${y} ${x + r},${y}H${x + w - r}Q${x + w},${y} ${x + w},${y + r}V${y + h}Z`
  }
  const r = Math.min(radius, h / 2, w)
  return `M${x},${y}H${x + w - r}Q${x + w},${y} ${x + w},${y + r}V${y + h - r}Q${x + w},${y + h} ${x + w - r},${y + h}H${x}Z`
}

// --- Tooltip -----------------------------------------------------------------

export interface TipRow {
  value: string
  label: string
  color?: string
  line?: boolean
}

export interface TipState {
  x: number
  y: number
  title?: string
  rows: TipRow[]
}

/** Hover and focus tooltip state, positioned relative to a container. */
export function useTooltip<T extends HTMLElement>() {
  const containerRef = useRef<T>(null)
  const [tip, setTip] = useState<TipState | null>(null)

  const showAt = useCallback((clientX: number, clientY: number, title: string | undefined, rows: TipRow[]) => {
    const box = containerRef.current?.getBoundingClientRect()
    if (!box) return
    setTip({ x: clientX - box.left, y: clientY - box.top, title, rows })
  }, [])

  /** Handlers for a mark: pointer move/enter and keyboard focus. */
  const bind = useCallback(
    (title: string | undefined, rows: TipRow[]) => ({
      onPointerMove: (e: React.PointerEvent) => showAt(e.clientX, e.clientY, title, rows),
      onPointerLeave: () => setTip(null),
      onFocus: (e: React.FocusEvent<Element>) => {
        const r = (e.currentTarget as Element).getBoundingClientRect()
        showAt(r.left + r.width / 2, r.top, title, rows)
      },
      onBlur: () => setTip(null),
      tabIndex: 0,
      role: 'img' as const,
      'aria-label': [title, ...rows.map((r) => `${r.label}: ${r.value}`)].filter(Boolean).join('. '),
    }),
    [showAt],
  )

  return { containerRef, tip, setTip, showAt, bind }
}

/** The tooltip box: values lead, labels follow, keyed by short line marks. */
export function ChartTooltip({ tip, width }: { tip: TipState | null; width: number }) {
  if (!tip) return null
  const flip = tip.x > width - 190
  return (
    <div
      className="pointer-events-none absolute z-30 min-w-[140px] max-w-[260px] rounded-xl bg-surface px-3 py-2 text-xs shadow-[var(--shadow-pop)] ring-1 ring-line"
      style={{
        left: flip ? undefined : tip.x + 14,
        right: flip ? Math.max(4, width - tip.x + 14) : undefined,
        top: Math.max(0, tip.y - 12),
        transform: 'translateY(-100%)',
      }}
      role="status"
    >
      {tip.title && <div className="mb-1 font-medium text-ink-2">{tip.title}</div>}
      <div className="space-y-0.5">
        {tip.rows.map((r, i) => (
          <div key={i} className="flex items-center gap-2">
            {r.color && (
              <span
                className={r.line ? 'h-[2px] w-3 rounded-full' : 'size-2 rounded-[2px]'}
                style={{ background: r.color }}
                aria-hidden="true"
              />
            )}
            <span className="tabular font-semibold text-ink">{r.value}</span>
            <span className="truncate text-ink-2">{r.label}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

/** Legend: rect keys for bars/areas, line keys for lines. */
export function Legend({ items, className }: { items: { label: string; color: string; line?: boolean }[]; className?: string }) {
  return (
    <ul className={['flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-2', className].filter(Boolean).join(' ')}>
      {items.map((it) => (
        <li key={it.label} className="inline-flex items-center gap-1.5">
          <span
            className={it.line ? 'h-[2px] w-3.5 rounded-full' : 'size-2.5 rounded-[3px]'}
            style={{ background: it.color }}
            aria-hidden="true"
          />
          {it.label}
        </li>
      ))}
    </ul>
  )
}

/** Simple accessible table used as every chart's table view. */
export function DataTable({ columns, rows, caption }: { columns: { label: string; align?: 'left' | 'right' }[]; rows: ReactNode[][]; caption?: string }) {
  return (
    <div className="scroll-thin max-h-[360px] overflow-auto rounded-xl ring-1 ring-line">
      <table className="w-full border-collapse text-sm">
        {caption && <caption className="sr-only">{caption}</caption>}
        <thead className="sticky top-0 bg-surface-2">
          <tr>
            {columns.map((c, i) => (
              <th
                key={i}
                scope="col"
                className={`px-3 py-2 text-xs font-semibold text-ink-2 ${c.align === 'right' ? 'text-right' : 'text-left'}`}
              >
                {c.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} className="border-t border-grid">
              {r.map((cell, j) => (
                <td key={j} className={`tabular px-3 py-1.5 text-ink ${columns[j]?.align === 'right' ? 'text-right' : ''}`}>
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
