// 管理台接口契约：与 lib/admin 的 JSON 字段一一对应。

export interface MeView {
  ok: boolean
  admin: boolean
}

export interface LogEntry {
  id: number
  time: number
  level: 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' | string
  scope: string
  msg: string
}

export interface LogsView {
  entries: LogEntry[]
  scopes: string[]
  total: number
  errors: number
}

export interface LogFilter {
  limit?: number
  minLevel?: string
  scope?: string
  text?: string
}

export interface Counters {
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
  counters: Counters
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

export interface CountsView {
  plugins: number
  commands: number
  jobs: number
  templates: number
}

export interface LogCountsView {
  total: number
  errors: number
}

export interface AssetsSummary {
  providers: number
  enabled: number
  whitelist: number
}

export interface Overview {
  runtime: RuntimeView
  counts: CountsView
  logs: LogCountsView
  stats: StatsView
  gateway?: GatewayStatus | null
  profile?: BotProfile | null
  assets?: AssetsSummary
  control?: ControlView
}

/** 进程控制: supervised 为假时面板不提供重启(会与外部守护抢拉起实例)。 */
export interface ControlView {
  supervised: boolean
}

/** 实时快照在 Overview 上多带一个服务器时钟毫秒，供前端每秒重锚 uptime。 */
export type LiveOverview = Overview & { now: number }

export interface PluginField {
  key: string
  label: string
  description?: string
  type: string
  placeholder?: string
  required?: boolean
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

export interface ManagedPlugin {
  id: string
  name: string
  description: string
  fields: PluginField[]
  values: Record<string, unknown>
  commands: string[]
  access: AccessConfig
  console?: string
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
  kind: 'text' | 'secret' | 'number' | 'bool' | 'intlist' | 'strlist' | 'textlist' | 'multiselect' | 'select' | 'note'
  options?: string[]
  placeholder?: string
  hot: boolean
  restart: boolean
  value: unknown
  set: boolean
}

export interface ConfigView {
  fields: CoreField[]
}

export interface ConfigSaveResult {
  ok: boolean
  restart_needed: boolean
  restart_fields?: string[]
  hot_fields?: string[]
}

export interface AssetsConfigField {
  key: string
  label: string
  description?: string
  type: string
  required?: boolean
  default?: unknown
}

export interface AssetsProvider {
  name: string
  enabled: boolean
  priority: number
  configured: boolean
  has_secrets: boolean
  schema: AssetsConfigField[]
  config: Record<string, unknown>
}

export interface AssetsView {
  whitelist: string[]
  providers: AssetsProvider[]
}

export interface OkResult {
  ok: boolean
}

export interface NoticeResult {
  ok: boolean
  notice?: string
}

export interface ProfileView {
  profile: BotProfile | null
}
