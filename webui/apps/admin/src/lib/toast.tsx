import { useSyncExternalStore } from 'react'
import { Check, Info, TriangleAlert, X, type LucideIcon } from 'lucide-react'
import { Button, cn } from '@aurorix/webui'

export type ToastKind = 'ok' | 'err' | 'info'

interface ToastItem {
  id: number
  kind: ToastKind
  text: string
}

let items: ToastItem[] = []
let nextId = 1
const listeners = new Set<() => void>()

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function getItems() {
  return items
}

export function dismiss(id: number) {
  if (!items.some((item) => item.id === id)) return
  items = items.filter((item) => item.id !== id)
  for (const listener of listeners) listener()
}

export function toast(text: string, kind: ToastKind = 'ok') {
  const id = nextId++
  items = [...items, { id, kind, text }]
  for (const listener of listeners) listener()
  window.setTimeout(() => dismiss(id), kind === 'err' ? 6000 : 3200)
}

const TONE: Record<ToastKind, { icon: LucideIcon; className: string; label: string }> = {
  ok: { icon: Check, className: 'text-ok', label: '成功' },
  err: { icon: TriangleAlert, className: 'text-danger-ink', label: '失败' },
  info: { icon: Info, className: 'text-ink-soft', label: '提示' },
}

export function Toaster() {
  const list = useSyncExternalStore(subscribe, getItems)
  if (list.length === 0) return null
  return (
    <div
      aria-live="polite"
      className="pointer-events-none fixed bottom-6 right-6 z-50 flex w-full max-w-sm flex-col gap-2"
    >
      {list.map((item) => {
        const { icon: Icon, className, label } = TONE[item.kind]
        return (
          <div
            key={item.id}
            role={item.kind === 'err' ? 'alert' : 'status'}
            className={cn(
              'animate-fade-in pointer-events-auto flex items-start gap-2 rounded-md border border-line bg-surface px-3 py-2 text-sm text-ink shadow-md',
              item.kind === 'err' && 'border-danger/35',
            )}
          >
            <Icon aria-hidden className={cn('mt-0.5 size-4 shrink-0', className)} />
            <span className="min-w-0 flex-1 break-words">{item.text}</span>
            <span className="sr-only">{label}</span>
            <Button
              variant="ghost"
              size="icon"
              icon={X}
              aria-label="关闭提示"
              className="-mr-1 -mt-1 shrink-0"
              onClick={() => dismiss(item.id)}
            />
          </div>
        )
      })}
    </div>
  )
}
