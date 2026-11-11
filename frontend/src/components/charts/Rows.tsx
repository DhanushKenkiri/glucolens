import { ChartTooltip, Legend, barPath, linear, useChartColors, useMeasure, useTooltip } from './core'
import { truncate } from './Bars'
import { fmtNumber, fmtPct, fmtRatio, fmtValue, niceScale, niceTicks, ordinal } from '@/lib/format'
import { STATUS, seriesColor } from '@/lib/palette'
import type { Column, ProfileItem, RiskLevel, Strength, Summary } from '@/lib/types'

const AXIS_H = 22

function Axis({ ticks, x, y, color, format }: { ticks: number[]; x: (v: number) => number; y: number; color: string; format: (v: number) => string }) {
  return (
    <g>
      {ticks.map((t) => (
        <text key={t} x={x(t)} y={y + 14} textAnchor="middle" fontSize={10.5} fill={color} className="tabular">
          {format(t)}
        </text>
      ))}
    </g>
  )
}

/** Horizontal box plots: middle half as a box, 5th–95th percentile whiskers, median tick. */
export function BoxPlots({ rows, column, you, labelWidth = 120 }: {
  rows: { label: string; summary?: Summary; lowN?: boolean }[]
  column: Column
  you?: number
  labelWidth?: number
}) {
  const [wrapRef, width] = useMeasure<HTMLDivElement>()
  const { containerRef, tip, bind } = useTooltip<HTMLDivElement>()
  const c = useChartColors()
  const rowH = 34
  const lw = Math.min(labelWidth, Math.max(70, width * 0.3))
  const right = 12
  const valid = rows.filter((r) => r.summary)
  const lo = Math.min(...valid.map((r) => r.summary!.p5), you ?? Infinity)
  const hi = Math.max(...valid.map((r) => r.summary!.p95), you ?? -Infinity)
  const ticks = niceTicks(lo, hi, Math.max(3, Math.floor((width - lw) / 90)))
  const d0 = Math.min(lo, ticks[0] ?? lo)
  const d1 = Math.max(hi, ticks[ticks.length - 1] ?? hi)
  const x = linear(d0, d1, lw, width - right)
  const plotH = rows.length * rowH
  const boxFill = c.mode === 'dark' ? '#1c5cab' : '#9ec5f4'

  return (
    <div ref={wrapRef} className="w-full">
      <div ref={containerRef} className="relative">
        {width > 0 && valid.length > 0 && (
          <svg width={width} height={plotH + AXIS_H} className="block overflow-visible" role="group" aria-label={`${column.label} by group`}>
            {ticks.map((t) => (
              <line key={t} x1={x(t)} x2={x(t)} y1={0} y2={plotH} stroke={c.grid} strokeWidth={1} />
            ))}
            {rows.map((r, i) => {
              const s = r.summary
              const yMid = i * rowH + rowH / 2
              if (!s) return null
              const tipRows = [
                { value: fmtValue(column, s.median), label: 'median' },
                { value: `${fmtValue(column, s.p25)} – ${fmtValue(column, s.p75)}`, label: 'middle half' },
                { value: `${fmtValue(column, s.p5)} – ${fmtValue(column, s.p95)}`, label: '5th–95th pct' },
                { value: fmtNumber(s.n), label: 'people' },
              ]
              return (
                <g key={r.label} {...bind(r.label, tipRows)} opacity={r.lowN ? 0.5 : 1}>
                  <rect x={0} y={i * rowH} width={width} height={rowH} fill="transparent" />
                  <text x={lw - 10} y={yMid} dy="0.32em" textAnchor="end" fontSize={12} fill={c.ink2}>
                    {truncate(r.label, Math.floor(lw / 7))}
                  </text>
                  <line x1={x(s.p5)} x2={x(s.p95)} y1={yMid} y2={yMid} stroke={c.ink2} strokeWidth={1.5} />
                  <line x1={x(s.p5)} x2={x(s.p5)} y1={yMid - 5} y2={yMid + 5} stroke={c.ink2} strokeWidth={1.5} />
                  <line x1={x(s.p95)} x2={x(s.p95)} y1={yMid - 5} y2={yMid + 5} stroke={c.ink2} strokeWidth={1.5} />
                  <rect x={x(s.p25)} y={yMid - 9} width={Math.max(2, x(s.p75) - x(s.p25))} height={18} rx={4} fill={boxFill} />
                  <line x1={x(s.median)} x2={x(s.median)} y1={yMid - 9} y2={yMid + 9} stroke={c.ink} strokeWidth={2} />
                </g>
              )
            })}
            {you !== undefined && (
              <g pointerEvents="none">
                <line x1={x(you)} x2={x(you)} y1={-4} y2={plotH} stroke={c.ink} strokeWidth={1.5} />
                <text x={x(you) + 5} y={-6} fontSize={11} fontWeight={600} fill={c.ink} stroke={c.surface} strokeWidth={3} paintOrder="stroke">
                  You
                </text>
              </g>
            )}
            <line x1={lw} x2={width - right} y1={plotH} y2={plotH} stroke={c.axis} />
            <Axis ticks={ticks} x={x} y={plotH} color={c.muted} format={(v) => fmtNumber(v, column.decimals)} />
          </svg>
        )}
        <ChartTooltip tip={tip} width={width} />
      </div>
      <Legend
        className="mt-2"
        items={[
          { label: 'Middle half (25th–75th percentile)', color: boxFill },
          { label: 'Median', color: c.ink, line: true },
          { label: '5th–95th percentile', color: c.ink2, line: true },
        ]}
      />
    </div>
  )
}

/**
 * Outcome rate per group with its 95% confidence interval (a forest plot),
 * a hairline at the overall rate, and relative risk against the reference.
 */
/**
 * The x domain for rate dots: fitted to the intervals (dots encode position,
 * not length, so the axis need not start at zero) but never narrower than 20
 * percentage points, so small differences are not blown up.
 */
export function rateDomain(levels: RiskLevel[], base?: number): [number, number] {
  const shown = levels.filter((l) => l.rate.n > 0)
  let lo = Math.min(...shown.map((l) => l.rate.low), base ?? Infinity)
  let hi = Math.max(...shown.map((l) => l.rate.high), base ?? -Infinity)
  if (!Number.isFinite(lo) || !Number.isFinite(hi)) return [0, 1]
  if (hi - lo < 0.2) {
    const mid = (lo + hi) / 2
    lo = mid - 0.1
    hi = mid + 0.1
  }
  if (lo < 0) {
    hi -= lo
    lo = 0
  }
  if (hi > 1) {
    lo -= hi - 1
    hi = 1
  }
  const s = niceScale(Math.max(0, lo), Math.min(1, hi), 4)
  return [Math.max(0, s.lo), Math.min(1, s.hi)]
}

export function RateRows({ levels, base, labelWidth = 120, compact = false, showRR = true, domain }: {
  levels: RiskLevel[]
  base?: number
  labelWidth?: number
  compact?: boolean
  showRR?: boolean
  domain?: [number, number]
}) {
  const [wrapRef, width] = useMeasure<HTMLDivElement>()
  const { containerRef, tip, bind } = useTooltip<HTMLDivElement>()
  const c = useChartColors()
  const rowH = compact ? 26 : 34
  const narrow = width < 460
  const lw = Math.min(labelWidth, Math.max(64, width * (compact ? 0.34 : 0.26)))
  const valueW = compact ? 50 : showRR && !narrow ? 150 : 64
  const plotR = width - valueW - 8
  const dom = domain ?? rateDomain(levels, base)
  const ticks = niceTicks(dom[0], dom[1], compact ? 2 : 4)
  const x = linear(dom[0], dom[1], lw, plotR)
  const plotH = levels.length * rowH
  const dot = seriesColor(c.mode, 0)

  return (
    <div ref={wrapRef} className="w-full">
      <div ref={containerRef} className="relative">
        {width > 0 && (
          <svg width={width} height={plotH + (compact ? 4 : AXIS_H)} className="block overflow-visible" role="group" aria-label="Rate by group with 95% confidence intervals">
            {!compact &&
              ticks.map((t) => <line key={t} x1={x(t)} x2={x(t)} y1={0} y2={plotH} stroke={c.grid} strokeWidth={1} />)}
            {base !== undefined && (
              <g>
                <line x1={x(base)} x2={x(base)} y1={-2} y2={plotH} stroke={c.muted} strokeWidth={1} />
                {!compact && (
                  <text x={x(base)} y={-6} textAnchor="middle" fontSize={10.5} fill={c.muted}>
                    Overall {fmtPct(base)}
                  </text>
                )}
              </g>
            )}
            {levels.map((l, i) => {
              const yMid = i * rowH + rowH / 2
              const muted = l.lowN || l.rate.n === 0
              const color = muted ? c.deEmphasis : dot
              const rr = l.relativeRisk
              const tipRows = [
                { value: fmtPct(l.rate.value), label: `rate (${fmtNumber(l.rate.events)} of ${fmtNumber(l.rate.n)})` },
                { value: `${fmtPct(l.rate.low)} – ${fmtPct(l.rate.high)}`, label: '95% confidence interval' },
                ...(rr ? [{ value: fmtRatio(rr.value), label: `vs reference (${rr.low.toFixed(2)}–${rr.high.toFixed(2)})` }] : []),
              ]
              return (
                <g key={l.key} {...bind(l.label, tipRows)}>
                  <rect x={0} y={i * rowH} width={width} height={rowH} fill="transparent" />
                  <text x={lw - 10} y={yMid} dy="0.32em" textAnchor="end" fontSize={compact ? 11 : 12} fill={l.isReference ? c.ink : c.ink2} fontWeight={l.isReference ? 600 : 400}>
                    {truncate(l.label, Math.floor(lw / 6.6))}
                  </text>
                  {l.rate.n > 0 && (
                    <>
                      <line x1={x(l.rate.low)} x2={x(l.rate.high)} y1={yMid} y2={yMid} stroke={color} strokeWidth={2} strokeLinecap="round" />
                      <circle cx={x(l.rate.value)} cy={yMid} r={compact ? 4 : 5} fill={color} stroke={c.surface} strokeWidth={2} />
                    </>
                  )}
                  <text x={plotR + 8} y={yMid} dy="0.32em" fontSize={compact ? 11 : 12} fill={c.ink} className="tabular">
                    {l.rate.n > 0 ? fmtPct(l.rate.value) : '–'}
                    {!compact && showRR && !narrow && (
                      <tspan fill={c.ink2} dx={8}>
                        {l.isReference ? 'reference' : rr ? `${fmtRatio(rr.value)} risk` : ''}
                      </tspan>
                    )}
                  </text>
                </g>
              )
            })}
            {!compact && (
              <>
                <line x1={lw} x2={plotR} y1={plotH} y2={plotH} stroke={c.axis} />
                <Axis ticks={ticks} x={x} y={plotH} color={c.muted} format={(v) => fmtPct(v, 0)} />
              </>
            )}
          </svg>
        )}
        <ChartTooltip tip={tip} width={width} />
      </div>
    </div>
  )
}

/** Factors ranked by strength of link (Cramér's V), strongest first. */
export function RankBars({ factors, selected, onSelect }: {
  factors: { column: string; label: string; v: number; strength: Strength; detail: string }[]
  selected?: string
  onSelect?: (column: string) => void
}) {
  const c = useChartColors()
  const max = Math.max(0.3, ...factors.map((f) => f.v))
  return (
    <ul className="divide-y divide-[var(--grid)]">
      {factors.map((f) => {
        const isSel = f.column === selected
        const none = f.strength === 'none'
        return (
          <li key={f.column}>
            <button
              type="button"
              onClick={() => onSelect?.(f.column)}
              aria-pressed={isSel}
              className={`grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 rounded-xl px-3 py-2.5 text-left transition-colors ${isSel ? 'bg-accent-soft' : 'hover:bg-surface-2'}`}
            >
              <span className={`truncate text-sm ${isSel ? 'font-semibold text-ink' : 'text-ink'}`}>{f.label}</span>
              <span className="tabular text-xs text-ink-2">V {f.v.toFixed(2)}</span>
              <span className="col-span-2 flex items-center gap-3">
                <svg width="100%" height={8} className="min-w-0 flex-1" preserveAspectRatio="none" viewBox="0 0 100 8" aria-hidden="true">
                  <rect x={0} y={0} width={100} height={8} rx={4} fill={c.grid} opacity={0.5} />
                  <path d={barPath(0, 0, Math.max(1.5, (f.v / max) * 100), 8, 'right', 4)} fill={none ? c.deEmphasis : seriesColor(c.mode, 0)} />
                </svg>
              </span>
              <span className="col-span-2 truncate text-xs text-muted">{f.detail}</span>
            </button>
          </li>
        )
      })}
    </ul>
  )
}

/** Two rates per row: everyone (light) and people like you (accent). */
export function Dumbbells({ rows }: { rows: { label: string; overall: number; similar: number; n: number }[] }) {
  const [wrapRef, width] = useMeasure<HTMLDivElement>()
  const { containerRef, tip, bind } = useTooltip<HTMLDivElement>()
  const c = useChartColors()
  const rowH = 34
  const lw = Math.min(170, Math.max(90, width * 0.34))
  const valueW = 92
  const plotR = width - valueW
  const x = linear(0, 1, lw, plotR)
  const light = c.mode === 'dark' ? '#184f95' : '#9ec5f4'
  const strong = seriesColor(c.mode, 0)
  const plotH = rows.length * rowH
  return (
    <div ref={wrapRef} className="w-full">
      <div ref={containerRef} className="relative">
        {width > 0 && (
          <svg width={width} height={plotH + AXIS_H} className="block overflow-visible" role="group" aria-label="Outcome rates among similar people compared with everyone">
            {[0, 0.25, 0.5, 0.75, 1].map((t) => (
              <line key={t} x1={x(t)} x2={x(t)} y1={0} y2={plotH} stroke={c.grid} />
            ))}
            {rows.map((r, i) => {
              const yMid = i * rowH + rowH / 2
              return (
                <g
                  key={r.label}
                  {...bind(r.label, [
                    { value: fmtPct(r.similar), label: `people like you (${fmtNumber(r.n)})`, color: strong },
                    { value: fmtPct(r.overall), label: 'everyone', color: light },
                  ])}
                >
                  <rect x={0} y={i * rowH} width={width} height={rowH} fill="transparent" />
                  <text x={lw - 10} y={yMid} dy="0.32em" textAnchor="end" fontSize={12} fill={c.ink2}>
                    {truncate(r.label, Math.floor(lw / 6.6))}
                  </text>
                  <line x1={x(r.overall)} x2={x(r.similar)} y1={yMid} y2={yMid} stroke={c.axis} strokeWidth={2} />
                  <circle cx={x(r.overall)} cy={yMid} r={5} fill={light} stroke={c.surface} strokeWidth={2} />
                  <circle cx={x(r.similar)} cy={yMid} r={5.5} fill={strong} stroke={c.surface} strokeWidth={2} />
                  <text x={plotR + 10} y={yMid} dy="0.32em" fontSize={12} fill={c.ink} className="tabular">
                    {fmtPct(r.similar)}
                    <tspan fill={c.muted} dx={6}>
                      vs {fmtPct(r.overall, 0)}
                    </tspan>
                  </text>
                </g>
              )
            })}
            <line x1={lw} x2={plotR} y1={plotH} y2={plotH} stroke={c.axis} />
            <Axis ticks={[0, 0.25, 0.5, 0.75, 1]} x={x} y={plotH} color={c.muted} format={(v) => fmtPct(v, 0)} />
          </svg>
        )}
        <ChartTooltip tip={tip} width={width} />
      </div>
      <Legend className="mt-2" items={[{ label: 'People like you', color: strong }, { label: 'Everyone', color: light }]} />
    </div>
  )
}

/** Where each value sits among the comparison group: a 0–100 percentile strip. */
export function PercentileStrips({ items }: { items: ProfileItem[] }) {
  const c = useChartColors()
  const dot = seriesColor(c.mode, 0)
  return (
    <ul className="space-y-3">
      {items.map((it) => (
        <li key={it.column} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1.5">
          <div className="min-w-0 truncate text-sm text-ink">
            {it.label} <span className="font-semibold">{it.display}</span>
          </div>
          <div className="flex items-center gap-2 text-xs text-ink-2">
            {it.band && (
              <span className="inline-flex items-center gap-1">
                <span className="size-2 rounded-full" style={{ background: STATUS[it.band.tone] }} aria-hidden="true" />
                {it.band.label}
              </span>
            )}
          </div>
          {it.percentile !== undefined ? (
            <div className="col-span-2 flex items-center gap-3">
              <div className="relative h-2.5 flex-1 rounded-full" style={{ background: c.mode === 'dark' ? '#1c2a3d' : '#e6effa' }} aria-hidden="true">
                <div className="absolute inset-y-0 rounded-full" style={{ left: '25%', width: '50%', background: c.mode === 'dark' ? '#184f95' : '#b7d3f6' }} />
                <div
                  className="absolute top-1/2 size-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full"
                  style={{ left: `${Math.min(100, Math.max(0, it.percentile))}%`, background: dot, boxShadow: `0 0 0 2px ${c.surface}` }}
                />
              </div>
              <span className="tabular w-24 shrink-0 text-right text-xs text-ink-2">{ordinal(it.percentile)} percentile</span>
            </div>
          ) : it.share !== undefined ? (
            <div className="col-span-2 text-xs text-ink-2">{fmtPct(it.share)} of the group share this value</div>
          ) : null}
        </li>
      ))}
    </ul>
  )
}
