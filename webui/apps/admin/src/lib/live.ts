import { useSyncExternalStore } from 'react'
import { get } from './api'
import { queryClient, qk } from './query'
import type { JobInfo, LiveOverview, LogEntry } from './types'

export type ConnState = 'idle' | 'connecting' | 'open' | 'down'

const TOPIC_OVERVIEW = 'overview.snapshot'
const TOPIC_JOBS = 'jobs.list'
const TOPIC_LOG = 'log.entry'

interface Envelope {
  topic: string
  seq: number
  at: number
  data: unknown
}

let conn: ConnState = 'idle'
const connListeners = new Set<() => void>()

function subscribeConn(listener: () => void) {
  connListeners.add(listener)
  return () => {
    connListeners.delete(listener)
  }
}

function getConn() {
  return conn
}

function setConn(next: ConnState) {
  if (next === conn) return
  conn = next
  for (const listener of connListeners) listener()
}

export function useConnState(): ConnState {
  return useSyncExternalStore(subscribeConn, getConn)
}

const logListeners = new Set<(entry: LogEntry) => void>()

/** 订阅实时日志；返回注销函数。 */
export function subscribeLogs(cb: (entry: LogEntry) => void): () => void {
  logListeners.add(cb)
  return () => {
    logListeners.delete(cb)
  }
}

let source: EventSource | null = null
let refs = 0
let wantOpen = false
let failCount = 0
let lastSeq = 0
let lastActivity = 0
let lastProbe = 0
let reopenTimer: number | undefined
let watchdog: number | undefined

/** Shell 挂载时调用；引用计数避免多页面重复建连。 */
export function connectLive() {
  refs++
  if (refs > 1) return
  wantOpen = true
  failCount = 0
  lastSeq = 0
  if (!source) open()
  if (watchdog === undefined) watchdog = window.setInterval(watch, 5000)
}

/** Shell 卸载时调用，归零即整体断开。 */
export function disconnectLive() {
  refs--
  if (refs > 0) return
  wantOpen = false
  if (watchdog !== undefined) {
    clearInterval(watchdog)
    watchdog = undefined
  }
  if (reopenTimer !== undefined) {
    clearTimeout(reopenTimer)
    reopenTimer = undefined
  }
  source?.close()
  source = null
  lastSeq = 0
  setConn('idle')
}

function open() {
  if (!wantOpen || source) return
  setConn('connecting')
  const query = lastSeq > 0 ? `?last_event_id=${lastSeq}` : ''
  const src = new EventSource(`/admin/api/stream${query}`)
  source = src

  src.onopen = () => {
    failCount = 0
    lastActivity = Date.now()
    setConn('open')
  }

  src.onmessage = (event) => {
    lastActivity = Date.now()
    const seq = Number(event.lastEventId)
    if (Number.isFinite(seq) && seq > lastSeq) lastSeq = seq
    let envelope: Envelope
    try {
      envelope = JSON.parse(event.data) as Envelope
    } catch {
      return
    }
    dispatch(envelope)
  }

  src.onerror = () => {
    if (src !== source) return
    source = null
    src.close()
    setConn('down')
    if (!wantOpen) return
    failCount++
    scheduleReopen(Math.min(15000, 1000 * 2 ** Math.min(failCount - 1, 4)))
  }
}

function dispatch(envelope: Envelope) {
  switch (envelope.topic) {
    case TOPIC_OVERVIEW:
      queryClient.setQueryData(qk.overview, envelope.data as LiveOverview)
      break
    case TOPIC_JOBS:
      queryClient.setQueryData(qk.jobs, envelope.data as JobInfo[])
      break
    case TOPIC_LOG: {
      const entry = envelope.data as LogEntry
      for (const listener of logListeners) listener(entry)
      break
    }
  }
}

function scheduleReopen(ms: number) {
  if (reopenTimer !== undefined) clearTimeout(reopenTimer)
  reopenTimer = window.setTimeout(() => {
    reopenTimer = undefined
    open()
  }, ms)
}

function watch() {
  if (!wantOpen) return
  const now = Date.now()
  if (source && now - lastActivity > 12000) {
    source.close()
    source = null
    lastActivity = now
    open()
    return
  }
  if (now - lastProbe > 30000) {
    lastProbe = now
    get<{ ok: boolean }>('/me').catch(() => {})
  }
}
