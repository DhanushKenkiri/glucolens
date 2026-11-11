import { ChartTooltip, Legend, barPath, useChartColors, useMeasure, useTooltip } from './core'
import { fmtNumber, fmtPct } from '@/lib/format'
import { seriesColor } from '@/lib/palette'

const ROW = 30
const BAR = 16

/**
 * Horizontal bars for the levels of one column (one series, so one hue). When
 * `highlight` is set, that level keeps the accent and the rest step back to
 * gray: emphasis on the person's own category.
 */
export function LevelBars({ levels, shares, counts, highlight, labelWidth = 120 }: {
  levels: string[]
  shares: number[]
  counts?: number[]
  highlight?: string
  labelWidth?: number
}) {
  const [wrapRef, width] = useMeasure<HTMLDivElement>()
  const { containerRef, tip, bind } = useTooltip<HTMLDivElement>()
  const c = useChartColors()
  const valueW = 52
  const lw = Math.min(labelWidth, Math.max(70, width * 0.32))
  const innerW = Math.max(0, width - lw - valueW - 8)
  const max = Math.max(0.0001, ...shares)
  const height = levels.length * ROW

  return (
    <div ref={wrapRef} className="w-full">
      <div ref={containerRef} className="relative">
        {width > 0 && (
          <svg width={width} height={height} className="block overflow-visible" role="group" aria-label="Share of people by value">
            {levels.map((l, i) => {
              const yMid = i * ROW + ROW / 2
              const w = (shares[i] / max) * innerW
              const isYou = highlight !== undefined && l === highlight
              const fill = highlight === undefined || isYou ? seriesColor(c.mode, 0) : c.deEmphasis
              return (
                <g key={l} {...bind(l, [{ value: fmtPct(shares[i]), label: counts ? `of people (${fmtNumber(counts[i])})` : 'of people' }])}>
                  <rect x={0} y={i * ROW} width={width} height={ROW} fill="transparent" />
                  <text x={lw - 10} y={yMid} dy="0.32em" textAnchor="end" fontSize={12} fill={isYou ? c.ink : c.ink2} fontWeight={isYou ? 600 : 400}>
                    {truncate(l, Math.floor(lw / 7))}
                  </text>
                  <rect x={lw} y={yMid - BAR / 2} width={innerW} height={BAR} rx={4} fill={c.grid} opacity={0.35} />
                  <path d={barPath(lw, yMid - BAR / 2, Math.max(w, shares[i] > 0 ? 2 : 0), BAR, 'right')} fill={fill} />
                  <text x={lw + innerW + 8} y={yMid} dy="0.32em" fontSize={12} fill={c.ink} className="tabular" fontWeight={isYou ? 600 : 400}>
                    {fmtPct(shares[i], shares[i] < 0.1 ? 1 : 0)}
                  </text>
                  {isYou && w > 34 && (
                    <text x={lw + 6} y={yMid} dy="0.32em" fontSize={10.5} fontWeight={600} fill="#ffffff">
                      You
                    </text>
                  )}
                </g>
              )
            })}
          </svg>
        )}
        <ChartTooltip tip={tip} width={width} />
      </div>
    </div>
  )
}

/**
 * One 100% bar per group, split into the column's levels (part-to-whole for
 * several wholes on one axis). Level i always takes categorical slot i.
 */
export function StackedShareBars({ groups: rawGroups, levels: rawLevels, labelWidth = 110 }: {
  groups: { label: string; n: number; shares: number[]; lowN?: boolean }[]
  levels: string[]
  labelWidth?: number
}) {
  const { levels, groups } = foldLevels(rawLevels, rawGroups)
  const [wrapRef, width] = useMeasure<HTMLDivElement>()
  const { containerRef, tip, bind } = useTooltip<HTMLDivElement>()
  const c = useChartColors()
  const lw = Math.min(labelWidth, Math.max(70, width * 0.3))
  const innerW = Math.max(0, width - lw - 4)
  const rowH = 34
  const barH = 22
  const height = groups.length * rowH

  return (
    <div ref={wrapRef} className="w-full">
      <div ref={containerRef} className="relative">
        {width > 0 && (
          <svg width={width} height={height} className="block" role="group" aria-label="Share of each value within each group">
            {groups.map((g, gi) => {
              const y0 = gi * rowH + (rowH - barH) / 2
              let acc = 0
              const total = g.shares.reduce((a, b) => a + b, 0) || 1
              return (
                <g key={g.label}>
                  <text x={lw - 10} y={y0 + barH / 2} dy="0.32em" textAnchor="end" fontSize={12} fill={c.ink2}>
                    {truncate(g.label, Math.floor(lw / 7))}
                  </text>
                  {g.shares.map((s, li) => {
                    const share = s / total
                    const x0 = lw + acc * innerW
                    const w = share * innerW
                    acc += share
                    if (w <= 0.5) return null
                    const gap = 2
                    const fill = seriesColor(c.mode, li)
                    const textFill = c.mode === 'light' && li === 3 ? '#0b0b0b' : '#ffffff'
                    const label = fmtPct(share, 0)
                    return (
                      <g key={li} {...bind(`${g.label} · ${levels[li]}`, [{ value: fmtPct(share), label: `of ${fmtNumber(g.n)} people`, color: fill }])}>
                        <rect x={x0} y={y0} width={Math.max(0.5, w - (acc < 0.999 ? gap : 0))} height={barH} rx={4} fill={fill} opacity={g.lowN ? 0.5 : 1} />
                        {w > label.length * 7 + 10 && (
                          <text x={x0 + 6} y={y0 + barH / 2} dy="0.34em" fontSize={10.5} fontWeight={600} fill={textFill} pointerEvents="none">
                            {label}
                          </text>
                        )}
                      </g>
                    )
                  })}
                </g>
              )
            })}
          </svg>
        )}
        <ChartTooltip tip={tip} width={width} />
      </div>
      <Legend className="mt-2" items={levels.map((l, i) => ({ label: l, color: seriesColor(c.mode, i) }))} />
    </div>
  )
}

/**
 * A list of rates, one per named row: name, a thin bar on a lighter track
 * and the figure. Used for yes/no columns and outcome rates.
 */
export function MeterList({ rows, scaleMax, highlight }: {
  rows: { label: string; value: number; low?: number; high?: number; n?: number; lowN?: boolean; note?: string }[]
  scaleMax?: number
  highlight?: string
}) {
  const c = useChartColors()
  const max = scaleMax ?? Math.max(0.0001, ...rows.map((r) => r.high ?? r.value))
  return (
    <ul className="space-y-2.5">
      {rows.map((r) => {
        const muted = r.lowN || (highlight !== undefined && r.label !== highlight)
        return (
          <li key={r.label} className="grid grid-cols-[minmax(0,9rem)_1fr_auto] items-center gap-3 text-sm">
            <span className={`truncate ${highlight === r.label ? 'font-semibold text-ink' : 'text-ink-2'}`} title={r.label}>
              {r.label}
            </span>
            <span className="relative h-2 overflow-hidden rounded-full" style={{ background: c.mode === 'dark' ? '#1c2a3d' : '#e6effa' }}>
              <span
                className="absolute inset-y-0 left-0 rounded-full"
                style={{ width: `${Math.min(100, (r.value / max) * 100)}%`, background: muted ? c.deEmphasis : seriesColor(c.mode, 0) }}
              />
            </span>
            <span className="tabular text-right text-ink">
              {fmtPct(r.value)}
              {r.n !== undefined && <span className="ml-1.5 text-xs text-muted">n={fmtNumber(r.n)}</span>}
            </span>
          </li>
        )
      })}
    </ul>
  )
}

const MAX_SLOTS = 8

/** Keeps at most eight categories: the tail folds into "7+" (numbers) or "Other". */
export function foldLevels<G extends { shares: number[] }>(levels: string[], groups: G[]): { levels: string[]; groups: G[] } {
  if (levels.length <= MAX_SLOTS) return { levels, groups }
  const k = MAX_SLOTS - 1
  const tail = /^[0-9]/.test(levels[k]) ? `${levels[k]}+` : 'Other'
  return {
    levels: [...levels.slice(0, k), tail],
    groups: groups.map((g) => ({ ...g, shares: [...g.shares.slice(0, k), g.shares.slice(k).reduce((a, b) => a + b, 0)] })),
  }
}

export function truncate(s: string, n: number) {
  return s.length > n ? s.slice(0, Math.max(1, n - 1)) + '…' : s
}
