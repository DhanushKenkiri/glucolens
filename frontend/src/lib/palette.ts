import type { Tone } from './types'

export type Mode = 'light' | 'dark'

/**
 * Chart colors from the validated dataviz palette (validated with
 * validate_palette.js against these surfaces: light #fcfcfb, dark #1a1a19).
 * Categorical slots are assigned in fixed order by an entity's level index,
 * never by rank, so filtering never repaints a series.
 */
export const CATEGORICAL: Record<Mode, string[]> = {
  light: ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4', '#008300', '#6250d6', '#e34948'],
  dark: ['#3987e5', '#d95926', '#199e70', '#c98500', '#d55181', '#008300', '#9085e9', '#e66767'],
}

/** Status colors: reserved meaning, always shown with a label. */
export const STATUS: Record<Tone, string> = {
  ok: '#0ca30c',
  watch: '#fab219',
  alert: '#d03b3b',
  info: '#898781',
}

/** Sequential blue, steps 100 to 700. */
export const SEQ_BLUE = [
  '#cde2fb', '#b7d3f6', '#9ec5f4', '#86b6ef', '#6da7ec', '#5598e7', '#3987e5',
  '#2a78d6', '#256abf', '#1c5cab', '#184f95', '#104281', '#0d366b',
]

export const CHART: Record<Mode, { surface: string; ink: string; ink2: string; muted: string; grid: string; axis: string; deEmphasis: string; neutral: string }> = {
  light: { surface: '#fcfcfb', ink: '#0b0b0b', ink2: '#52514e', muted: '#898781', grid: '#e1e0d9', axis: '#c3c2b7', deEmphasis: '#c3c2b7', neutral: '#f0efec' },
  dark: { surface: '#1a1a19', ink: '#f0efec', ink2: '#c3c2b7', muted: '#898781', grid: '#2c2c2a', axis: '#383835', deEmphasis: '#4a4a46', neutral: '#383835' },
}

export function seriesColor(mode: Mode, index: number): string {
  const slots = CATEGORICAL[mode]
  return slots[Math.min(Math.max(index, 0), slots.length - 1)]
}

// --- OKLab interpolation for continuous scales -------------------------------

type Lab = [number, number, number]

function srgbToLinear(c: number) {
  return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
}
function linearToSrgb(c: number) {
  const v = c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055
  return Math.min(1, Math.max(0, v))
}
function hexToLab(hex: string): Lab {
  const n = parseInt(hex.slice(1), 16)
  const r = srgbToLinear(((n >> 16) & 255) / 255)
  const g = srgbToLinear(((n >> 8) & 255) / 255)
  const b = srgbToLinear((n & 255) / 255)
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ]
}
function labToHex([L, A, B]: Lab): string {
  const l = Math.pow(L + 0.3963377774 * A + 0.2158037573 * B, 3)
  const m = Math.pow(L - 0.1055613458 * A - 0.0638541728 * B, 3)
  const s = Math.pow(L - 0.0894841775 * A - 1.291485548 * B, 3)
  const r = linearToSrgb(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s)
  const g = linearToSrgb(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s)
  const b = linearToSrgb(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s)
  const to = (v: number) => Math.round(v * 255).toString(16).padStart(2, '0')
  return `#${to(r)}${to(g)}${to(b)}`
}

export function mix(a: string, b: string, t: number): string {
  const x = hexToLab(a)
  const y = hexToLab(b)
  const k = Math.min(1, Math.max(0, t))
  return labToHex([x[0] + (y[0] - x[0]) * k, x[1] + (y[1] - x[1]) * k, x[2] + (y[2] - x[2]) * k])
}

/** Sequential color for t in [0, 1]; in dark mode low values recede to the surface. */
export function sequential(mode: Mode, t: number): string {
  const k = Math.min(1, Math.max(0, t))
  const ramp = mode === 'light' ? SEQ_BLUE : [...SEQ_BLUE].reverse()
  const pos = k * (ramp.length - 1)
  const i = Math.floor(pos)
  if (i >= ramp.length - 1) return ramp[ramp.length - 1]
  return mix(ramp[i], ramp[i + 1], pos - i)
}

/** Diverging blue (negative) to red (positive) through a neutral gray at 0. */
export function diverging(mode: Mode, v: number): string {
  const neutral = CHART[mode].neutral
  const k = Math.min(1, Math.abs(v))
  const pole = v < 0 ? CATEGORICAL[mode][0] : CATEGORICAL[mode][7]
  return mix(neutral, pole, Math.pow(k, 0.85))
}
