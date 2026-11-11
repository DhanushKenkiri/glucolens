import { useEffect, useRef, type ButtonHTMLAttributes, type ReactNode } from 'react'
import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/react'
import { Alert02Icon, Cancel01Icon, InformationCircleIcon } from '@hugeicons/core-free-icons'
import LatticeLoader from '@/components/reactbits/LatticeLoader'
import { cn } from '@/lib/utils'
import { STATUS } from '@/lib/palette'
import type { Strength, Tone } from '@/lib/types'

export function Icon({ icon, size = 16, className, strokeWidth = 1.8 }: { icon: IconSvgElement; size?: number; className?: string; strokeWidth?: number }) {
  return <HugeiconsIcon icon={icon} size={size} strokeWidth={strokeWidth} className={className} aria-hidden="true" />
}

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger'
const variants: Record<Variant, string> = {
  primary: 'bg-accent text-white hover:bg-accent-hover shadow-sm',
  secondary: 'bg-surface-2 text-ink ring-1 ring-line hover:bg-surface-3',
  ghost: 'text-ink-2 hover:text-ink hover:bg-surface-2',
  danger: 'text-alert-text hover:bg-surface-2',
}

export function Button({
  variant = 'secondary',
  size = 'md',
  icon,
  className,
  children,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: 'sm' | 'md'; icon?: IconSvgElement }) {
  return (
    <button
      type="button"
      className={cn(
        'inline-flex items-center justify-center gap-1.5 rounded-full font-medium whitespace-nowrap transition-colors disabled:opacity-50',
        size === 'sm' ? 'h-8 px-3 text-[13px]' : 'h-10 px-4 text-sm',
        variants[variant],
        className,
      )}
      {...rest}
    >
      {icon && <Icon icon={icon} size={size === 'sm' ? 15 : 17} />}
      {children}
    </button>
  )
}

export function IconButton({ icon, label, className, ...rest }: ButtonHTMLAttributes<HTMLButtonElement> & { icon: IconSvgElement; label: string }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={cn('inline-grid size-9 place-items-center rounded-full text-ink-2 transition-colors hover:bg-surface-2 hover:text-ink', className)}
      {...rest}
    >
      <Icon icon={icon} size={18} />
    </button>
  )
}

/** A status chip: a colored dot plus a text label (never color alone). */
export function ToneBadge({ tone, children, className }: { tone: Tone; children: ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full bg-surface-2 px-2 py-0.5 text-xs font-medium text-ink ring-1 ring-line',
        className,
      )}
    >
      <span className="size-2 rounded-full" style={{ background: STATUS[tone] }} aria-hidden="true" />
      {children}
    </span>
  )
}

const strengthTone: Record<Strength, string> = {
  none: 'var(--axis)',
  weak: '#86b6ef',
  moderate: 'var(--accent)',
  strong: '#1c5cab',
}

/** Strength of a link: a 4-step meter (magnitude, so one blue hue) plus a label. */
export function StrengthBadge({ strength }: { strength: Strength }) {
  const steps = { none: 0, weak: 1, moderate: 2, strong: 3 }[strength]
  const label = { none: 'No meaningful link', weak: 'Weak', moderate: 'Moderate', strong: 'Strong' }[strength]
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-ink-2">
      <span className="flex items-end gap-[2px]" aria-hidden="true">
        {[1, 2, 3].map((i) => (
          <span
            key={i}
            className="w-[4px] rounded-[1px]"
            style={{ height: 4 + i * 3, background: i <= steps ? strengthTone[strength] : 'var(--grid)' }}
          />
        ))}
      </span>
      {label}
    </span>
  )
}

export function Card({ className, children, as: Tag = 'section' }: { className?: string; children: ReactNode; as?: 'section' | 'div' | 'article' }) {
  return <Tag className={cn('card', className)}>{children}</Tag>
}

export function Loading({ label = 'Loading', className }: { label?: string; className?: string }) {
  return (
    <div className={cn('flex items-center justify-center py-10 text-ink-2', className)}>
      <LatticeLoader label={label} status="working" pattern="ripple" color="var(--accent)" fontSize={13} showTimer={false} />
    </div>
  )
}

export function ErrorState({ error, onRetry, className }: { error: unknown; onRetry?: () => void; className?: string }) {
  const message = error instanceof Error ? error.message : 'Something went wrong.'
  return (
    <div role="alert" className={cn('flex items-start gap-3 rounded-2xl bg-surface-2 p-4 text-sm ring-1 ring-line', className)}>
      <Icon icon={Alert02Icon} size={18} className="mt-0.5 shrink-0 text-alert" />
      <div className="min-w-0 flex-1">
        <p className="font-medium text-ink">Couldn't load this</p>
        <p className="mt-0.5 text-ink-2">{message}</p>
      </div>
      {onRetry && (
        <Button size="sm" onClick={onRetry}>
          Retry
        </Button>
      )}
    </div>
  )
}

export function Note({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <p className={cn('flex items-start gap-2 text-xs leading-relaxed text-muted', className)}>
      <Icon icon={InformationCircleIcon} size={14} className="mt-px shrink-0" />
      <span>{children}</span>
    </p>
  )
}

export function EmptyState({ title, children, action, icon }: { title: string; children?: ReactNode; action?: ReactNode; icon?: IconSvgElement }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-6 py-12 text-center">
      {icon && (
        <span className="mb-1 grid size-11 place-items-center rounded-2xl bg-accent-soft text-accent">
          <Icon icon={icon} size={22} />
        </span>
      )}
      <p className="text-[15px] font-semibold text-ink">{title}</p>
      {children && <div className="max-w-md text-sm text-ink-2">{children}</div>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  )
}

/** A right-side sheet with a backdrop; Escape and backdrop clicks close it. */
export function Drawer({ open, onClose, title, children, footer, width = 440 }: { open: boolean; onClose: () => void; title: ReactNode; children: ReactNode; footer?: ReactNode; width?: number }) {
  const panelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', onKey)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    panelRef.current?.focus()
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = prev
    }
  }, [open, onClose])

  return (
    <div className={cn('fixed inset-0 z-[60]', open ? '' : 'pointer-events-none')} aria-hidden={!open}>
      <div
        className={cn('absolute inset-0 bg-black/30 backdrop-blur-[2px] transition-opacity duration-300', open ? 'opacity-100' : 'opacity-0')}
        onClick={onClose}
      />
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === 'string' ? title : undefined}
        tabIndex={-1}
        className={cn(
          'absolute top-0 right-0 flex h-full max-w-full flex-col bg-surface outline-none',
          open ? 'visible translate-x-0 shadow-[var(--shadow-pop)]' : 'invisible translate-x-full',
        )}
        style={{ width, transition: 'translate 300ms cubic-bezier(0.23,1,0.32,1), visibility 300ms' }}
      >
        <div className="flex items-center justify-between gap-3 border-b border-line px-5 py-4">
          <div className="min-w-0 text-base font-semibold text-ink">{title}</div>
          <IconButton icon={Cancel01Icon} label="Close" onClick={onClose} />
        </div>
        <div className="scroll-thin min-h-0 flex-1 overflow-y-auto">{children}</div>
        {footer && <div className="border-t border-line px-5 py-4">{footer}</div>}
      </div>
    </div>
  )
}
