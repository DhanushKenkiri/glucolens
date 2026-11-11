import { forwardRef, type AnchorHTMLAttributes } from 'react'
import { Link, Outlet, useLocation } from 'react-router'
import { Moon02Icon, Sun03Icon, UserCircleIcon } from '@hugeicons/core-free-icons'
import PillNav from '@/components/reactbits/PillNav'
import { Icon, IconButton } from '@/components/ui/primitives'
import { MyValuesDrawer } from '@/components/values/MyValuesDrawer'
import { useHealth, useMyValues, useSchema } from '@/lib/schema'
import { useSettings, useUi } from '@/lib/store'
import { useApplyTheme } from '@/lib/theme'
import { fmtNumber } from '@/lib/format'
import { STATIC, useRuntimeStatus } from '@/lib/runtime'
import LatticeLoader from '@/components/reactbits/LatticeLoader'

// PillNav renders links with an href prop; adapt it to the router's Link.
const RouterLink = forwardRef<HTMLAnchorElement, AnchorHTMLAttributes<HTMLAnchorElement> & { href?: string }>(
  function RouterLink({ href = '/', ...rest }, ref) {
    return <Link ref={ref} to={href} {...rest} />
  },
)

const NAV = [
  { label: 'Ask', href: '/' },
  { label: 'Insights', href: '/insights' },
  { label: 'Risk Explorer', href: '/risk' },
]

export function Logo() {
  return (
    <Link to="/" className="flex shrink-0 items-center gap-2 rounded-xl" aria-label="GlucoLens home">
      <svg width="30" height="30" viewBox="0 0 32 32" aria-hidden="true">
        <rect width="32" height="32" rx="9" fill="var(--accent)" />
        <circle cx="14" cy="14" r="7" fill="none" stroke="#fff" strokeWidth="2.6" />
        <path d="M19.2 19.2 24 24" stroke="#fff" strokeWidth="2.8" strokeLinecap="round" />
        <path d="M10.5 15.2c1.2-2.6 2.4-2.6 3.5 0s2.3 2.6 3.5 0" fill="none" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" />
      </svg>
      <span className="hidden text-[17px] font-semibold tracking-tight text-ink sm:inline">GlucoLens</span>
    </Link>
  )
}

export function AppShell() {
  const mode = useApplyTheme()
  const { pathname } = useLocation()
  const setTheme = useSettings((s) => s.setTheme)
  const openValues = useUi((s) => s.openValues)
  const { count } = useMyValues()
  const { data: schema } = useSchema()
  const health = useHealth()
  const runtime = useRuntimeStatus()
  const active = NAV.find((n) => (n.href === '/' ? pathname === '/' : pathname.startsWith(n.href)))?.href ?? '/'

  return (
    <div className="flex min-h-dvh flex-col bg-page text-ink">
      <header className="sticky top-0 z-40 border-b border-line bg-[color-mix(in_srgb,var(--page)_82%,transparent)] backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-[1200px] items-center gap-3 px-4">
          <Logo />
          <div className="flex min-w-0 flex-1 justify-center">
            <PillNav
              items={NAV}
              activeHref={active}
              linkComponent={RouterLink}
              theme={mode}
              size="sm"
              surface="glass"
              hoverEffect="fill"
              indicator="dot"
              intro={false}
              highlightColor="var(--accent)"
              highlightTextColor="#ffffff"
              ariaLabel="Sections"
            />
          </div>
          <button
            type="button"
            onClick={() => openValues(true)}
            className="inline-flex h-9 shrink-0 items-center gap-2 rounded-full bg-surface-2 px-3 text-sm font-medium text-ink ring-1 ring-line transition-colors hover:bg-surface-3"
            aria-label={`My values${count ? `, ${count} entered` : ''}`}
          >
            <Icon icon={UserCircleIcon} size={18} />
            <span className="hidden sm:inline">My values</span>
            {count > 0 && (
              <span className="tabular grid h-5 min-w-5 place-items-center rounded-full bg-accent px-1.5 text-[11px] font-semibold text-white">{count}</span>
            )}
          </button>
          <IconButton
            icon={mode === 'dark' ? Sun03Icon : Moon02Icon}
            label={mode === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
            onClick={() => setTheme(mode === 'dark' ? 'light' : 'dark')}
          />
        </div>
      </header>

      <main className="flex-1">
        {STATIC && runtime.data === 'loading' && (
          <div className="mx-auto mt-4 max-w-[1200px] px-4" role="status">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-2xl bg-surface-2 px-4 py-3 text-sm text-ink ring-1 ring-line">
              <LatticeLoader label="Loading 100,000 checkups into your browser" status="working" pattern="ripple" color="var(--accent)" fontSize={13.5} />
              <span className="text-ink-2">The analysis runs on your device; the first visit downloads a few megabytes.</span>
            </div>
          </div>
        )}
        {(health.isError || (STATIC && runtime.data === 'failed')) && (
          <div role="alert" className="mx-auto mt-4 max-w-[1200px] px-4">
            <div className="rounded-2xl bg-surface-2 px-4 py-3 text-sm text-ink ring-1 ring-line">
              {STATIC ? (
                <>
                  <span className="font-semibold">GlucoLens could not load its data in this browser.</span>{' '}
                  <span className="text-ink-2">{runtime.error ?? 'Reload the page to try again.'}</span>
                </>
              ) : (
                <>
                  <span className="font-semibold">The GlucoLens API is not reachable.</span>{' '}
                  <span className="text-ink-2">Start it with <code className="font-mono">npm run dev</code> from the project folder, then reload.</span>
                </>
              )}
            </div>
          </div>
        )}
        <Outlet />
      </main>

      <footer className="border-t border-line">
        <div className="mx-auto flex max-w-[1200px] flex-col gap-1 px-4 py-5 text-xs text-muted sm:flex-row sm:items-center sm:justify-between">
          <p>For learning only, not medical advice. Talk to a clinician about your results.</p>
          <p>
            {schema ? `${fmtNumber(schema.dataset.rows)} anonymised checkup records` : 'Checkup dataset'} · your values are saved only in this browser
          </p>
        </div>
      </footer>
      <MyValuesDrawer />
    </div>
  )
}
