export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch('/admin' + path, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  if (res.status === 401) {
    location.hash = '#/login'
    throw new ApiError(401, '未登录或会话已失效')
  }
  if (!res.ok) {
    let message = res.statusText || '请求失败'
    try {
      const body = await res.json()
      if (body?.error) message = body.error
    } catch {
      /* 保留状态文本 */
    }
    throw new ApiError(res.status, message)
  }
  return res.json() as Promise<T>
}

export const post = (path: string, body?: unknown) =>
  api(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) })

export const put = (path: string, body: unknown) => api(path, { method: 'PUT', body: JSON.stringify(body) })

// ---------- 类型 ----------

export interface LogEntry {
  id: number
  time: number
  level: 'DEBUG' | 'INFO' | 'WARN' | 'ERROR'
  scope: string
  msg: string
}

export interface LogsView {
  entries: LogEntry[]
  scopes: string[]
  total: number
  errors: number
}

export interface CounterSnapshot {
  recv: number
  sent: number
  button: number
}

export interface MemView {
  heap_alloc: number
  heap_sys: number
  num_gc: number
  last_gc: number
}

export interface RuntimeView {
  uptime_sec: number
  protocol: string
  port: number
  go_version: string
  goroutines: number
  mem: MemView
  counters: CounterSnapshot
}

export interface GatewayStatus {
  connected?: boolean
  session_id?: string
  seq?: number
  heartbeat_ack?: boolean
  since_ms?: number
}

export interface BotProfile {
  id: string
  username: string
  avatar: string
  bot: boolean
  union_openid: string
  union_user_account: string
  share_url: string
  welcome_msg: string
}

export interface DayStat {
  recv: number
  sent: number
  button: number
}

export interface GroupRow {
  id: string
  name: string
  msg: number
}

export interface StatsView {
  groups: number
  peers: number
  total_recv: number
  total_sent: number
  today: DayStat
  top_groups: GroupRow[]
}

export interface Overview {
  runtime: RuntimeView
  counts: { plugins: number; commands: number; jobs: number; templates: number }
  logs: { total: number; errors: number }
  gateway?: GatewayStatus | null
  assets?: { providers: number; enabled: number; whitelist: number }
  profile?: BotProfile | null
  stats?: StatsView
}

export interface PluginField {
  key: string
  label: string
  description?: string
  type: string
  placeholder?: string
  required?: boolean
}

export interface ManagedPlugin {
  id: string
  name: string
  description: string
  fields: PluginField[]
  values: Record<string, unknown>
  commands: string[]
  access: AccessConfig
}

export interface AccessRule {
  mode: 'off' | 'whitelist' | 'blacklist'
  users: string[]
  groups: string[]
}

export interface AccessConfig {
  default: AccessRule
  commands: Record<string, AccessRule>
  disabled?: boolean
}

export interface JobInfo {
  id: string
  plugin_id: string
  kind: 'interval' | 'cron'
  cron?: string
  interval_ms?: number
  immediate: boolean
  paused: boolean
  last_fire: number
  next_fire: number
}

export interface CoreField {
  key: string
  label: string
  desc?: string
  kind: 'text' | 'secret' | 'number' | 'bool' | 'intlist' | 'strlist' | 'multiselect' | 'select' | 'note'
  options?: string[]
  hot: boolean
  restart: boolean
  value: unknown
  set: boolean
}

export interface AssetsProvider {
  name: string
  enabled: boolean
  priority: number
  configured: boolean
  has_secrets: boolean
  schema: Array<{
    key: string
    label: string
    description?: string
    type: string
    required?: boolean
    default?: unknown
  }>
  config: Record<string, unknown>
}

export interface AssetsView {
  whitelist: string[]
  providers: AssetsProvider[]
}
