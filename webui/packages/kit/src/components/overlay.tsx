//浮层与切换器：Modal / Drawer / ConfirmDialog / Tabs / Segmented。
import { useEffect, useId, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'
import { cn } from '../lib/cn'
import { Button, DangerNote } from './ui'
import { Input } from './form'

function useBodyLock(active: boolean) {
  useEffect(() => {
    if (!active) return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [active])
}

function useEscape(active: boolean, onEscape: () => void) {
  useEffect(() => {
    if (!active) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onEscape()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [active, onEscape])
}

type ModalProps = {
  open: boolean
  onClose: () => void
  title: ReactNode
  description?: ReactNode
  children?: ReactNode
  footer?: ReactNode
  width?: 'sm' | 'md' | 'lg' | 'xl'
}

const MODAL_WIDTH = {
  sm: 'max-w-md',
  md: 'max-w-xl',
  lg: 'max-w-3xl',
  xl: 'max-w-5xl',
} as const

export function Modal({ open, onClose, title, description, children, footer, width = 'md' }: ModalProps) {
  const titleId = useId()
  useBodyLock(open)
  useEscape(open, onClose)
  if (!open) return null

  return createPortal(
    <div className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-scrim p-4 py-10 md:p-8">
      <div aria-hidden className="absolute inset-0" onClick={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className={cn(
          'animate-fade-rise texture-paper relative w-full rounded-[2rem] border border-line bg-surface p-6 shadow-md md:p-8',
          MODAL_WIDTH[width],
        )}
      >
        <header className="mb-5 flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h2 id={titleId} className="font-serif text-xl text-ink md:text-2xl">
              {title}
            </h2>
            {description && <p className="mt-1 text-sm text-ink-soft">{description}</p>}
          </div>
          <Button variant="ghost" size="icon" aria-label="关闭" onClick={onClose} icon={X} />
        </header>
        {children}
        {footer && <footer className="mt-6 flex flex-wrap justify-end gap-3">{footer}</footer>}
      </div>
    </div>,
    document.body,
  )
}

type DrawerProps = {
  open: boolean
  onClose: () => void
  title: ReactNode
  description?: ReactNode
  children?: ReactNode
  footer?: ReactNode
  width?: 'md' | 'lg'
}

export function Drawer({ open, onClose, title, description, children, footer, width = 'md' }: DrawerProps) {
  const titleId = useId()
  useBodyLock(open)
  useEscape(open, onClose)
  if (!open) return null

  return createPortal(
    <div className="fixed inset-0 z-40 flex justify-end bg-scrim">
      <div aria-hidden className="absolute inset-0" onClick={onClose} />
      <aside
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className={cn(
          'animate-fade-rise relative flex h-full w-full flex-col border-l border-line bg-surface shadow-md',
          width === 'lg' ? 'md:max-w-3xl' : 'md:max-w-xl',
        )}
      >
        <header className="flex items-start justify-between gap-4 border-b border-line p-6">
          <div className="min-w-0">
            <h2 id={titleId} className="font-serif text-xl text-ink">
              {title}
            </h2>
            {description && <p className="mt-1 text-sm text-ink-soft">{description}</p>}
          </div>
          <Button variant="ghost" size="icon" aria-label="关闭" onClick={onClose} icon={X} />
        </header>
        <div className="flex-1 overflow-y-auto p-6">{children}</div>
        {footer && <footer className="flex flex-wrap justify-end gap-3 border-t border-line p-6">{footer}</footer>}
      </aside>
    </div>,
    document.body,
  )
}

type ConfirmDialogProps = {
  open: boolean
  onClose: () => void
  onConfirm: () => void
  title: string
  description?: ReactNode
  confirmLabel?: string
  danger?: boolean
  pending?: boolean
  requireText?: string
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  description,
  confirmLabel = '确认执行',
  danger = false,
  pending = false,
  requireText,
}: ConfirmDialogProps) {
  const [typed, setTyped] = useState('')
  const inputId = useId()

  useEffect(() => {
    if (open) setTyped('')
  }, [open])

  const ready = !requireText || typed.trim() === requireText

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      description={description}
      width="sm"
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={pending}>
            取消
          </Button>
          <Button variant={danger ? 'danger' : 'primary'} onClick={onConfirm} loading={pending} disabled={!ready}>
            {confirmLabel}
          </Button>
        </>
      }
    >
      {requireText && (
        <div className="flex flex-col gap-2">
          <label htmlFor={inputId} className="text-sm text-ink-soft">
            请输入 <code className="code-block inline-block px-2 py-0.5">{requireText}</code> 以确认
          </label>
          <Input
            id={inputId}
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            aria-invalid={typed !== '' && !ready}
            placeholder={requireText}
            autoComplete="off"
          />
          {typed !== '' && !ready && <p className="text-xs text-danger-text">输入不匹配，无法继续</p>}
        </div>
      )}
      {danger && <DangerNote />}
    </Modal>
  )
}

export type TabItem = { id: string; label: string; badge?: ReactNode }

type TabsProps = {
  tabs: TabItem[]
  value: string
  onChange: (id: string) => void
  className?: string
}

export function Tabs({ tabs, value, onChange, className }: TabsProps) {
  return (
    <div
      role="tablist"
      className={cn('inline-flex flex-wrap gap-1 rounded-full border border-line bg-subtle p-1', className)}
    >
      {tabs.map((tab) => {
        const active = tab.id === value
        return (
          <button
            key={tab.id}
            role="tab"
            type="button"
            aria-selected={active}
            onClick={() => onChange(tab.id)}
            className={cn(
              'inline-flex min-h-11 items-center gap-2 rounded-full px-5 text-sm transition-colors duration-300 ease-in-out',
              active ? 'bg-surface font-medium text-accent-text' : 'text-ink-soft hover:text-accent-text',
            )}
          >
            {tab.label}
            {tab.badge}
          </button>
        )
      })}
    </div>
  )
}

type SegmentedProps<T extends string> = {
  options: { value: T; label: string }[]
  value: T
  onChange: (next: T) => void
  ariaLabel?: string
  className?: string
}

export function Segmented<T extends string>({ options, value, onChange, ariaLabel, className }: SegmentedProps<T>) {
  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      className={cn('inline-flex flex-wrap gap-1 rounded-full border border-line bg-subtle p-1', className)}
    >
      {options.map((o) => {
        const active = o.value === value
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(o.value)}
            className={cn(
              'inline-flex min-h-11 items-center rounded-full px-4 text-sm transition-colors duration-300 ease-in-out',
              active ? 'bg-surface font-medium text-accent-text' : 'text-ink-soft hover:text-accent-text',
            )}
          >
            {o.label}
          </button>
        )
      })}
    </div>
  )
}
