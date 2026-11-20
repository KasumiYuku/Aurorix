import type { ReactNode } from 'react'

import { cn } from '../lib/cn'

export type KpiTone = 'ok' | 'warn' | 'danger' | 'accent' | 'neutral'

const KPI_TONE: Record<KpiTone, string> = {
  ok: 'text-ok',
  warn: 'text-warn-ink',
  danger: 'text-danger-ink',
  accent: 'text-ink',
  neutral: 'text-ink',
}

export type KpiItem = {
  label: string
  value: ReactNode
  hint?: ReactNode
  tone?: KpiTone
  onClick?: () => void
}

type OpsPageProps = {
  title: string
  description?: ReactNode
  meta?: ReactNode
  actions?: ReactNode
  kpis?: KpiItem[]
  children: ReactNode
}

export function OpsPage({ title, description, meta, actions, kpis, children }: OpsPageProps) {
  return (
    <>
      <header className="mb-4 flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-xl text-ink">{title}</h1>
          {description && <p className="mt-1 max-w-[68ch] text-sm text-ink-soft">{description}</p>}
          {meta && <p className="mt-1 font-mono text-xs text-ink-faint">{meta}</p>}
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </header>
      {kpis && kpis.length > 0 && <KpiStrip items={kpis} />}
      {children}
    </>
  )
}

export function KpiStrip({ items }: { items: KpiItem[] }) {
  return (
    <dl className="mb-6 flex flex-wrap items-stretch divide-x divide-line border-y border-line">
      {items.map((k) => (
        <div
          key={k.label}
          className={cn('min-w-32 flex-1 px-4 py-2.5 first:pl-0', k.onClick && 'cursor-pointer hover:bg-sunk')}
          onClick={k.onClick}
        >
          <dt className="text-xs text-ink-faint">{k.label}</dt>
          <dd className={cn('mt-0.5 font-mono text-base leading-6 tabular-nums', KPI_TONE[k.tone ?? 'neutral'])}>{k.value}</dd>
          {k.hint && <p className="mt-0.5 truncate text-xs text-ink-faint">{k.hint}</p>}
        </div>
      ))}
    </dl>
  )
}

export function OpsToolbar({ children, right }: { children?: ReactNode; right?: ReactNode }) {
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2 border-b border-line pb-3">
      {children}
      {right && <div className="ml-auto flex items-center gap-2 font-mono text-xs text-ink-faint">{right}</div>}
    </div>
  )
}

type OpsSectionProps = {
  title: string
  hint?: ReactNode
  actions?: ReactNode
  children: ReactNode
}

export function OpsSection({ title, hint, actions, children }: OpsSectionProps) {
  return (
    <section className="border-b border-line py-4 last:border-b-0">
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="text-sm font-medium text-ink">{title}</h2>
        <div className="flex items-center gap-3">
          {hint && <span className="text-xs text-ink-faint">{hint}</span>}
          {actions}
        </div>
      </div>
      {children}
    </section>
  )
}

export const Panel = OpsSection
