import { ChartTooltip, Legend, linear, useChartColors, useMeasure, useTooltip } from './core'
import { truncate } from './Bars'
import { fmtNumber, fmtValue, niceTicks } from '@/lib/format'
import { diverging, seriesColor, sequential } from '@/lib/palette'
import type { CorrelationResponse, RelationshipResponse } from '@/lib/types'

/**
 * Correlation matrix (lower triangle). Diverging scale: blue for negative,
 * red for positive, neutral gray at zero. Click a cell to explore the pair.
 */
export function CorrHeatmap({ data, selected, onSelect }: {
  data: CorrelationResponse
  selected?: [string, string]
  onSelect?: (a: string, b: string) => void
}) {
  const [wrapRef, width] = useMeasure<HTMLDivElement>()
  const { containerRef, tip, showAt, setTip } = useTooltip<HTMLDivElement>()
  const c = useChartColors()
  const n = data.columns.length
  const lw = 118
  const cell = Math.max(14, Math.min(28, Math.floor((width - lw - 8) / (n - 1))))
  const gridW = cell * (n - 1)
  const bottom = 92
  const height = cell * (n - 1) + bottom
  const svgW = lw + gridW + 8
  const isSel = (a: string, b: string) => !!selected && ((selected[0] === a && selected[1] === b) || (selected[0] === b && selected[1] === a))

  return (
    <div ref={wrapRef} className="w-full">
      <div ref={containerRef} className="scroll-thin relative overflow-x-auto">
        {width > 0 && (
          <svg width={svgW} height={height} className="block" role="img" aria-label="Correlation matrix of numeric columns; the strongest pairs are listed beside it">
            {data.columns.slice(1).map((row, ri) => {
              const i = ri + 1
              return (
                <g key={row.name}>
                  <text x={lw - 8} y={ri * cell + cell / 2} dy="0.32em" textAnchor="end" fontSize={11} fill={c.ink2}>
                    {truncate(row.short, 16)}
                  </text>
                  {data.columns.slice(0, i).map((col, j) => {
                    const r = data.matrix[i][j]
                    const sel = isSel(row.name, col.name)
                    return (
                      <rect
                        key={col.name}
                        x={lw + j * cell + 1}
                        y={ri * cell + 1}
                        width={cell - 2}
                        height={cell - 2}
                        rx={3}
                        fill={diverging(c.mode, r)}
                        stroke={sel ? c.ink : 'none'}
                        strokeWidth={sel ? 2 : 0}
                        className="cursor-pointer"
                        onPointerMove={(e) =>
                          showAt(e.clientX, e.clientY, `${row.label} × ${col.label}`, [{ value: `r = ${r.toFixed(2)}`, label: 'correlation' }])
                        }
                        onPointerLeave={() => setTip(null)}
                        onClick={() => onSelect?.(row.name, col.name)}
                      />
                    )
                  })}
                </g>
              )
            })}
            {data.columns.slice(0, n - 1).map((col, j) => {
              const cx = lw + j * cell + cell / 2
              const cy = (n - 1) * cell + 6
              return (
                <text key={col.name} x={cx} y={cy} fontSize={11} fill={c.ink2} textAnchor="end" transform={`rotate(-55 ${cx} ${cy})`}>
                  {truncate(col.short, 16)}
                </text>
              )
            })}
          </svg>
        )}
        <ChartTooltip tip={tip} width={width} />
      </div>
      <div className="mt-2 flex items-center gap-2 text-xs text-ink-2">
        <span>−1</span>
        <span
          className="h-2 w-40 rounded-full"
          style={{ background: `linear-gradient(90deg, ${[-1, -0.5, 0, 0.5, 1].map((v) => diverging(c.mode, v)).join(',')})` }}
          aria-hidden="true"
        />
        <span>+1</span>
        <span className="ml-2 text-muted">Blue: one falls as the other rises · Red: both rise together</span>
      </div>
    </div>
  )
}

/** 2-D density of two numeric columns with the median trend and middle half. */
export function DensityPlot({ data, height = 300 }: { data: RelationshipResponse; height?: number }) {
  const [wrapRef, width] = useMeasure<HTMLDivElement>()
  const { containerRef, tip, showAt, setTip } = useTooltip<HTMLDivElement>()
  const c = useChartColors()
  const M = { top: 24, right: 12, bottom: 34, left: 48 }
  const innerW = Math.max(0, width - M.left - M.right)
  const innerH = height - M.top - M.bottom
  const xb = data.xBins
  const yb = data.yBins
  const x = linear(xb[0].start, xb[xb.length - 1].end, M.left, M.left + innerW)
  const y = linear(yb[0].start, yb[yb.length - 1].end, M.top + innerH, M.top)
  const xTicks = niceTicks(xb[0].start, xb[xb.length - 1].end, Math.max(3, Math.floor(innerW / 90)))
  const yTicks = niceTicks(yb[0].start, yb[yb.length - 1].end, 5)
  const trendColor = seriesColor(c.mode, 1)
  const sorted = [...data.trend].sort((a, b) => a.x - b.x)
  const xCol = { decimals: data.x.decimals, unit: data.x.unit }
  const yCol = { decimals: data.y.decimals, unit: data.y.unit }
  const band =
    sorted.length > 1
      ? `M${sorted.map((t) => `${x(t.x)},${y(t.p75)}`).join('L')}L${[...sorted].reverse().map((t) => `${x(t.x)},${y(t.p25)}`).join('L')}Z`
      : ''

  return (
    <div ref={wrapRef} className="w-full">
      <div ref={containerRef} className="relative">
        {width > 0 && (
          <svg width={width} height={height} className="block" role="img" aria-label={`${data.y.label} against ${data.x.label}. ${data.summary}`}>
            {data.cells.map((cl) => {
              const t = Math.sqrt(cl.count / Math.max(1, data.maxCount))
              const x0 = x(xb[cl.x].start)
              const x1 = x(xb[cl.x].end)
              const y0 = y(yb[cl.y].end)
              const y1 = y(yb[cl.y].start)
              return <rect key={`${cl.x}-${cl.y}`} x={x0} y={y0} width={Math.max(0.5, x1 - x0)} height={Math.max(0.5, y1 - y0)} fill={sequential(c.mode, 0.08 + t * 0.92)} />
            })}
            {yTicks.map((t) => (
              <text key={t} x={M.left - 6} y={y(t)} dy="0.32em" textAnchor="end" fontSize={10.5} fill={c.muted} className="tabular">
                {fmtNumber(t, data.y.decimals)}
              </text>
            ))}
            {xTicks.map((t) => (
              <text key={t} x={x(t)} y={M.top + innerH + 16} textAnchor="middle" fontSize={10.5} fill={c.muted} className="tabular">
                {fmtNumber(t, data.x.decimals)}
              </text>
            ))}
            <line x1={M.left} x2={M.left + innerW} y1={M.top + innerH} y2={M.top + innerH} stroke={c.axis} />
            <text x={M.left - 40} y={12} fontSize={11} fill={c.ink2}>
              ↑ {data.y.label}
              {data.y.unit ? ` (${data.y.unit})` : ''}
            </text>
            <text x={M.left + innerW} y={height - 2} textAnchor="end" fontSize={11} fill={c.ink2}>
              {data.x.label}
              {data.x.unit ? ` (${data.x.unit})` : ''} →
            </text>
            {band && <path d={band} fill={trendColor} opacity={0.14} />}
            {sorted.length > 1 && (
              <polyline
                points={sorted.map((t) => `${x(t.x)},${y(t.median)}`).join(' ')}
                fill="none"
                stroke={trendColor}
                strokeWidth={2}
                strokeLinejoin="round"
                strokeLinecap="round"
              />
            )}
            <rect
              x={M.left}
              y={M.top}
              width={innerW}
              height={innerH}
              fill="transparent"
              onPointerMove={(e) => {
                const box = (e.currentTarget as SVGRectElement).getBoundingClientRect()
                const v = xb[0].start + ((e.clientX - box.left) / box.width) * (xb[xb.length - 1].end - xb[0].start)
                let best = sorted[0]
                for (const t of sorted) if (Math.abs(t.x - v) < Math.abs(best.x - v)) best = t
                if (!best) return
                const bin = xb[best.bin]
                showAt(e.clientX, e.clientY, `${data.x.label} ${fmtNumber(bin.start, data.x.decimals)}–${fmtValue(xCol, bin.end)}`, [
                  { value: fmtValue(yCol, best.median), label: `median ${data.y.label}`, color: trendColor, line: true },
                  { value: `${fmtValue(yCol, best.p25)} – ${fmtValue(yCol, best.p75)}`, label: 'middle half' },
                  { value: fmtNumber(best.n), label: 'people' },
                ])
              }}
              onPointerLeave={() => setTip(null)}
            />
          </svg>
        )}
        <ChartTooltip tip={tip} width={width} />
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1">
        <Legend
          items={[
            { label: `Median ${data.y.label}`, color: trendColor, line: true },
            { label: 'Middle half', color: trendColor },
          ]}
        />
        <span className="flex items-center gap-1.5 text-xs text-ink-2">
          <span
            className="h-2 w-16 rounded-full"
            style={{ background: `linear-gradient(90deg, ${[0.08, 0.4, 0.7, 1].map((t) => sequential(c.mode, t)).join(',')})` }}
            aria-hidden="true"
          />
          More people
        </span>
      </div>
    </div>
  )
}
