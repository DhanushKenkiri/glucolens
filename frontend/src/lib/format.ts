import type { Column, Strength } from './types'

const nf = new Map<string, Intl.NumberFormat>()
function formatter(maxDecimals: number, minDecimals = 0, compact = false) {
  const key = `${maxDecimals}|${minDecimals}|${compact}`
  let f = nf.get(key)
  if (!f) {
    f = new Intl.NumberFormat('en-US', {
      maximumFractionDigits: maxDecimals,
      minimumFractionDigits: minDecimals,
      notation: compact ? 'compact' : 'standard',
    })
    nf.set(key, f)
  }
  return f
}

/** 100000 -> "100,000"; 1234.5 -> "1,234.5". */
export function fmtNumber(v: number | undefined | null, decimals = 0): string {
  if (v === undefined || v === null || !Number.isFinite(v)) return '–'
  return formatter(decimals).format(v)
}

/** 12900 -> "12.9K" (axis ticks that reach 10,000). */
export function fmtCompact(v: number): string {
  if (Math.abs(v) < 10000) return formatter(0).format(v)
  return formatter(1, 0, true).format(v)
}

/** 0.6012 -> "60.1%". */
export function fmtPct(v: number | undefined | null, decimals = 1): string {
  if (v === undefined || v === null || !Number.isFinite(v)) return '–'
  return `${(v * 100).toFixed(decimals)}%`
}

/** A value of a column with its unit: "6.1%", "128 mmHg", "7.5 h/day". */
export function fmtValue(col: Pick<Column, 'decimals' | 'unit'> | undefined, v: number | undefined | null): string {
  if (v === undefined || v === null || !Number.isFinite(v)) return '–'
  const s = fmtNumber(v, col?.decimals ?? 2)
  const unit = col?.unit
  if (!unit) return s
  if (unit === '%' || unit.startsWith('/')) return s + unit
  return `${s} ${unit}`
}

/** Display any patient value: numbers with units, categories as is. */
export function fmtAny(col: Column | undefined, v: number | string | undefined | null): string {
  if (v === undefined || v === null || v === '') return '–'
  if (typeof v === 'string') return v
  if (col?.kind === 'binary') return col.levels?.[v >= 0.5 ? 1 : 0] ?? String(v)
  return fmtValue(col, v)
}

export function fmtPValue(p: number): string {
  if (!Number.isFinite(p)) return '–'
  if (p < 0.001) return 'p < 0.001'
  return `p = ${p.toFixed(3)}`
}

export function fmtRatio(v: number): string {
  if (!Number.isFinite(v)) return '–'
  return `${v.toFixed(2)}×`
}

/** 1 -> "1st", 62 -> "62nd". */
export function ordinal(n: number): string {
  const r = Math.round(n)
  const mod100 = r % 100
  if (mod100 >= 11 && mod100 <= 13) return `${r}th`
  switch (r % 10) {
    case 1:
      return `${r}st`
    case 2:
      return `${r}nd`
    case 3:
      return `${r}rd`
    default:
      return `${r}th`
  }
}

export const strengthLabel: Record<Strength, string> = {
  none: 'No meaningful link',
  weak: 'Weak link',
  moderate: 'Moderate link',
  strong: 'Strong link',
}

/** Lowercase a label for use mid-sentence, keeping acronyms (BMI, HbA1c). */
export function lowerFirst(s: string): string {
  if (!s) return s
  const word = s.split(' ')[0]
  if (/[A-Z0-9]/.test(word.slice(1))) return s
  return s[0].toLowerCase() + s.slice(1)
}

/** Nice round tick values covering [lo, hi]. */
export function niceTicks(lo: number, hi: number, count = 5): number[] {
  if (!(hi > lo)) return [lo]
  const raw = (hi - lo) / Math.max(1, count)
  const mag = Math.pow(10, Math.floor(Math.log10(raw)))
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => raw <= s) ?? 10 * mag
  const start = Math.ceil(lo / step - 1e-9) * step
  const ticks: number[] = []
  for (let v = start; v <= hi + step * 1e-9; v += step) ticks.push(Number(v.toFixed(10)))
  return ticks
}

/** A nice axis covering [lo, hi]: ticks plus the rounded-out domain. */
export function niceScale(lo: number, hi: number, count = 5): { ticks: number[]; lo: number; hi: number } {
  if (!(hi > lo)) return { ticks: [lo], lo, hi: lo + 1 }
  const raw = (hi - lo) / Math.max(1, count)
  const mag = Math.pow(10, Math.floor(Math.log10(raw)))
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => raw <= s) ?? 10 * mag
  const start = Math.floor(lo / step + 1e-9) * step
  const end = Math.ceil(hi / step - 1e-9) * step
  const ticks: number[] = []
  for (let v = start; v <= end + step * 1e-9; v += step) ticks.push(Number(v.toFixed(10)))
  return { ticks, lo: start, hi: end }
}
