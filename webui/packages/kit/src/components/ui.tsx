//基础组件：无业务逻辑，只有形状与行为；视觉全部来自 styles/index.css 的语义令牌。
import { useEffect, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from 'react'
import { Check, Info, Power, TriangleAlert, Zap, type LucideIcon } from 'lucide-react'
import { cn } from '../lib/cn'

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
  sm: 'btn-sm min-h-11',
  icon: 'px-3 min-h-11',
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
    <section className={cn('card texture-paper md:p-8', className)}>
      {hasHeader && (
        <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0">
            {title && <h2 className="font-serif text-xl text-ink md:text-2xl">{title}</h2>}
            {description && <p className="mt-1 max-w-prose text-sm text-ink-soft">{description}</p>}
          </div>
          {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
        </header>
      )}
      <div className={bodyClassName}>{children}</div>
    </section>
  )
}

//Panel 与 Card 同接口但没有卡片外壳：一页里叠多张 Card 会变成"贴了一墙便签"
export function Panel({ title, description, actions, children, className }: CardProps) {
  const hasHeader = Boolean(title || description || actions)
  return (
    <section className={cn('border-t border-line pt-6', className)}>
      {hasHeader && (
        <header className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            {title && <h2 className="font-serif text-lg text-ink">{title}</h2>}
            {description && <p className="mt-1 max-w-prose text-xs text-ink-faint">{description}</p>}
          </div>
          {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
        </header>
      )}
      {children}
    </section>
  )
}

export type BadgeTone = 'ok' | 'warn' | 'danger' | 'neutral'

const BADGE_TONE: Record<BadgeTone, string> = {
  ok: 'bg-ok/15 text-ok-text border border-ok/35',
  warn: 'bg-warn/20 text-warn-text border border-warn/50',
  danger: 'bg-danger/12 text-danger-text border border-danger/35',
  neutral: 'bg-subtle text-ink-soft border border-line',
}

type BadgeProps = {
  tone?: BadgeTone
  icon?: LucideIcon
  children: ReactNode
  className?: string
  title?: string
}

//状态一律「图标 + 文字」，不靠颜色单独传递
export function Badge({ tone = 'neutral', icon: Icon, children, className, title }: BadgeProps) {
  return (
    <span className={cn('badge', BADGE_TONE[tone], className)} title={title}>
      {Icon && <Icon aria-hidden className="size-3.5 shrink-0" />}
      {children}
    </span>
  )
}

//热重载徽章：配置字段与分组都用它，语义是「保存即生效 / 需重启」
export function HotBadge({ hot }: { hot: boolean }) {
  return hot ? (
    <Badge tone="ok" icon={Zap} title="保存即生效，无需重启">
      热
    </Badge>
  ) : (
    <Badge tone="warn" icon={Power} title="需重启进程后生效">
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

export const Td = ({
  children,
  className,
  colSpan,
}: {
  children?: ReactNode
  className?: string
  colSpan?: number
}) => (
  <td colSpan={colSpan} className={className}>
    {children}
  </td>
)

export const Skeleton = ({ className }: { className?: string }) => (
  <span aria-hidden className={cn('animate-breathe block h-4 rounded-full bg-subtle', className)} />
)

//加载态用骨架屏而不是转圈遮罩，避免闪烁
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
      className={cn(
        'inline-block size-4 animate-spin rounded-full border-2 border-current/25 border-t-current',
        className,
      )}
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
    <div
      className={cn(
        'flex flex-col items-center gap-4 rounded-[2rem] border border-dashed border-line px-6 py-12 text-center',
        className,
      )}
    >
      <span className="flex size-14 items-center justify-center rounded-full bg-subtle">
        <Icon aria-hidden className="size-6 text-ink-faint" />
      </span>
      <div>
        <p className="font-serif text-lg text-ink">{title}</p>
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
        className="pointer-events-none absolute bottom-full left-1/2 z-30 mb-2 w-max max-w-xs -translate-x-1/2 rounded-[1.25rem] border border-line bg-surface px-3 py-2 text-xs text-ink-soft opacity-0 shadow-md transition-opacity duration-300 ease-in-out group-hover/tip:opacity-100 group-focus-within/tip:opacity-100"
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
        timer.current = window.setTimeout(() => setDone(false), 1500)
      }}
    >
      {done ? '已复制' : label}
    </Button>
  )
}

export function DangerNote({ children }: { children?: ReactNode }) {
  return (
    <p className="mt-4 flex items-start gap-2 rounded-[1.5rem] border border-danger/30 bg-danger/8 p-3 text-xs text-ink-soft">
      <TriangleAlert aria-hidden className="mt-0.5 size-4 shrink-0 text-danger-text" />
      {children ?? '这是危险操作，可能产生不可逆后果或上游副作用。'}
    </p>
  )
}
