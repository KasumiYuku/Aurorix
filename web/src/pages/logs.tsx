import { createEffect, createMemo, createSignal, For, Show } from 'solid-js'
import { api, LogEntry, LogsView } from '../api'
import { Button, Card, Empty, Icon, Select, fmtTime, downloadLogs } from '../ui'
import { onLog } from '../live'

const LEVELS = ['ALL', 'DEBUG', 'INFO', 'WARN', 'ERROR'] as const
type Level = (typeof LEVELS)[number]
const LEVEL_WEIGHT: Record<string, number> = { DEBUG: 0, INFO: 1, WARN: 2, ERROR: 3 }

const LEVEL_PILL: Record<string, string> = {
  DEBUG: 'text-muted-foreground-2',
  INFO: 'text-muted-foreground',
  WARN: 'bg-amber-50 text-amber-700 dark:bg-amber-400/15 dark:text-amber-300',
  ERROR: 'bg-red-50 text-red-600 dark:bg-red-400/15 dark:text-red-300',
}

function applyView(cur: { entries: LogEntry[]; scopes: string[]; total: number; errors: number }, v: LogsView) {
  const merged = v.entries.slice()
  const maxSnap = merged[0]?.id ?? -1
  const known = new Set(merged.map((e) => e.id))
  for (const e of cur.entries) {
    if (e.id > maxSnap && !known.has(e.id)) merged.unshift(e)
  }
  return { ...v, entries: merged.slice(0, 2000) }
}

export default function LogsPage() {
  const [view, setView] = createSignal<LogsView>({ entries: [], scopes: [], total: 0, errors: 0 })
  const [level, setLevel] = createSignal<Level>('ALL')
  const [scope, setScope] = createSignal('ALL')
  const [query, setQuery] = createSignal('')
  const [live, setLive] = createSignal(true)
  const [busy, setBusy] = createSignal(false)

  const append = (entry: LogEntry) => {
    setView((v) => ({
      ...v,
      total: Math.max(v.total, entry.id),
      entries: [entry, ...v.entries.filter((x) => x.id !== entry.id)].slice(0, 2000),
    }))
  }

  createEffect(() => {
    if (!live()) return
    let active = true
    let off: (() => void) | undefined
    const subscribe = () => {
      if (active) off = onLog(append)
    }
    setBusy(true)
    api<LogsView>('/api/logs?limit=500')
      .then((v) => setView((cur) => applyView(cur, v)))
      .catch(() => {})
      .finally(() => {
        setBusy(false)
        subscribe()
      })
    return () => {
      active = false
      off?.()
    }
  })

  const load = async () => {
    setBusy(true)
    try {
      const v = await api<LogsView>('/api/logs?limit=500')
      setView((cur) => applyView(cur, v))
    } catch {
    } finally {
      setBusy(false)
    }
  }

  const filtered = createMemo(() => {
    const minW = level() === 'ALL' ? -1 : LEVEL_WEIGHT[level()]
    const sc = scope()
    const q = query().trim().toLowerCase()
    return view().entries.filter((e) => {
      if (minW >= 0 && LEVEL_WEIGHT[e.level] < minW) return false
      if (sc !== 'ALL' && e.scope !== sc) return false
      if (q && !e.msg.toLowerCase().includes(q)) return false
      return true
    })
  })

  let listEl: HTMLDivElement | undefined
  const [following, setFollowing] = createSignal(true)
  createEffect(() => {
    view().entries.length
    if (following() && listEl) listEl.scrollTop = 0
  })
  const onListScroll = () => {
    if (listEl && listEl.scrollTop > 24) setFollowing(false)
  }
  const backToLatest = () => {
    setFollowing(true)
    if (listEl) listEl.scrollTop = 0
  }

  return (
    <div class="px-rise flex min-h-0 flex-1 flex-col gap-4">
      <div>
        <h2 class="font-display text-[26px] font-semibold leading-tight tracking-tight text-foreground">日志</h2>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <div class="inline-flex gap-0.5 rounded-full border border-line-3 bg-card p-1 shadow-sm">
          <For each={LEVELS}>
            {(lv) => (
              <button
                class={`rounded-full px-3 py-1 text-[12.5px] transition-colors duration-300 outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 ${
                  level() === lv ? 'bg-primary-50 font-medium text-primary-700 dark:bg-primary-400/15 dark:text-primary-300' : 'text-muted-foreground hover:text-foreground'
                }`}
                onClick={() => setLevel(lv)}
              >
                {lv}
              </button>
            )}
          </For>
        </div>
        <div class="w-[150px] shrink-0">
          <Select
            options={[{ label: '全部来源', value: 'ALL' }, ...view().scopes.map((s) => ({ label: s, value: s }))]}
            value={scope()}
            onChange={(v) => setScope(v)}
          />
        </div>
        <div class="relative">
          <Icon name="search" size={14} class="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground-2" />
          <input
            class="h-9 w-[220px] rounded-full border border-line-3 bg-field pl-8 pr-3.5 text-[12.5px] text-foreground outline-none transition-all duration-300 placeholder:text-muted-foreground-2 focus:border-primary-600 focus:ring-2 focus:ring-primary-500/30"
            placeholder="搜索日志内容"
            value={query()}
            onInput={(e) => setQuery(e.currentTarget.value)}
          />
        </div>
        <div class="flex-1" />
        <Button variant={live() ? 'primary' : 'ghost'} onClick={() => setLive(!live())}>
          <Icon name={live() ? 'pause' : 'play'} size={14} />
          {live() ? '暂停' : '实时'}
        </Button>
        <Button onClick={() => downloadLogs().catch(() => {})}>导出</Button>
        <Button onClick={load} disabled={busy()}>
          <Icon name="refresh" size={14} />
          刷新
        </Button>
      </div>

      <Card class="relative flex min-h-[320px] flex-1 flex-col overflow-hidden">
        <div class="grid shrink-0 grid-cols-[150px_56px_92px_1fr] gap-3 border-b border-card-divider px-5 py-2.5 text-[11px] font-medium uppercase tracking-wider text-muted-foreground-2">
          <span>时间</span>
          <span>级别</span>
          <span class="max-sm:hidden">来源</span>
          <span>内容</span>
        </div>
        <Show when={filtered().length} fallback={<Empty text="没有匹配的日志" />}>
          <div class="min-h-0 flex-1 overflow-y-auto" ref={listEl} onScroll={onListScroll}>
            <For each={filtered().slice(0, 800)}>
              {(e) => (
                <div class="px-rise grid grid-cols-[150px_56px_92px_1fr] items-baseline gap-3 border-b border-card-divider px-5 py-[4.5px] font-mono text-[12.3px] tnum transition-colors duration-300 last:border-b-0 hover:bg-muted-hover">
                  <span class="whitespace-nowrap text-muted-foreground-2">{fmtTime(e.time)}</span>
                  <span class={`inline-block justify-self-start rounded-full px-2 py-px text-[10px] font-semibold uppercase tracking-wider ${LEVEL_PILL[e.level] ?? LEVEL_PILL.INFO}`}>
                    {e.level}
                  </span>
                  <span class="truncate text-primary-600 dark:text-primary-400">{e.scope}</span>
                  <span class="whitespace-pre-wrap break-words text-foreground" style={{ 'font-family': 'var(--font-sans)' }}>{e.msg}</span>
                </div>
              )}
            </For>
          </div>
          <Show when={!following() && live()}>
            <button
              class="absolute bottom-4 left-1/2 inline-flex -translate-x-1/2 items-center gap-1.5 rounded-full border border-line-3 bg-card px-4 py-1.5 text-[12.5px] text-foreground shadow-md transition-colors duration-300 hover:border-primary-500 hover:text-primary-600 dark:hover:text-primary-400"
              onClick={backToLatest}
            >
              <Icon name="chevron" size={14} class="rotate-180" />
              回到最新
            </button>
          </Show>
        </Show>
      </Card>
    </div>
  )
}