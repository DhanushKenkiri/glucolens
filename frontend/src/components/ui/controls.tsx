import type { ReactElement, ReactNode } from 'react'
import GlideSelect, { type GlideSelectOption } from '@/components/reactbits/GlideSelect'
import RubberSegment, { type RubberSegmentItem } from '@/components/reactbits/RubberSegment'
import SquishSwitch from '@/components/reactbits/SquishSwitch'
import WarmTooltip from '@/components/reactbits/WarmTooltip'
import { useResolvedTheme } from '@/lib/theme'

export type Option = GlideSelectOption

/** A themed GlideSelect. */
export function Select(props: {
  options: (string | Option)[]
  value?: string
  onChange: (value: string) => void
  placeholder?: string
  ariaLabel: string
  size?: 'sm' | 'md' | 'lg'
  align?: 'left' | 'right'
  placement?: 'top' | 'bottom'
  menuWidth?: number
  disabled?: boolean
  className?: string
}) {
  const mode = useResolvedTheme()
  return (
    <GlideSelect
      options={props.options}
      value={props.value ?? ''}
      onChange={(v) => props.onChange(v)}
      placeholder={props.placeholder}
      ariaLabel={props.ariaLabel}
      size={props.size ?? 'md'}
      align={props.align}
      placement={props.placement}
      menuWidth={props.menuWidth ?? 240}
      disabled={props.disabled}
      showTags
      accentColor="var(--accent)"
      surfaceColor={mode === 'dark' ? 'var(--surface-2)' : 'var(--surface)'}
      highlightColor="var(--surface-3)"
      textColor="var(--ink)"
      radius={12}
      className={['[&>button]:ring-1 [&>button]:ring-[var(--line)] [&>button]:bg-[var(--surface-2)]', props.className]
        .filter(Boolean)
        .join(' ')}
    />
  )
}

/** A themed RubberSegment. */
export function Segmented(props: {
  items: RubberSegmentItem[]
  value: string
  onChange: (value: string) => void
  ariaLabel: string
  size?: 'sm' | 'md' | 'lg'
  disabled?: boolean
  className?: string
}) {
  const mode = useResolvedTheme()
  return (
    <RubberSegment
      items={props.items}
      value={props.value}
      onChange={(v) => props.onChange(v)}
      aria-label={props.ariaLabel}
      size={props.size ?? 'sm'}
      disabled={props.disabled}
      trackColor="var(--surface-2)"
      thumbColor={mode === 'dark' ? '#383835' : '#ffffff'}
      textColor="var(--ink-2)"
      activeTextColor="var(--ink)"
      className={['ring-1 ring-[var(--line)]', props.className].filter(Boolean).join(' ')}
    />
  )
}

/** A themed SquishSwitch with a visible label. */
export function Switch(props: { checked: boolean; onChange: (on: boolean) => void; label: string; id?: string; disabled?: boolean }) {
  return (
    <label className="inline-flex items-center gap-2.5 text-sm text-ink-2 select-none" htmlFor={props.id}>
      <SquishSwitch
        id={props.id}
        checked={props.checked}
        onChange={props.onChange}
        ariaLabel={props.label}
        disabled={props.disabled}
        width={40}
        height={24}
        trackColor="var(--surface-3)"
        trackOnColor="var(--accent)"
        thumbColor="#ffffff"
        thumbOnColor="#ffffff"
      />
      <span>{props.label}</span>
    </label>
  )
}

/** A themed WarmTooltip. The child must accept a ref and pointer events. */
export function Tip(props: { content: ReactNode; children: ReactElement<Record<string, unknown>>; side?: 'top' | 'bottom' | 'left' | 'right' }) {
  return (
    <WarmTooltip content={props.content} side={props.side ?? 'top'} surfaceColor="var(--ink)" inkColor="var(--surface)" size="sm">
      {props.children}
    </WarmTooltip>
  )
}
