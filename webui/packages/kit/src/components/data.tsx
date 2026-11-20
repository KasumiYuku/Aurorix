import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Inbox, RotateCw, TriangleAlert, type LucideIcon } from 'lucide-react'

import { Button, CopyButton, EmptyState, SkeletonRows, Tooltip, cn } from './ui'
import { formatDateTime, formatRelative } from '../lib/format'
import { useCopy } from '../lib/hooks'

function describeError(err: unknown): { detail: string; requestId?: string } {
  if (err && typeof err === 'object') {
    const e = err as { detail?: unknown; message?: unknown; requestId?: unknown; request_id?: unknown }
    const detail = typeof e.detail === 'string' && e.detail ? e.detail : typeof e.message === 'string' ? e.message : ''
    const requestId =
      typeof e.requestId === 'string' ? e.requestId : typeof e.request_id === 'string' ? e.request_id : undefined
    if (detail || requestId) return { detail: detail || '请求失败', requestId }
  }
  if (typeof err === 'string' && err) return { detail: err }
  return { detail: '未知错误' }
}

function LoadError({ error, onRetry, retryLabel = '重试' }: { error: unknown; onRetry?: () => void; retryLabel?: string }) {
  const e = describeError(error)
  return (
    <div className="flex items-start gap-3 rounded-lg border border-danger/35 bg-danger/6 p-5">
      <TriangleAlert aria-hidden className="mt-0.5 size-5 shrink-0 text-danger-ink" />
      <div className="min-w-0">
        <p className="text-base text-ink">数据加载失败</p>
        <p className="mt-1 text-sm text-ink-soft">{e.detail}</p>
        {e.requestId && <p className="mt-1 font-mono text-xs text-ink-faint">请求 ID {e.requestId}</p>}
        {onRetry && (
          <Button variant="secondary" size="sm" icon={RotateCw} onClick={onRetry} className="mt-3">
            {retryLabel}
          </Button>
        )}
      </div>
    </div>
  )
}

export type Column<T> = {
  key: string
  label: ReactNode
  render: (row: T) => ReactNode
  align?: 'left' | 'right' | 'center'
  className?: string
  headClassName?: string
  hideOnMobile?: boolean
  primary?: boolean
  width?: string
}

type ResourceTableProps<T> = {
  columns: Column<T>[]
  rows: T[] | undefined
  rowKey: (row: T) => string
  loading?: boolean
  error?: unknown
  empty?: { icon?: LucideIcon; title: string; description?: ReactNode; action?: ReactNode }
  onRowClick?: (row: T) => void
  activeKey?: string
  skeletonRows?: number
  className?: string
  tableClassName?: string
}

const ALIGN = { left: 'text-left', right: 'text-right', center: 'text-center' } as const

function cssPx(width?: string): number {
  if (!width) return 0
  const m = /^([\d.]+)(px|rem)$/.exec(width.trim())
  if (!m) return 0
  return m[2] === 'rem' ? Number(m[1]) * 16 : Number(m[1])
}

export function ResourceTable<T>({
  columns,
  rows,
  rowKey,
  loading = false,
  error,
  empty,
  onRowClick,
  activeKey,
  skeletonRows = 6,
  className,
  tableClassName,
}: ResourceTableProps<T>) {
  if (loading && !rows) return <SkeletonRows rows={skeletonRows} cols={Math.min(columns.length, 5)} />

  if (error) return <LoadError error={error} />

  const data = rows ?? []
  if (data.length === 0) {
    return (
      <EmptyState
        icon={empty?.icon ?? Inbox}
        title={empty?.title ?? '暂无数据'}
        description={empty?.description}
        action={empty?.action}
      />
    )
  }

  const fixed = columns.some((c) => c.width)
  const declared = columns.reduce((sum, c) => sum + cssPx(c.width), 0)
  const minWidth = declared > 0 ? declared + 200 : undefined

  return (
    <div className={className}>
      <div className="table-wrap hidden md:block">
        <table className={cn('data', fixed && 'table-fixed', tableClassName)} style={minWidth ? { minWidth } : undefined}>
          <thead>
            <tr>
              {columns.map((col) => (
                <th
                  key={col.key}
                  scope="col"
                  className={cn(ALIGN[col.align ?? 'left'], col.headClassName)}
                  style={col.width ? { width: col.width } : undefined}
                >
                  {col.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.map((row) => (
              <tr
                key={rowKey(row)}
                onClick={onRowClick ? () => onRowClick(row) : undefined}
                className={cn(onRowClick && 'cursor-pointer', activeKey === rowKey(row) && 'bg-ok/10')}
              >
                {columns.map((col) => (
                  <td key={col.key} className={cn(ALIGN[col.align ?? 'left'], col.className)}>
                    {col.render(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <ul className="flex flex-col gap-4 md:hidden">
        {data.map((row) => {
          const primary = columns.find((c) => c.primary) ?? columns[0]
          const rest = columns.filter((c) => c !== primary && !c.hideOnMobile)
          return (
            <li key={rowKey(row)}>
              <div
                className={cn('rounded-lg border border-line bg-surface p-5', onRowClick && 'cursor-pointer')}
                onClick={onRowClick ? () => onRowClick(row) : undefined}
              >
                <div className="text-base text-ink">{primary.render(row)}</div>
                <dl className="mt-3 flex flex-col gap-2">
                  {rest.map((col) => (
                    <div key={col.key} className="flex items-start justify-between gap-4">
                      <dt className="shrink-0 text-xs text-ink-faint">{col.label}</dt>
                      <dd className="min-w-0 text-right text-sm text-ink-soft">{col.render(row)}</dd>
                    </div>
                  ))}
                </dl>
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}


type AsyncStateProps = {
  pending?: boolean
  error?: unknown
  isEmpty?: boolean
  skeletonRows?: number
  skeletonCols?: number
  empty?: { icon?: LucideIcon; title: string; description?: ReactNode; action?: ReactNode }
  onRetry?: () => void
  retryLabel?: string
  children: ReactNode
  className?: string
}

export function AsyncState({
  pending = false,
  error,
  isEmpty = false,
  skeletonRows = 5,
  skeletonCols = 4,
  empty,
  onRetry,
  retryLabel,
  children,
  className,
}: AsyncStateProps) {
  return (
    <div className={className}>
      {pending ? (
        <SkeletonRows rows={skeletonRows} cols={skeletonCols} />
      ) : error ? (
        <LoadError error={error} onRetry={onRetry} retryLabel={retryLabel} />
      ) : isEmpty ? (
        <EmptyState
          icon={empty?.icon ?? Inbox}
          title={empty?.title ?? '暂无数据'}
          description={empty?.description}
          action={empty?.action}
        />
      ) : (
        children
      )}
    </div>
  )
}


export function TimeAgo({ value, className }: { value: string | number | null | undefined; className?: string }) {
  const [, setTick] = useState(0)

  useEffect(() => {
    const timer = window.setInterval(() => setTick((n) => n + 1), 30_000)
    return () => window.clearInterval(timer)
  }, [])

  if (!value) return <span className={cn('text-ink-faint', className)}>—</span>

  return (
    <Tooltip label={formatDateTime(value)}>
      <time dateTime={String(value)} className={cn('text-ink-soft', className)}>
        {formatRelative(value)}
      </time>
    </Tooltip>
  )
}


type JsonViewerProps = {
  value: unknown
  className?: string
  maxHeight?: string
  label?: string
}

export function JsonViewer({ value, className, maxHeight = '24rem', label = '内容' }: JsonViewerProps) {
  const { copy } = useCopy()
  const text = useMemo(() => {
    if (value === undefined || value === null) return '—'
    if (typeof value === 'string') return value
    try {
      return JSON.stringify(value, null, 2)
    } catch {
      return String(value)
    }
  }, [value])

  return (
    <div className={cn('relative', className)}>
      <div className="absolute right-3 top-3 z-10">
        <CopyButton onCopy={() => void copy(text)} label={`复制${label}`} />
      </div>
      <pre className="code-block overflow-auto" style={{ maxHeight }}>
        {text}
      </pre>
    </div>
  )
}


export type KeyValue = { label: ReactNode; value: ReactNode; mono?: boolean; copy?: string }
export type KeyValueItem = KeyValue

export function KeyValueList({ items, className }: { items: KeyValue[]; className?: string }) {
  const { copy } = useCopy()

  return (
    <dl className={cn('flex flex-col gap-3', className)}>
      {items.map((item, i) => (
        <div key={i} className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
          <dt className="text-xs text-ink-faint">{item.label}</dt>
          <dd className={cn('min-w-0 text-right text-sm text-ink', item.mono && 'font-mono text-xs')}>
            <span className="break-all">{item.value}</span>
            {item.copy && (
              <button
                type="button"
                className="ml-2 text-xs text-ink-faint transition-colors duration-150 hover:text-ok"
                onClick={() => void copy(item.copy!)}
              >
                复制
              </button>
            )}
          </dd>
        </div>
      ))}
    </dl>
  )
}


type StatTone = 'default' | 'ok' | 'warn' | 'danger'

const STAT_TONE: Record<StatTone, string> = {
  default: 'text-ink',
  ok: 'text-ok',
  warn: 'text-warn-ink',
  danger: 'text-danger-ink',
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
    <div className={cn('min-w-32 rounded-md border border-line px-4 py-2.5', className)}>
      <p className="text-xs text-ink-faint">{label}</p>
      <p className={cn('mt-0.5 font-mono text-base leading-6 tabular-nums', STAT_TONE[tone])}>{value}</p>
      {hint && <p className="mt-0.5 truncate text-xs text-ink-faint">{hint}</p>}
    </div>
  )
}
