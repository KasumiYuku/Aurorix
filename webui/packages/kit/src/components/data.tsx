// 展示型组件：只负责排版，不含任何取数逻辑。
import type { ReactNode } from 'react'
import { cn } from '../lib/cn'

type StatTone = 'default' | 'ok' | 'warn' | 'danger'

const STAT_TONE: Record<StatTone, string> = {
  default: 'text-ink',
  ok: 'text-ok-text',
  warn: 'text-warn-text',
  danger: 'text-danger-text',
}

type StatCardProps = {
  label: ReactNode
  value: ReactNode
  hint?: ReactNode
  tone?: StatTone
  className?: string
}

export function StatCard({ label, value, hint, tone = 'default', className }: StatCardProps) {
  return (
    <div className={cn('rounded-[1.5rem] border border-line bg-surface px-4 py-3', className)}>
      <p className="text-xs text-ink-faint">{label}</p>
      <p className={cn('mt-1 font-mono text-lg tabular-nums', STAT_TONE[tone])}>{value}</p>
      {hint && <p className="mt-0.5 text-xs text-ink-soft">{hint}</p>}
    </div>
  )
}

export type KeyValueItem = { label: ReactNode; value: ReactNode }

export function KeyValueList({ items, className }: { items: KeyValueItem[]; className?: string }) {
  return (
    <dl className={cn('grid gap-x-8 gap-y-1 sm:grid-cols-2', className)}>
      {items.map((item, index) => (
        <div key={index} className="flex items-baseline justify-between gap-3 border-b border-line/60 py-1.5">
          <dt className="shrink-0 text-xs text-ink-faint">{item.label}</dt>
          <dd className="min-w-0 truncate text-right text-sm text-ink">{item.value}</dd>
        </div>
      ))}
    </dl>
  )
}
