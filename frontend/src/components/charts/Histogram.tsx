import { useMemo } from 'react'
import { ChartTooltip, Legend, barPath, linear, useChartColors, useMeasure, useTooltip } from './core'
import { fmtCompact, fmtNumber, fmtPct, fmtValue, niceScale, niceTicks } from '@/lib/format'
import { STATUS, seriesColor } from '@/lib/palette'
import type { Band, Bin, Column, DistGroup } from '@/lib/types'

const M = { top: 22, right: 12, bottom: 30, left: 40 }

interface Props {
  column: Column
  bins: Bin[]
  overall: DistGroup
  groups?: DistGroup[]
  bands?: Band[]
  you?: number
  height?: number
}

/**
 * Histogram of one numeric column. With groups it switches to one line per
 * group (share of each group per bin) so groups of different sizes compare.
 */
export function Histogram({ column, bins, overall, groups, bands, you, height = 220 }: Props) {
  const [wrapRef, width] = useMeasure<HTMLDivElement>()
  const { containerRef, tip, bind, showAt, setTip } = useTooltip<HTMLDivElement>()
  const c = useChartColors()
  const split = groups && groups.length > 0

  const lo = bins[0]?.start ?? 0
  const hi = bins[bins.length - 1]?.end ?? 1
  const innerW = Math.max(0, width - M.left - M.right)
  const innerH = height - M.top - M.bottom
  const x = linear(lo, hi, M.left, M.left + innerW)

  const series = split ? groups! : [overall]
  const maxShare = Math.max(0.0001, ...series.flatMap((g) => g.shares))
  const yScale = niceScale(0, maxShare, 4)
  const yTicks = yScale.ticks
  const yMax = yScale.hi
  const y = linear(0, yMax, M.top + innerH, M.top)
  const xTicks = useMemo(() => niceTicks(lo, hi, Math.max(3, Math.floor(innerW / 90))), [lo, hi, innerW])
  const shownBands = (bands ?? []).filter((b) => !b.when)

  const binLabel = (i: number) =>
    `${fmtNumber(bins[i].start, column.decimals)}–${fmtValue(column, bins[i].end)}`

  return (
    <div ref={wrapRef} className="w-full">
      <div ref={containerRef} className="relative">
        {width > 0 && (
          <svg width={width} height={height} role="group" aria-label={`Distribution of ${column.label}`} className="block overflow-visible">
            {/* Clinical bands: faint status wash with a text label. */}
            {shownBands.map((b) => {
              const x0 = Math.max(M.left, x(b.min ?? lo))
              const x1 = Math.min(M.left + innerW, x(b.max ?? hi))
              if (x1 - x0 < 1) return null
              return (
                <g key={b.label}>
                  <rect x={x0} y={M.top} width={x1 - x0} height={innerH} fill={STATUS[b.tone]} opacity={c.mode === 'dark' ? 0.09 : 0.07} />
                  {x1 - x0 > b.label.length * 6 + 18 && (
                    <text x={x0 + 4} y={M.top - 7} fontSize={10.5} fill={c.ink2}>
                      <tspan fill={STATUS[b.tone]}>●</tspan> {b.label}
                    </text>
                  )}
                </g>
              )
            })}
            {/* Gridlines and y ticks */}
            {yTicks.map((t) => (
              <g key={t}>
                <line x1={M.left} x2={M.left + innerW} y1={y(t)} y2={y(t)} stroke={c.grid} strokeWidth={1} />
                <text x={M.left - 6} y={y(t)} dy="0.32em" textAnchor="end" fontSize={10.5} fill={c.muted} className="tabular">
                  {fmtPct(t, t < 0.1 && yMax < 0.1 ? 1 : 0)}
                </text>
              </g>
            ))}
            {/* x axis */}
            <line x1={M.left} x2={M.left + innerW} y1={M.top + innerH} y2={M.top + innerH} stroke={c.axis} strokeWidth={1} />
            {xTicks.map((t) => (
              <text key={t} x={x(t)} y={M.top + innerH + 17} textAnchor="middle" fontSize={10.5} fill={c.muted} className="tabular">
                {fmtCompact(Number(t.toFixed(column.decimals + 2)))}
              </text>
            ))}

            {!split &&
              bins.map((bin, i) => {
                const x0 = x(bin.start) + 1
                const w = Math.max(1, x(bin.end) - x(bin.start) - 2)
                const share = overall.shares[i] ?? 0
                const top = y(share)
                return (
                  <g key={i} {...bind(binLabel(i), [{ value: fmtPct(share), label: `of people (${fmtNumber(overall.counts[i])})` }])}>
                    <rect x={x(bin.start)} y={M.top} width={Math.max(1, x(bin.end) - x(bin.start))} height={innerH} fill="transparent" />
                    <path d={barPath(x0, top, w, M.top + innerH - top, 'up')} fill={seriesColor(c.mode, 0)} />
                  </g>
                )
              })}

            {split && (
              <>
                {series.map((g, gi) => {
                  const pts = bins.map((bin, i) => `${(x(bin.start) + x(bin.end)) / 2},${y(g.shares[i] ?? 0)}`)
                  return (
                    <polyline
                      key={g.key}
                      points={pts.join(' ')}
                      fill="none"
                      stroke={seriesColor(c.mode, gi)}
                      strokeWidth={2}
                      strokeLinejoin="round"
                      strokeLinecap="round"
                      opacity={g.lowN ? 0.45 : 1}
                    />
                  )
                })}
                {/* Crosshair layer: snaps to the nearest bin and lists every group. */}
                <rect
                  x={M.left}
                  y={M.top}
                  width={innerW}
                  height={innerH}
                  fill="transparent"
                  onPointerMove={(e) => {
                    const box = (e.currentTarget as SVGRectElement).getBoundingClientRect()
                    const v = lo + ((e.clientX - box.left) / box.width) * (hi - lo)
                    const i = Math.min(bins.length - 1, Math.max(0, bins.findIndex((b) => v < b.end)))
                    showAt(e.clientX, e.clientY, binLabel(i === -1 ? bins.length - 1 : i), series.map((g, gi) => ({
                      value: fmtPct(g.shares[i] ?? 0),
                      label: g.label,
                      color: seriesColor(c.mode, gi),
                      line: true,
                    })))
                  }}
                  onPointerLeave={() => setTip(null)}
                />
              </>
            )}

            {you !== undefined && you >= lo && you <= hi && (
              <g pointerEvents="none">
                <line x1={x(you)} x2={x(you)} y1={M.top - 2} y2={M.top + innerH} stroke={c.ink} strokeWidth={1.5} />
                <rect x={x(you) - 1} y={M.top - 2} width={2} height={2} fill={c.ink} />
                <text
                  x={x(you) + (x(you) > M.left + innerW - 90 ? -6 : 6)}
                  y={M.top + 12}
                  textAnchor={x(you) > M.left + innerW - 90 ? 'end' : 'start'}
                  fontSize={11}
                  fontWeight={600}
                  fill={c.ink}
                  stroke={c.surface}
                  strokeWidth={3}
                  paintOrder="stroke"
                >
                  You · {fmtValue(column, you)}
                </text>
              </g>
            )}
          </svg>
        )}
        <ChartTooltip tip={tip} width={width} />
      </div>
      {split && (
        <Legend className="mt-2" items={series.map((g, gi) => ({ label: g.lowN ? `${g.label} (few people)` : g.label, color: seriesColor(c.mode, gi), line: true }))} />
      )}
    </div>
  )
}
