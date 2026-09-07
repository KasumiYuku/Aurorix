import { createEffect, createSignal, For, onCleanup, Show } from 'solid-js'
import { api, LogsView, LogEntry, BotProfile } from '../api'
import { Card, Badge, Icon, IconName, toast, fmtClock, fmtDur, fmtBytes, fmtNum, fmtTime } from '../ui'
import { useLiveOverview, onLog, LiveOverview, patchOverview } from '../live'

export default function OverviewPage(props: { uptime?: () => string }) {
  const overview = useLiveOverview()
  const [recent, setRecent] = createSignal<LogEntry[]>([])

  // 首次进入拉一份日志历史, 之后由实时订阅增量补齐
  api<LogsView>('/api/logs?limit=12')
    .then((v) => setRecent(v.entries))
    .catch(() => {})
  const unlog = onLog((e) => {
    setRecent((cur) => [e, ...cur.filter((x) => x.id !== e.id)].slice(0, 12))
  })
  onCleanup(unlog)

  // 快捷操作: 两步确认, 3 秒内再点执行
  const [action, setAction] = createSignal<'restart' | 'stop' | null>(null)
  const [arming, setArming] = createSignal(false)
  const arm = (kind: 'restart' | 'stop') => {
    setAction(kind)
    setArming(true)
  }
  createEffect(() => {
    if (!arming()) return
    const t = setTimeout(() => setArming(false), 3000)
    onCleanup(() => clearTimeout(t))
  })
  const runAction = async (kind: 'restart' | 'stop') => {
    setAction(null)
    setArming(false)
    toast(kind === 'restart' ? '重启指令已发出…' : '停止指令已发出…', 'info')
    try {
      await api(`/api/system/${kind}`, { method: 'POST', body: '{}' })
    } catch (err) {
      toast((err as Error).message, 'err')
      return
    }
    if (kind === 'restart') {
      // 轮询等待新进程就绪后整页刷新
      const iv = setInterval(async () => {
        try {
          await api('/api/me')
          clearInterval(iv)
          location.reload()
        } catch {
          /* 进程尚未就绪 */
        }
      }, 2000)
      setTimeout(() => clearInterval(iv), 120000)
    } else {
      toast('服务已停止', 'info')
    }
  }

  const ov = () => overview() as LiveOverview

  // 手动刷新机器人档案
  const [refreshing, setRefreshing] = createSignal(false)
  const refreshProfile = async () => {
    setRefreshing(true)
    try {
      const res = await api<{ profile: BotProfile }>('/api/profile/refresh', { method: 'POST', body: '{}' })
      if (res.profile) patchOverview({ profile: res.profile } as Partial<LiveOverview>)
      toast('档案已刷新', 'ok')
    } catch (err) {
      toast(`刷新失败: ${(err as Error).message}`, 'err')
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <Show when={overview()} fallback={<OverviewSkeleton />}>
      <div class="px-rise flex items-center justify-between gap-4">
        <div>
          <h2 class="font-display text-[26px] font-semibold leading-tight tracking-tight text-foreground">概览</h2>
        </div>
        <span class="inline-flex items-center gap-2 rounded-full border border-primary-500/30 bg-primary-50 px-4 py-1.5 text-xs font-medium text-primary-700 dark:bg-primary-400/15 dark:text-primary-300">
          <span class="relative flex h-1.5 w-1.5">
            <span class="px-breathe absolute inset-0 rounded-full bg-primary-500/60" />
            <span class="relative inline-flex h-1.5 w-1.5 rounded-full bg-primary-500" style={{ '--dot-c': 'var(--primary-500)', '--ring-color': 'color-mix(in srgb, var(--primary-500) 20%, transparent)' }} />
          </span>
          实时
        </span>
      </div>

      <Show when={ov().profile} fallback={<ProfileEmpty onRefresh={refreshProfile} busy={refreshing()} />}>
        <ProfileCard profile={ov().profile!} busy={refreshing()} onRefresh={refreshProfile} />
      </Show>

      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Kpi icon="layers" label="注册统计" value={`${fmtNum(ov().counts.plugins)} 插件`} foot={`${fmtNum(ov().counts.commands)} 指令 · ${fmtNum(ov().counts.templates)} 模板`} delay={0} />
        <Kpi icon="users" label="活跃群" value={fmtNum(ov().stats?.groups ?? 0)} foot={`累计接收 ${fmtNum(ov().stats?.total_recv ?? 0)}`} delay={1} />
        <Kpi icon="message" label="私聊用户" value={fmtNum(ov().stats?.peers ?? 0)} foot="QQ 机器人无好友数, 此为私聊用户去重" delay={2} />
        <Kpi icon="trending" label="今日消息" value={fmtNum((ov().stats?.today.recv ?? 0) + (ov().stats?.today.sent ?? 0))} foot={`收 ${fmtNum(ov().stats?.today.recv ?? 0)} · 发 ${fmtNum(ov().stats?.today.sent ?? 0)}`} delay={3} />
      </div>

      <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card title="快捷操作">
          <div class="flex flex-col gap-2.5 px-6 py-4">
            <ActionRow
              icon="refresh"
              label="重启服务"
              desc="配置修改后需重启生效"
              armed={action() === 'restart' && arming()}
              onArm={() => arm('restart')}
              onRun={() => runAction('restart')}
            />
            <ActionRow
              icon="power"
              label="停止服务"
              desc="停止后需手动拉起进程"
              danger
              armed={action() === 'stop' && arming()}
              onArm={() => arm('stop')}
              onRun={() => runAction('stop')}
            />
          </div>
        </Card>

        <Card title="资源占用">
          <div class="px-6 py-3">
            <Stat label="Goroutines" value={fmtNum(ov().runtime.goroutines)} />
            <Stat label="堆内存分配" value={fmtBytes(ov().runtime.mem.heap_alloc)} />
            <Stat label="系统占用" value={fmtBytes(ov().runtime.mem.heap_sys)} />
            <Stat label="GC 次数" value={String(ov().runtime.mem.num_gc)} />
            <Stat label="最近 GC" value={ov().runtime.mem.last_gc ? fmtTime(ov().runtime.mem.last_gc) : '尚未发生'} />
            <Stat label="运行环境" value={`${ov().runtime.go_version} · :${ov().runtime.port}`} />
          </div>
        </Card>
      </div>

      <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card title="注册统计">
          <div class="px-6 py-3">
            <Stat label="插件" value={fmtNum(ov().counts.plugins)} />
            <Stat label="指令" value={fmtNum(ov().counts.commands)} />
            <Stat label="定时任务" value={fmtNum(ov().counts.jobs)} />
            <Stat label="Markdown 模板" value={fmtNum(ov().counts.templates)} />
            <Show when={ov().assets}>
              <Stat label="图床 Provider" value={`${ov().assets!.enabled} / ${ov().assets!.providers} 启用`} />
            </Show>
            <Stat label="累计日志 / 错误" value={`${fmtNum(ov().logs.total)} / ${fmtNum(ov().logs.errors)}`} />
          </div>
        </Card>

        <Card title="网关状态">
          <GatewayCard ov={ov()} />
        </Card>
      </div>

      <Card
        title="最近日志"
        actions={
          <a class="cursor-pointer text-[13px] text-primary-600 hover:underline dark:text-primary-400" href="#/logs" onClick={(e) => { e.preventDefault(); location.hash = '#/logs' }}>
            查看全部 →
          </a>
        }
      >
        <Show when={recent().length} fallback={<div class="px-6 py-10 text-center text-[13px] text-muted-foreground-2">暂无日志</div>}>
          <div class="max-h-[320px] overflow-y-auto">
            <For each={recent()}>
              {(entry) => (
                <div class="px-rise grid grid-cols-[auto_auto_minmax(0,1fr)] items-baseline gap-x-3.5 border-b border-card-divider px-6 py-[9px] font-mono text-[12.3px] tnum last:border-b-0">
                  <span class="whitespace-nowrap text-muted-foreground-2">{fmtClock(entry.time)}</span>
                  <LevelPill level={entry.level} />
                  <span class="truncate text-primary-600 dark:text-primary-400">{entry.scope}</span>
                  <span class="col-span-3 truncate text-foreground" style={{ 'font-family': 'var(--font-sans)' }}>{entry.msg}</span>
                </div>
              )}
            </For>
          </div>
        </Show>
      </Card>
    </Show>
  )
}

const LEVEL_PILL: Record<string, string> = {
  DEBUG: 'text-muted-foreground-2',
  INFO: 'text-muted-foreground',
  WARN: 'bg-amber-50 text-amber-700 dark:bg-amber-400/15 dark:text-amber-300',
  ERROR: 'bg-red-50 text-red-600 dark:bg-red-400/15 dark:text-red-300',
}

function LevelPill(props: { level: string }) {
  return (
    <span class={`inline-block justify-self-start rounded-full px-2 py-px text-[10px] font-semibold uppercase tracking-wider ${LEVEL_PILL[props.level] ?? LEVEL_PILL.INFO}`}>
      {props.level}
    </span>
  )
}

function ActionRow(props: { icon: IconName; label: string; desc: string; danger?: boolean; armed: boolean; onArm: () => void; onRun: () => void }) {
  return (
    <button
      class={`flex w-full cursor-pointer items-center gap-3.5 rounded-xl border px-4 py-3 text-left transition-all duration-300 outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 ${
        props.armed
          ? props.danger
            ? 'border-destructive/60 bg-destructive/10'
            : 'border-primary-500/50 bg-primary-50 dark:bg-primary-400/15'
          : props.danger
            ? 'border-line-2 bg-card hover:border-destructive/40 hover:bg-destructive/5'
            : 'border-line-2 bg-card hover:border-line-4 hover:bg-muted-hover'
      }`}
      onClick={props.armed ? props.onRun : props.onArm}
    >
      <span
        class={`grid h-9 w-9 shrink-0 place-items-center rounded-lg border ${
          props.danger
            ? 'border-destructive/25 bg-destructive/10 text-destructive'
            : 'border-primary-500/20 bg-primary-50 text-primary-600 dark:bg-primary-400/15 dark:text-primary-300'
        }`}
      >
        <Icon name={props.icon} size={15} />
      </span>
      <span class="min-w-0 flex-1">
        <b class={`block text-[13px] font-medium ${props.armed ? (props.danger ? 'text-destructive' : 'text-primary-700 dark:text-primary-300') : 'text-foreground'}`}>
          {props.armed ? `确认${props.label.replace('服务', '')}？再次点击执行` : props.label}
        </b>
        <small class="block truncate text-[11.5px] text-muted-foreground-2">{props.desc}</small>
      </span>
    </button>
  )
}

function RollNum(props: { value: () => string }) {
  let node: HTMLElement | undefined
  createEffect(() => {
    const el = node
    const text = props.value()
    if (!el || el.textContent === text) return
    el.textContent = text
    el.classList.remove('px-tick')
    void el.offsetWidth
    el.classList.add('px-tick')
  })
  return <b ref={node} class="px-tick tnum block truncate font-mono text-[24px] font-semibold leading-7 tracking-tight text-foreground">{props.value()}</b>
}

function Kpi(props: { icon: IconName; label: string; value: string; foot?: string; delay: number }) {
  return (
    <div
      class="px-rise px-blob flex flex-col gap-2 border border-line-2 bg-card p-5 shadow-sm transition-all duration-500 hover:translate-y-0.5 hover:border-line-4 hover:bg-muted-hover"
      style={{ 'animation-delay': `${props.delay * 60}ms` }}
    >
      <div class="flex items-center justify-between gap-2.5">
        <span class="text-[12.5px] font-medium text-muted-foreground">{props.label}</span>
        <span class="px-blob-sm grid h-8 w-8 shrink-0 place-items-center border border-primary-500/20 bg-primary-50 text-primary-600 dark:bg-primary-400/15 dark:text-primary-300">
          <Icon name={props.icon} size={15} />
        </span>
      </div>
      <RollNum value={() => props.value} />
      <Show when={props.foot}>
        <span class="truncate text-xs text-muted-foreground-2">{props.foot}</span>
      </Show>
    </div>
  )
}

function Stat(props: { label: string; value: string }) {
  return (
    <div class="flex items-baseline justify-between gap-3 border-b border-dashed border-card-divider py-2.5 last:border-b-0">
      <span class="text-[13px] text-muted-foreground">{props.label}</span>
      <b class="tnum break-all text-right font-mono text-[12.8px] font-medium text-foreground">{props.value}</b>
    </div>
  )
}

function GatewayCard(props: { ov: LiveOverview }) {
  const gw = props.ov.gateway
  if (props.ov.runtime.protocol === 'webhook') {
    return (
      <div class="flex flex-col items-start gap-2.5 px-6 py-4">
        <Badge tone="ok">监听中</Badge>
        <p class="text-[13px] text-muted-foreground">:{props.ov.runtime.port}/webhook</p>
      </div>
    )
  }
  if (!gw) {
    return (
      <div class="flex flex-col items-start gap-2.5 px-6 py-4">
        <Badge tone="dim">未知</Badge>
        <p class="text-[13px] text-muted-foreground-2">网关状态尚未就绪</p>
      </div>
    )
  }
  return (
    <div class="px-6 py-3">
      <Stat label="连接" value={gw.connected ? '已连接' : '断开'} />
      <Stat label="心跳 ACK" value={gw.heartbeat_ack ? '正常' : '等待中'} />
      <Stat label="Session" value={gw.session_id || '—'} />
      <Stat label="Seq" value={String(gw.seq ?? 0)} />
      <Stat label="连接时长" value={gw.since_ms ? fmtDur(gw.since_ms / 1000) : '—'} />
    </div>
  )
}

function OverviewSkeleton() {
  return (
    <div class="flex flex-col gap-5">
      <div class="flex items-center justify-between">
        <div class="px-skeleton h-[30px] w-32" />
      </div>
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <For each={[0, 1, 2, 3]}>
          {() => <div class="px-blob border border-line-2 bg-card p-5 shadow-sm"><div class="flex flex-col gap-3"><div class="px-skeleton h-4 w-20" /><div class="px-skeleton h-7 w-28" /></div></div>}
        </For>
      </div>
      <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <div class="px-blob border border-line-2 bg-card p-6"><div class="px-skeleton h-40 w-full" /></div>
        <div class="px-blob border border-line-2 bg-card p-6"><div class="flex flex-col gap-3.5"><For each={[0, 1, 2, 3, 4]}>{() => <div class="px-skeleton h-4 w-full" />}</For></div></div>
      </div>
    </div>
  )
}

// ---------- 机器人档案 ----------

function ProfileCard(props: { profile: BotProfile; busy: boolean; onRefresh: () => void }) {
  const p = props.profile
  return (
    <Card
      title="机器人档案"
      actions={
        <button
          class="inline-flex cursor-pointer items-center gap-1.5 rounded-lg border border-line-2 bg-card px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:border-primary-500/40 hover:text-primary-600 disabled:opacity-50 dark:hover:text-primary-400"
          onClick={props.onRefresh}
          disabled={props.busy}
        >
          <Icon name="refresh" size={12} />
          {props.busy ? '刷新中…' : '刷新'}
        </button>
      }
    >
      <div class="flex items-center gap-4 px-6 py-4">
        <Show
          when={p.avatar}
          fallback={
            <span class="px-blob-sm grid h-14 w-14 shrink-0 place-items-center border border-primary-500/20 bg-primary-50 text-primary-600 dark:bg-primary-400/15 dark:text-primary-300">
              <Icon name="users" size={24} />
            </span>
          }
        >
          <img src={p.avatar} alt={p.username} class="h-14 w-14 shrink-0 rounded-full border border-line-2 object-cover" />
        </Show>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2.5">
            <b class="truncate text-[15px] text-foreground">{p.username || '未命名机器人'}</b>
            <Show when={p.bot}><Badge tone="accent">Bot</Badge></Show>
          </div>
          <p class="mt-1 truncate text-[12px] text-muted-foreground-2" title={p.welcome_msg || p.id}>
            {p.welcome_msg || p.id || '—'}
          </p>
        </div>
        <Show when={p.union_user_account || p.union_openid}>
          <div class="hidden shrink-0 flex-col items-end gap-1 md:flex">
            <Show when={p.union_user_account}><span class="truncate font-mono text-[11.5px] text-muted-foreground-2">账号 {p.union_user_account}</span></Show>
            <Show when={p.union_openid}><span class="truncate font-mono text-[11.5px] text-muted-foreground-2">Union {p.union_openid}</span></Show>
          </div>
        </Show>
      </div>
    </Card>
  )
}

function ProfileEmpty(props: { busy: boolean; onRefresh: () => void }) {
  return (
    <Card
      title="机器人档案"
      actions={
        <button
          class="inline-flex cursor-pointer items-center gap-1.5 rounded-lg border border-line-2 bg-card px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:border-primary-500/40 hover:text-primary-600 disabled:opacity-50 dark:hover:text-primary-400"
          onClick={props.onRefresh}
          disabled={props.busy}
        >
          <Icon name="refresh" size={12} />
          {props.busy ? '刷新中…' : '重试'}
        </button>
      }
    >
      <div class="flex items-center gap-3 px-6 py-4 text-[13px] text-muted-foreground-2">
        <Icon name="alert" size={16} />
        档案未获取（凭证错误或网络不可达），概览其余功能不受影响
      </div>
    </Card>
  )
}
