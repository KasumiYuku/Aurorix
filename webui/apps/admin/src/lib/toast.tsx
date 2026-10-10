import { useSyncExternalStore } from 'react'
import { cn } from '@aurorix/webui'

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

/** 轻提示：错误停留更久，避免一闪而过。 */
export function toast(text: string, kind: ToastKind = 'ok') {
  const id = nextId++
  items = [...items, { id, kind, text }]
  for (const listener of listeners) listener()
  window.setTimeout(() => dismiss(id), kind === 'err' ? 6000 : 3200)
}

const TONE: Record<ToastKind, string> = {
  ok: 'border-ok/40 text-ok-text',
  err: 'border-danger/40 text-danger-text',
  info: 'border-line text-ink-soft',
}

export function Toaster() {
  const list = useSyncExternalStore(subscribe, getItems)
  if (list.length === 0) return null
  return (
    <div className="pointer-events-none fixed bottom-6 right-6 z-50 flex w-full max-w-sm flex-col gap-2">
      {list.map((item) => (
        <button
          key={item.id}
          type="button"
          onClick={() => dismiss(item.id)}
          className={cn(
            'animate-fade-rise pointer-events-auto rounded-[1.25rem] border bg-surface px-4 py-3 text-left text-sm shadow-md',
            TONE[item.kind],
          )}
        >
          {item.text}
        </button>
      ))}
    </div>
  )
}
