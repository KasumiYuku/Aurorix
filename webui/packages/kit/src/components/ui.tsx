import { useEffect, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from 'react'
import { Check, Info, Power, TriangleAlert, Zap, type LucideIcon } from 'lucide-react'
import { cn } from '../lib/cn'

export { cn }


type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
  size?: 'md' | 'sm' | 'icon'
  icon?: LucideIcon
  loading?: boolean
}

const VARIANT_CLASS = {
  primary: 'btn-primary',
  secondary: 'btn-secondary',
  danger: 'btn-danger',
  ghost: 'btn-ghost',
} as const

const SIZE_CLASS = {
  md: '',
  sm: 'btn-sm',
  icon: 'px-2',
} as const

export function Button({
  variant = 'primary',
  size = 'md',
  icon: Icon,
  loading = false,
  className,
  children,
  disabled,
  type = 'button',
  ...rest
}: ButtonProps) {
  return (
    <button
      type={type}
      className={cn('btn', VARIANT_CLASS[variant], SIZE_CLASS[size], className)}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...rest}
    >
      {loading ? <Spinner /> : Icon ? <Icon aria-hidden className="size-4 shrink-0" /> : null}
      {children}
    </button>
  )
}


type CardProps = {
  title?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  children?: ReactNode
  className?: string
  bodyClassName?: string
}

export function Card({ title, description, actions, children, className, bodyClassName }: CardProps) {
  const hasHeader = Boolean(title || description || actions)
  return (
    <section className={cn('card md:p-6', className)}>
      {hasHeader && (
        <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0">
            {title && <h2 className="text-xl text-ink">{title}</h2>}
            {description && <p className="mt-1 max-w-prose text-sm text-ink-soft">{description}</p>}
          </div>
          {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
        </header>
      )}
      <div className={bodyClassName}>{children}</div>
    </section>
  )
}


export type BadgeTone = 'ok' | 'warn' | 'danger' | 'accent' | 'neutral'

const BADGE_TONE: Record<BadgeTone, string> = {
  ok: 'bg-ok/12 text-ok border border-ok/25',
  warn: 'bg-warn/14 text-warn-ink border border-warn/35',
  danger: 'bg-danger/10 text-danger-ink border border-danger/25',
  accent: 'bg-accent/10 text-accent border border-accent/25',
  neutral: 'bg-sunk text-ink-soft border border-line',
}

type BadgeProps = {
  tone?: BadgeTone
  icon?: LucideIcon
  children: ReactNode
  className?: string
  title?: string
}

export function Badge({ tone = 'neutral', icon: Icon, children, className, title }: BadgeProps) {
  return (
    <span className={cn('badge', BADGE_TONE[tone], className)} title={title}>
      {Icon && <Icon aria-hidden className="size-3.5 shrink-0" />}
      {children}
    </span>
  )
}

export function HotBadge({ hot }: { hot: boolean }) {
  return hot ? (
    <Badge tone="ok" icon={Zap} title="保存即生效">
      热
    </Badge>
  ) : (
    <Badge tone="warn" icon={Power} title="需重启后生效">
      重启
    </Badge>
  )
}


export function Table({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn('table-wrap', className)}>
      <table className="data">{children}</table>
    </div>
  )
}

export const Th = ({ children, className }: { children?: ReactNode; className?: string }) => (
  <th scope="col" className={className}>
    {children}
  </th>
)

export const Td = ({ children, className, colSpan }: { children?: ReactNode; className?: string; colSpan?: number }) => (
  <td colSpan={colSpan} className={className}>
    {children}
  </td>
)


export const Skeleton = ({ className }: { className?: string }) => (
  <span aria-hidden className={cn('animate-breathe block h-4 rounded bg-sunk', className)} />
)

export function SkeletonRows({ rows = 5, cols = 4 }: { rows?: number; cols?: number }) {
  return (
    <div className="table-wrap" role="status" aria-label="加载中">
      <div className="flex flex-col gap-3 p-4">
        {Array.from({ length: rows }).map((_, r) => (
          <div key={r} className="flex gap-4">
            {Array.from({ length: cols }).map((__, c) => (
              <Skeleton key={c} className={cn('h-5 flex-1', c === 0 && 'max-w-[8rem]')} />
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

export function Spinner({ className }: { className?: string }) {
  return (
    <span
      role="img"
      aria-label="处理中"
      className={cn('inline-block size-4 animate-spin rounded-full border-2 border-current/25 border-t-current', className)}
    />
  )
}


type EmptyStateProps = {
  icon?: LucideIcon
  title: string
  description?: ReactNode
  action?: ReactNode
  className?: string
}

export function EmptyState({ icon: Icon = Info, title, description, action, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center gap-4 rounded-lg border border-dashed border-line px-6 py-12 text-center', className)}>
      <span className="flex size-10 items-center justify-center rounded-md bg-sunk">
        <Icon aria-hidden className="size-5 text-ink-faint" />
      </span>
      <div>
        <p className="text-lg text-ink">{title}</p>
        {description && <p className="mt-1 max-w-prose text-sm text-ink-soft">{description}</p>}
      </div>
      {action}
    </div>
  )
}


export function Tooltip({ label, children, className }: { label: ReactNode; children: ReactNode; className?: string }) {
  return (
    <span className={cn('group/tip relative inline-flex', className)}>
      {children}
      <span
        role="tooltip"
        className="pointer-events-none absolute bottom-full left-1/2 z-30 mb-2 w-max max-w-xs -translate-x-1/2 rounded-md border border-line bg-surface px-3 py-2 text-xs text-ink-soft opacity-0 shadow-md transition-opacity duration-150 ease-out group-hover/tip:opacity-100 group-focus-within/tip:opacity-100"
      >
        {label}
      </span>
    </span>
  )
}


export function CopyButton({ onCopy, label = '复制' }: { onCopy: () => void; label?: string }) {
  const [done, setDone] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])

  return (
    <Button
      variant="ghost"
      size="sm"
      icon={done ? Check : undefined}
      onClick={() => {
        onCopy()
        setDone(true)
        timer.current = window.setTimeout(() => setDone(false), 1_500)
      }}
    >
      {done ? '已复制' : label}
    </Button>
  )
}


export function DangerNote({ children, className }: { children?: ReactNode; className?: string }) {
  return (
    <p className={cn('flex items-center gap-1.5 rounded-md bg-danger/8 px-2.5 py-1.5 text-xs text-danger-ink', className)}>
      <TriangleAlert aria-hidden className="size-3.5 shrink-0" />
      {children ?? '这是危险操作，可能产生不可逆后果或上游副作用。'}
    </p>
  )
}
