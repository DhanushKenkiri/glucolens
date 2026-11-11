import { useState, type ReactNode } from 'react'
import { HugeiconsIcon } from '@hugeicons/react'
import { ChartHistogramIcon, Table01Icon } from '@hugeicons/core-free-icons'
import { ErrorState, Loading } from '@/components/ui/primitives'
import { cn } from '@/lib/utils'

/**
 * Wraps a chart with a chart/table switch. While new data loads, the
 * previous render stays in place at reduced opacity (no skeleton flash).
 */
export function ViewFrame({
  title,
  subtitle,
  actions,
  table,
  footer,
  isFetching,
  children,
  className,
}: {
  title?: ReactNode
  subtitle?: ReactNode
  actions?: ReactNode
  table?: ReactNode
  footer?: ReactNode
  isFetching?: boolean
  children: ReactNode
  className?: string
}) {
  const [mode, setMode] = useState<'chart' | 'table'>('chart')
  return (
    <div className={cn('min-w-0', className)}>
      {(title || subtitle || table || actions) && (
        <div className="mb-3 flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
          <div className="min-w-[min(100%,14rem)] flex-1">
            {title && <h3 className="text-[15px] font-semibold leading-snug text-ink">{title}</h3>}
            {subtitle && <p className="mt-0.5 text-xs leading-relaxed text-ink-2">{subtitle}</p>}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {actions}
            {table && (
              <div className="flex rounded-full bg-surface-2 p-0.5 ring-1 ring-line" role="group" aria-label="Show as">
                {(['chart', 'table'] as const).map((m) => (
                  <button
                    key={m}
                    type="button"
                    onClick={() => setMode(m)}
                    aria-pressed={mode === m}
                    title={m === 'chart' ? 'Chart' : 'Table'}
                    className={cn(
                      'grid size-7 place-items-center rounded-full transition-colors',
                      mode === m ? 'bg-surface text-ink shadow-sm dark:bg-surface-3' : 'text-muted hover:text-ink',
                    )}
                  >
                    <HugeiconsIcon icon={m === 'chart' ? ChartHistogramIcon : Table01Icon} size={15} strokeWidth={1.8} />
                    <span className="sr-only">{m === 'chart' ? 'Chart' : 'Table'}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
      <div className={cn('transition-opacity duration-200', isFetching && 'opacity-55')}>{mode === 'chart' || !table ? children : table}</div>
      {footer && <div className="mt-3">{footer}</div>}
    </div>
  )
}

/** Standard loading and error handling for a query-backed view. */
export function QueryGate({ isPending, error, refetch, children, minHeight = 160 }: {
  isPending: boolean
  error: unknown
  refetch?: () => void
  children: () => ReactNode
  minHeight?: number
}) {
  if (isPending) return <Loading label="Crunching numbers" />
  if (error) return <ErrorState error={error} onRetry={refetch} />
  return <div style={{ minHeight }}>{children()}</div>
}
