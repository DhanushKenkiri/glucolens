import { useEffect, useSyncExternalStore } from 'react'
import { useSettings } from './store'
import type { Mode } from './palette'

const query = '(prefers-color-scheme: dark)'

function subscribe(cb: () => void) {
  const mq = window.matchMedia(query)
  mq.addEventListener('change', cb)
  return () => mq.removeEventListener('change', cb)
}

function systemDark() {
  return window.matchMedia(query).matches
}

/** The theme in effect ("light" or "dark"), following the system when asked to. */
export function useResolvedTheme(): Mode {
  const pref = useSettings((s) => s.theme)
  const dark = useSyncExternalStore(subscribe, systemDark, () => false)
  return pref === 'system' ? (dark ? 'dark' : 'light') : pref
}

/** Keeps <html data-theme> in sync with the resolved theme. */
export function useApplyTheme() {
  const mode = useResolvedTheme()
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', mode)
  }, [mode])
  return mode
}

/** Literal colors for the React Bits components that cannot read CSS variables. */
export const literal = {
  light: {
    surface: '#fcfcfb',
    surface2: '#f2f1ed',
    page: '#f9f9f7',
    ink: '#0b0b0b',
    ink2: '#52514e',
    accent: '#2a78d6',
    shine: '#2a78d6',
    shineBase: '#52514e',
    glowHsl: '213 67 50',
    glowColors: ['#86b6ef', '#2a78d6', '#1baf7a'],
    auroraA: '#86b6ef',
    auroraB: '#1baf7a',
  },
  dark: {
    surface: '#1a1a19',
    surface2: '#222220',
    page: '#0d0d0d',
    ink: '#f0efec',
    ink2: '#c3c2b7',
    accent: '#3987e5',
    shine: '#ffffff',
    shineBase: '#c3c2b7',
    glowHsl: '213 77 56',
    glowColors: ['#3987e5', '#5598e7', '#199e70'],
    auroraA: '#3987e5',
    auroraB: '#199e70',
  },
} as const
