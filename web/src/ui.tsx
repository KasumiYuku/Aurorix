import { createSignal, For, Show, onCleanup, onMount, JSX } from 'solid-js'
import { api } from './api'

// ---------- 图标 ----------

const ICONS = {
  grid: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z',
  list: 'M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01',
  package: 'M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16zM3.3 7l8.7 5 8.7-5M12 22V12',
  image: 'M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zM8.5 10.5a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM21 15l-5-5L5 21',
  clock: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zM12 6v6l4 2',
  sliders: 'M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6',
  power: 'M18.36 6.64a9 9 0 1 1-12.73 0M12 2v10',
  refresh: 'M23 4v6h-6M1 20v-6h6M3.5 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.5 15',
  search: 'M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM21 21l-4.35-4.35',
  pause: 'M8 5v14M16 5v14',
  play: 'M7 4l13 8-13 8z',
  sun: 'M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10zM12 1v2M12 21v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M1 12h2M21 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4',
  moon: 'M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z',
  monitor: 'M2 3h20v14H2zM8 21h8M12 17v4',
  x: 'M18 6L6 18M6 6l12 12',
  check: 'M20 6L9 17l-5-5',
  alert: 'M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0zM12 9v4M12 17h.01',
  activity: 'M22 12h-4l-3 9L9 3l-3 9H2',
  info: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 16v-4M12 8h.01',
  chevron: 'M6 9l6 6 6-6',
  cpu: 'M7 7h10v10H7zM10 7V3M14 7V3M10 21v-4M14 21v-4M7 10H3M7 14H3M21 10h-4M21 14h-4',
  layers: 'M12 2 2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5',
  users: 'M17 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M13 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0zM22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75',
  message: 'M21 15a2 2 0 0 1-2 2H8l-4 4V5a2 2 0 0 1 2-2h13a2 2 0 0 1 2 2z',
  trending: 'M23 6l-9.5 9.5-5-5L1 18M17 6h6v6',
} as const

export type IconName = keyof typeof ICONS

export function Icon(props: { name: IconName; size?: number; class?: string }) {
  return (
    <svg
      width={props.size ?? 18}
      height={props.size ?? 18}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.8"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
      class={props.class}
    >
      <path d={ICONS[props.name]} />
    </svg>
  )
}

// ---------- 自绘勾选框 ----------

export function Checkbox(props: { checked: boolean; onChange: (v: boolean) => void; class?: string }) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={props.checked}
      class={`grid h-4.5 w-4.5 shrink-0 cursor-pointer place-items-center rounded-[5px] border transition-all duration-300 outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 ${
        props.checked
          ? 'border-primary-600 bg-primary-600 text-primary-foreground'
          : 'border-line-4 bg-field hover:border-primary-500/50'
      } ${props.class ?? ''}`}
      onClick={(e) => {
        e.stopPropagation()
        props.onChange(!props.checked)
      }}
    >
      <Show when={props.checked}>
        <Icon name="check" size={11} />
      </Show>
    </button>
  )
}

// ---------- 按钮 (有机胶囊) ----------

type BtnVariant = 'primary' | 'ghost' | 'danger' | 'soft'
const BTN: Record<BtnVariant, string> = {
  primary: 'bg-primary-600 text-primary-foreground shadow-sm hover:bg-primary-700 active:bg-primary-800 disabled:hover:bg-primary-600',
  ghost: 'bg-secondary text-secondary-foreground border border-line-3 shadow-sm hover:bg-secondary-hover active:bg-secondary-active',
  danger: 'bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive-hover active:brightness-95 disabled:hover:bg-destructive',
  soft: 'bg-primary-50 text-primary-700 hover:bg-primary-100 dark:bg-primary-400/15 dark:text-primary-300 dark:hover:bg-primary-400/25',
}

export function Button(props: {
  variant?: BtnVariant
  disabled?: boolean
  onClick?: (e: MouseEvent) => void
  children?: JSX.Element
  type?: 'button' | 'submit'
  title?: string
  class?: string
}) {
  return (
    <button
      type={props.type ?? 'button'}
      class={`inline-flex h-9 shrink-0 cursor-pointer items-center justify-center gap-1.5 rounded-full px-4.5 text-[13px] font-medium transition-all duration-300 outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 disabled:pointer-events-none disabled:opacity-50 active:translate-y-px ${BTN[props.variant ?? 'ghost']} ${props.class ?? ''}`}
      disabled={props.disabled}
      onClick={props.onClick}
      title={props.title}
    >
      {props.children}
    </button>
  )
}

// ---------- 开关 ----------

export function Switch(props: { checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={props.checked}
      disabled={props.disabled}
      onClick={() => props.onChange(!props.checked)}
      class={`relative inline-flex h-[24px] w-10 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors duration-300 outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 disabled:cursor-not-allowed disabled:opacity-50 ${props.checked ? 'bg-primary-600' : 'bg-surface-3 dark:bg-surface-2'}`}
    >
      <span
        class={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-sm transition-transform duration-300 ${props.checked ? 'translate-x-5' : 'translate-x-0'}`}
      />
    </button>
  )
}

// ---------- 卡片 (不规则圆角纸片) ----------

export function Card(props: { title?: JSX.Element; actions?: JSX.Element; children: JSX.Element; class?: string }) {
  return (
    <section class={`px-blob border border-line-2 bg-card ${props.class ?? ''}`}>
      <Show when={props.title || props.actions}>
        <header class="flex min-h-[54px] items-center justify-between gap-3 border-b border-card-divider px-6">
          <h3 class="font-display text-[15.5px] font-semibold text-foreground">{props.title}</h3>
          <div class="flex items-center gap-2">{props.actions}</div>
        </header>
      </Show>
      {props.children}
    </section>
  )
}

// ---------- 表单字段 ----------

export function Field(props: { label: string; desc?: string; children: JSX.Element; wide?: boolean }) {
  return (
    <div class={`grid gap-3 border-b border-card-divider px-6 py-5 last:border-b-0 ${props.wide ? 'grid-cols-1' : 'sm:grid-cols-[minmax(0,220px)_1fr] sm:items-start'}`}>
      <div class="min-w-0">
        <label class="text-[13px] font-medium text-foreground">{props.label}</label>
        <Show when={props.desc}>
          <p class="mt-1 text-xs leading-relaxed text-muted-foreground-2">{props.desc}</p>
        </Show>
      </div>
      <div class="min-w-0 sm:max-w-[560px]">{props.children}</div>
    </div>
  )
}

// ---------- Toast ----------

type ToastKind = 'ok' | 'err' | 'info'
interface ToastItem {
  id: number
  kind: ToastKind
  text: string
}
const TOAST_ICON: Record<ToastKind, IconName> = { ok: 'check', err: 'alert', info: 'info' }
const TOAST_ACCENT: Record<ToastKind, string> = {
  ok: 'text-primary-600 dark:text-primary-400',
  err: 'text-destructive',
  info: 'text-muted-foreground',
}

const [toasts, setToasts] = createSignal<ToastItem[]>([])
let toastSeq = 0

export function toast(text: string, kind: ToastKind = 'ok') {
  const id = ++toastSeq
  setToasts((list) => [...list.slice(-4), { id, kind, text }])
  setTimeout(() => setToasts((list) => list.filter((t) => t.id !== id)), 3400)
}

export function Toasts() {
  return (
    <div class="pointer-events-none fixed right-5 top-5 z-[120] flex w-[min(92vw,380px)] flex-col gap-2.5" aria-live="polite">
      <For each={toasts()}>
        {(item) => (
          <div
            class={`px-toast-in pointer-events-auto flex items-center gap-3 rounded-full border border-line-3 bg-card px-5 py-3 text-[13px] text-foreground shadow-md`}
          >
            <Icon name={TOAST_ICON[item.kind]} size={16} class={TOAST_ACCENT[item.kind]} />
            <span class="break-words">{item.text}</span>
          </div>
        )}
      </For>
    </div>
  )
}

// ---------- 确认对话框 ----------

export function Modal(props: {
  open: boolean
  title: string
  children?: JSX.Element
  footer?: JSX.Element
  onClose?: () => void
}) {
  return (
    <Show when={props.open}>
      <div class="px-fade fixed inset-0 z-[110] grid place-items-center bg-[color-mix(in_srgb,var(--background-2)_55%,transparent)] p-5 backdrop-blur-[3px]" onClick={props.onClose}>
        <div
          class="px-pop w-full max-w-[480px] overflow-hidden border border-line-3 bg-card shadow-md"
          style={{ 'border-radius': '1.9rem 2.1rem 1.7rem 2.3rem' }}
          role="dialog"
          aria-modal="true"
          onClick={(e) => e.stopPropagation()}
        >
          <header class="flex items-center justify-between border-b border-card-divider px-6 py-4">
            <h3 class="font-display text-[16.5px] font-semibold text-foreground">{props.title}</h3>
            <IconBtn onClick={props.onClose} title="关闭">
              <Icon name="x" size={16} />
            </IconBtn>
          </header>
          <div class="px-6 py-5 text-[13.5px] leading-relaxed text-muted-foreground">{props.children}</div>
          <Show when={props.footer}>
            <footer class="flex justify-end gap-2.5 border-t border-card-divider bg-card-footer px-6 py-4">{props.footer}</footer>
          </Show>
        </div>
      </div>
    </Show>
  )
}

// ---------- 图标按钮 ----------

export function IconBtn(props: { onClick?: (e: MouseEvent) => void; children?: JSX.Element; title?: string; class?: string }) {
  return (
    <button
      type="button"
      title={props.title}
      onClick={props.onClick}
      class={`inline-grid h-8 w-8 cursor-pointer place-items-center rounded-full border border-line-3 bg-card text-muted-foreground transition-all duration-300 outline-none hover:border-line-4 hover:text-foreground focus-visible:ring-2 focus-visible:ring-primary-500/40 active:translate-y-px ${props.class ?? ''}`}
    >
      {props.children}
    </button>
  )
}

// ---------- 通用小组件 ----------

const BADGE: Record<string, string> = {
  ok: 'border-transparent bg-primary-50 text-primary-700 dark:bg-primary-400/15 dark:text-primary-300',
  warn: 'border-transparent bg-amber-50 text-amber-700 dark:bg-amber-400/15 dark:text-amber-300',
  err: 'border-transparent bg-red-50 text-red-600 dark:bg-red-400/15 dark:text-red-300',
  dim: 'border-transparent bg-surface text-muted-foreground',
  accent: 'border-transparent bg-primary-50 text-primary-700 dark:bg-primary-400/15 dark:text-primary-300',
}

export function Badge(props: { children: JSX.Element; tone?: 'ok' | 'warn' | 'err' | 'dim' | 'accent'; class?: string }) {
  return (
    <span
      class={`inline-flex items-center gap-1 whitespace-nowrap rounded-full border px-3 py-0.5 text-[11.5px] font-medium ${BADGE[props.tone ?? 'dim'] ?? BADGE.dim} ${props.class ?? ''}`}
    >
      {props.children}
    </span>
  )
}

export function Empty(props: { text: string }) {
  return (
    <div class="flex flex-col items-center justify-center gap-2.5 px-6 py-14 text-center">
      <span class="px-blob-sm grid h-12 w-12 place-items-center border border-dashed border-line-4 text-muted-foreground-2">
        <Icon name="layers" size={20} />
      </span>
      <span class="text-[13px] text-muted-foreground-2">{props.text}</span>
    </div>
  )
}

export function SkeletonRows(props: { rows?: number }) {
  const widths = [42, 58, 34, 66, 50]
  const widths2 = [74, 88, 62, 80, 70]
  return (
    <div class="flex flex-col gap-3.5 px-6 py-5">
      <For each={Array.from({ length: props.rows ?? 4 })}>
        {(_, i) => {
          const n = i()
          return (
            <div class="flex items-center gap-5">
              <span class="px-skeleton" style={{ width: `${widths[n % widths.length]}%`, height: '15px' }} />
              <span class="px-skeleton" style={{ width: `${widths2[n % widths2.length]}%`, height: '15px' }} />
            </div>
          )
        }}
      </For>
    </div>
  )
}

// ---------- 格式化 ----------

export function fmtTime(ms: number): string {
  if (!ms) return '—'
  const d = new Date(ms)
  const p = (n: number, w = 2) => String(n).padStart(w, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}.${p(d.getMilliseconds(), 3)}`
}

export function fmtClock(ms: number): string {
  if (!ms) return '—'
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

export function fmtDur(sec: number): string {
  sec = Math.max(0, Math.floor(sec))
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  const parts: string[] = []
  if (d) parts.push(`${d}天`)
  if (h) parts.push(`${h}时`)
  if (m) parts.push(`${m}分`)
  parts.push(`${s}秒`)
  return parts.join(' ')
}

export function fmtBytes(n: number): string {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

export function fmtNum(n: number | undefined): string {
  return (n ?? 0).toLocaleString('zh-CN')
}

export async function fetchText(path: string): Promise<string> {
  const res = await fetch('/admin' + path, { credentials: 'same-origin' })
  if (!res.ok) throw new Error('导出失败')
  return res.text()
}

// ---------- 自定义单选下拉 ----------

type SelectOption = string | { label: string; value: string }

function normOption(opt: SelectOption) {
  return typeof opt === 'string' ? { label: opt, value: opt } : opt
}

export function Select(props: {
  options: SelectOption[]
  value?: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  const [open, setOpen] = createSignal(false)
  let root: HTMLDivElement | undefined

  const close = (e: MouseEvent) => {
    if (root && !root.contains(e.target as Node)) setOpen(false)
  }
  onMount(() => document.addEventListener('mousedown', close))
  onCleanup(() => document.removeEventListener('mousedown', close))

  const opts = () => props.options.map(normOption)
  const current = () => opts().find((o) => o.value === props.value)

  const pick = (value: string) => {
    props.onChange(value)
    setOpen(false)
  }

  return (
    <div class="relative w-full" ref={root}>
      <button
        type="button"
        class={`flex h-9.5 w-full cursor-pointer items-center justify-between gap-2 rounded-full border bg-field px-4 text-left text-[13px] text-muted-foreground outline-none transition-all duration-300 focus-visible:ring-2 focus-visible:ring-primary-500/40 ${open() ? 'border-primary-600' : 'border-line-3 hover:border-line-4'}`}
        onClick={() => setOpen(!open())}
      >
        <span class="truncate">{current()?.label ?? props.placeholder ?? '请选择'}</span>
        <Icon name="chevron" size={14} class="shrink-0" />
      </button>
      <Show when={open()}>
        <div class="px-pop absolute left-0 right-0 top-[calc(100%+6px)] z-[60] overflow-hidden border border-line-3 bg-dropdown shadow-md" style={{ 'border-radius': '1.15rem 1.35rem 1rem 1.45rem' }}>
          <div class="max-h-60 overflow-y-auto p-1.5">
            <For each={opts()}>
              {(o) => (
                <button
                  type="button"
                  class={`flex w-full cursor-pointer items-center justify-between gap-2 rounded-full px-3 py-1.5 font-mono text-xs text-foreground transition-colors duration-300 ${
                    props.value === o.value ? 'bg-primary-50 text-primary-700 dark:bg-primary-400/15 dark:text-primary-300' : 'hover:bg-muted-hover'
                  }`}
                  onClick={() => pick(o.value)}
                >
                  <span class="truncate">{o.label}</span>
                  <Show when={props.value === o.value}>
                    <Icon name="check" size={14} />
                  </Show>
                </button>
              )}
            </For>
          </div>
        </div>
      </Show>
    </div>
  )
}

// ---------- 下拉多选 ----------

export function MultiSelect(props: {
  options: string[]
  values: string[]
  onChange: (v: string[]) => void
  placeholder?: string
}) {
  const [open, setOpen] = createSignal(false)
  let root: HTMLDivElement | undefined

  const close = (e: MouseEvent) => {
    if (root && !root.contains(e.target as Node)) setOpen(false)
  }
  onMount(() => document.addEventListener('mousedown', close))
  onCleanup(() => document.removeEventListener('mousedown', close))

  const toggle = (value: string) =>
    props.onChange(props.values.includes(value) ? props.values.filter((v) => v !== value) : [...props.values, value])

  const label = () => {
    if (!props.values.length) return props.placeholder ?? '请选择'
    return `${props.values.length} 项已选`
  }

  return (
    <div class="relative w-full" ref={root}>
      <button
        type="button"
        class={`flex h-9.5 w-full cursor-pointer items-center justify-between gap-2 rounded-full border bg-field px-4 text-left text-[13px] text-muted-foreground outline-none transition-all duration-300 focus-visible:ring-2 focus-visible:ring-primary-500/40 ${open() ? 'border-primary-600' : 'border-line-3 hover:border-line-4'}`}
        onClick={() => setOpen(!open())}
      >
        <span class="truncate">{label()}</span>
        <Icon name="chevron" size={14} class="shrink-0" />
      </button>
      <Show when={open()}>
        <div class="px-pop absolute left-0 right-0 top-[calc(100%+6px)] z-[60] overflow-hidden border border-line-3 bg-dropdown shadow-md" style={{ 'border-radius': '1.15rem 1.35rem 1rem 1.45rem' }}>
          <div class="flex justify-end gap-3 border-b border-card-divider bg-dropdown-header px-4 py-2">
            <button class="cursor-pointer text-xs text-primary-600 hover:underline dark:text-primary-400" onClick={() => props.onChange([])}>
              清空
            </button>
            <button class="cursor-pointer text-xs text-primary-600 hover:underline dark:text-primary-400" onClick={() => props.onChange([...props.options])}>
              全选
            </button>
          </div>
          <div class="max-h-60 overflow-y-auto p-1.5">
            <For each={props.options}>
              {(opt) => (
                <div
                  class={`flex cursor-pointer items-center gap-2 rounded-full px-3 py-1.5 transition-colors duration-300 ${props.values.includes(opt) ? 'bg-primary-50 dark:bg-primary-400/10' : 'hover:bg-muted-hover'}`}
                  onClick={() => toggle(opt)}
                >
                  <Checkbox checked={props.values.includes(opt)} onChange={() => toggle(opt)} />
                  <span class="flex-1 truncate font-mono text-xs text-foreground">{opt}</span>
                  <Show when={props.values.includes(opt)}>
                    <Icon name="check" size={14} class="text-primary-600 dark:text-primary-400" />
                  </Show>
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>
    </div>
  )
}

export async function downloadLogs() {
  const view = await api<{ entries: { time: number; level: string; scope: string; msg: string }[] }>(
    '/api/logs?limit=2048',
  )
  const lines = view.entries
    .slice()
    .reverse()
    .map((e) => `${fmtTime(e.time)} [${e.level}] [${e.scope}] ${e.msg}`)
    .join('\n')
  const blob = new Blob([lines + '\n'], { type: 'text/plain;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `aurorix-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.log`
  a.click()
  URL.revokeObjectURL(a.href)
}
