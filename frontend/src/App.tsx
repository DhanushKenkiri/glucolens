import { lazy, Suspense, type ReactNode } from 'react'
import { createBrowserRouter, Link, RouterProvider } from 'react-router'
import { AppShell } from '@/components/layout/AppShell'
import { EmptyState, Loading } from '@/components/ui/primitives'

// Each tab loads on demand so the first page paints sooner.
const AskPage = lazy(() => import('@/pages/AskPage').then((m) => ({ default: m.AskPage })))
const InsightsPage = lazy(() => import('@/pages/InsightsPage').then((m) => ({ default: m.InsightsPage })))
const RiskPage = lazy(() => import('@/pages/RiskPage').then((m) => ({ default: m.RiskPage })))

function Page({ children }: { children: ReactNode }) {
  return <Suspense fallback={<Loading label="Loading" className="py-24" />}>{children}</Suspense>
}

function NotFound() {
  return (
    <div className="mx-auto max-w-[1200px] px-4 py-16">
      <EmptyState title="That page doesn't exist" action={<Link to="/" className="text-sm font-medium text-accent">Go to Ask</Link>} />
    </div>
  )
}

// On GitHub Pages the app lives under /<repository>/.
const basename = import.meta.env.BASE_URL.replace(/[/]$/, '') || undefined

const router = createBrowserRouter([
  {
    element: <AppShell />,
    children: [
      { path: '/', element: <Page><AskPage /></Page> },
      { path: '/insights', element: <Page><InsightsPage /></Page> },
      { path: '/risk', element: <Page><RiskPage /></Page> },
      { path: '*', element: <NotFound /> },
    ],
  },
], { basename })

export function App() {
  return <RouterProvider router={router} />
}
