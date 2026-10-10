import { createSignal } from 'solid-js'
import { api, LogEntry, Overview, JobInfo } from './api'

export type LiveOverview = Overview & { now: number }

const [overview, setOverview] = createSignal<LiveOverview | null>(null)
const [liveJobs, setLiveJobs] = createSignal<JobInfo[] | null>(null)
const logCbs = new Set<(e: LogEntry) => void>()

export const useLiveOverview = () => overview
export const useLiveJobs = () => liveJobs
/** 供页面本地更新快照(如档案手动刷新后即时回填) */
export function patchOverview(patch: Partial<LiveOverview>) {
  setOverview((cur) => (cur ? { ...cur, ...patch } : cur))
}

/** 订阅实时日志; 返回注销函数 */
export function onLog(cb: (e: LogEntry) => void): () => void {
  logCbs.add(cb)
  return () => logCbs.delete(cb)
}

let es: EventSource | null = null
let refs = 0
let wantOpen = false
let failCount = 0
let lastActivity = 0
let lastProbe = 0
let reopenTimer: ReturnType<typeof setTimeout> | undefined
let watchdog: ReturnType<typeof setInterval> | undefined

/** Shell 挂载时调用; 引用计数, 防止多页面重复建连 */
export function connectLive() {
  refs++
  if (refs > 1) return
  wantOpen = true
  failCount = 0
  if (!es) open()
  if (!watchdog) watchdog = setInterval(watch, 5000)
}

/** Shell 卸载时调用, 归零即整体断开 */
export function disconnectLive() {
  refs--
  if (refs > 0) return
  wantOpen = false
  if (watchdog) {
    clearInterval(watchdog)
    watchdog = undefined
  }
  if (reopenTimer) {
    clearTimeout(reopenTimer)
    reopenTimer = undefined
  }
  es?.close()
  es = null
  setOverview(null)
  setLiveJobs(null)
}

function open() {
  if (!wantOpen || es) return
  const src = new EventSource('/admin/api/stream')
  es = src
  src.addEventListener('snapshot', handleSnapshot)
  src.addEventListener('jobs', handleJobs)
  src.addEventListener('log', handleLog)
  src.onopen = () => {
    failCount = 0
  }
  src.onerror = () => {
    if (src !== es) return
    es = null
    src.close()
    if (!wantOpen) return
    failCount++
    const delay = Math.min(15000, 1000 * 2 ** Math.min(failCount - 1, 4))
    scheduleReopen(delay)
  }
}

function scheduleReopen(ms: number) {
  if (reopenTimer) clearTimeout(reopenTimer)
  reopenTimer = setTimeout(() => {
    reopenTimer = undefined
    open()
  }, ms)
}

function handleSnapshot(e: Event) {
  const data = JSON.parse((e as MessageEvent).data) as LiveOverview
  setOverview(data)
  lastActivity = Date.now()
}

function handleJobs(e: Event) {
  const data = JSON.parse((e as MessageEvent).data) as JobInfo[]
  setLiveJobs(data)
  lastActivity = Date.now()
}

function handleLog(e: Event) {
  const data = JSON.parse((e as MessageEvent).data) as LogEntry
  lastActivity = Date.now()
  for (const cb of logCbs) cb(data)
}

function watch() {
  if (!wantOpen) return
  const now = Date.now()
  if (es && now - lastActivity > 12000) {
    forceReopen()
    return
  }
  if (now - lastProbe > 30000) {
    lastProbe = now
    api('/api/me').catch(() => {})
  }
}

function forceReopen() {
  es?.close()
  es = null
  lastActivity = Date.now()
  open()
}
