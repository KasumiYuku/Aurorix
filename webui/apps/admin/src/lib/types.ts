
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
  group?: string
  user?: string
  trace?: string
  fields?: Record<string, string>
}

export interface LogSummary {
  levels: Record<string, number>
  scopes: Record<string, number>
}

export interface LogsView {
  entries: LogEntry[]
  scopes: string[]
  total: number
  errors: number
  summary?: LogSummary
}

export interface LogFilter {
  limit?: number
  minLevel?: string
  scope?: string
  text?: string
  group?: string
  user?: string
  trace?: string
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

export type SceneScope = 'group' | 'c2c' | 'channel' | 'dm'

export interface ConversationRow {
  scene: SceneScope
  id: string
  name: string
  msg: number
}

export interface StatsView {
  groups: number
  peers: number
  friends: number
  channels: number
  total_recv: number
  total_sent: number
  today: DayStat
  top_conversations: ConversationRow[]
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

export interface ControlView {
  supervised: boolean
}

export type LiveOverview = Overview & { now: number }

export interface PluginShowIf {
  key?: string
  values?: string[]
  any?: PluginShowIf[]
}

export interface PluginField {
  key: string
  label: string
  description?: string
  type: string
  placeholder?: string
  required?: boolean
  options?: string[]
  show_if?: PluginShowIf
}

export interface AccessRule {
  mode: 'off' | 'whitelist' | 'blacklist'
  users: string[]
  groups: string[]
  role?: '' | 'member' | 'admin' | 'owner'
  maintenance?: { enabled: boolean; message?: string }
  deny?: AccessDeny
}

export interface AccessConfig {
  default: AccessRule
  commands: Record<string, AccessRule>
  disabled?: boolean
  maintenance?: { enabled: boolean; message?: string }
  deny?: AccessDeny
}

export interface AccessDeny {
  reply?: boolean
  message?: string
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
  kind: 'text' | 'secret' | 'number' | 'bool' | 'intlist' | 'strlist' | 'textlist' | 'textarea' | 'multiselect' | 'select' | 'note'
  options?: string[]
  placeholder?: string
  group: string
  unit?: string
  hot: boolean
  restart: boolean
  dedicated?: boolean
  value: unknown
  set: boolean
}

export interface ConfigGroup {
  id: string
  label: string
  desc?: string
}

export interface ConfigView {
  fields: CoreField[]
  groups?: ConfigGroup[]
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

export type PanelScope = SceneScope

export type PanelItemType = 'command' | 'link'

export interface PanelItem {
  name: string
  desc: string
  type: PanelItemType
  only_admin?: boolean
  link?: string
}

export interface Panel {
  items: PanelItem[]
  remark?: string
  version?: number
}

export interface PanelRecord {
  panel_id: string
  scope: PanelScope
  target_type: string
  panel: Panel
  created_at?: string
  updated_at?: string
  version: number
}

export interface MenuScopeInfo {
  scope: PanelScope
  label: string
}

export interface MenuLimits {
  name_width: number
  desc_width: number
  remark_len: number
  max_items: number
  max_per_bot: number
  item_types: PanelItemType[]
  target_type: string
  display_note: string
}

export interface MenuMeta {
  scopes: MenuScopeInfo[]
  limits: MenuLimits
}

export interface MenuScopePanels extends MenuScopeInfo {
  panels: PanelRecord[]
}

export interface MenuPanels {
  scopes: MenuScopePanels[]
  total: number
  max_per_bot: number
}

export interface MenuSnapshot {
  file: string
  panel_id: string
  scope: PanelScope
  target_type: string
  panel: Panel
  version: number
  action: string
  saved_at: string
}

export interface MenuSnapshots {
  snapshots: MenuSnapshot[]
  dir: string
}

export interface PanelWriteResult {
  ok: boolean
  version?: number
  panel_id?: string
  snapshot?: string
  snapshot_error?: string
}

export interface PanelRestoreResult {
  ok: boolean
  panel_id: string
  scope: PanelScope
  mode: 'overwrite' | 'recreated'
}

export interface PanelSyncTarget {
  scope: PanelScope
  panel_id?: string
}

export interface PanelSyncItemResult {
  scope: PanelScope
  panel_id?: string
  mode?: 'created' | 'overwritten'
  snapshot?: string
  error?: string
}

export interface PanelSyncResult {
  ok: boolean
  from: string
  results: PanelSyncItemResult[]
}

export interface InboxConversation {
  id: string
  kind: SceneScope
  name: string
  last_at: string
  last_preview: string
}

export interface InboxMessage {
  id: number
  conv_id: string
  kind: SceneScope
  direction: 'in' | 'out'
  sender_id?: string
  sender_name?: string
  content: string
  platform_id?: string
  created_at: string
}

export interface InboxMessagesView {
  messages: InboxMessage[]
  has_more: boolean
}

export interface BanEntry {
  openid: string
  message?: string
  note?: string
  at?: string
}

export interface BanList {
  entries: BanEntry[]
  default: string
  limit: number
}
