import { useEffect, useState, type ReactNode } from 'react'
import { NavLink, useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, ThemeSwitch, cn } from '@aurorix/webui'
import {
  Clock,
  ExternalLink,
  Image as ImageIcon,
  LayoutGrid,
  LogOut,
  Package,
  ScrollText,
  SlidersHorizontal,
} from 'lucide-react'
import { get, post } from '../lib/api'
import { fmtDur } from '../lib/format'
import { useConnState } from '../lib/live'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { ManagedPlugin, Overview } from '../lib/types'

/** 实时快照带服务器时钟；REST 回退缺该字段。 */
export type OverviewData = Overview & { now?: number }

const NAV = [
  { to: '/overview', label: '概览', icon: LayoutGrid },
  { to: '/logs', label: '日志', icon: ScrollText },
  { to: '/plugins', label: '插件', icon: Package },
  { to: '/assets', label: '图床', icon: ImageIcon },
  { to: '/jobs', label: '定时任务', icon: Clock },
  { to: '/settings', label: '设置', icon: SlidersHorizontal },
]

/** 每秒重绘的时间锚点，供 uptime 与相对时间共用。 */
export function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  return now
}

/** 概览数据：首屏走 REST，随后由 SSE 每秒覆写同一份缓存。 */
export function useOverview() {
  return useQuery({
    queryKey: qk.overview,
    queryFn: () => get<OverviewData>('/overview'),
    staleTime: 20_000,
  })
}

const CONN_LABEL: Record<string, string> = {
  idle: '未连接',
  connecting: '连接中',
  open: '实时',
  down: '已断开',
}

function ConnectionLamp() {
  const conn = useConnState()
  const live = conn === 'open'
  return (
    <div className={cn('flex items-center gap-2.5 px-3.5 py-2 text-[12.5px]', live ? 'text-ink-soft' : 'text-danger-text')}>
      <span className="relative flex size-2.5 shrink-0">
        <span className={cn('size-2.5 rounded-full', live ? 'bg-ok' : 'bg-danger')} />
        {live && <span className="animate-breathe absolute inset-0 rounded-full bg-ok/60" />}
      </span>
      <span className="truncate">{CONN_LABEL[conn] ?? conn}</span>
    </div>
  )
}

function PluginConsoles() {
  const { data } = useQuery({
    queryKey: qk.plugins,
    queryFn: () => get<ManagedPlugin[]>('/plugins'),
  })
  const consoles = (data ?? []).filter((plugin) => plugin.console)
  if (consoles.length === 0) return null

  return (
    <div className="mt-3 flex flex-col gap-1 border-t border-line pt-3">
      <span className="px-0.5 pb-1 text-[11px] tracking-[0.18em] text-ink-faint">插件控制台</span>
      {consoles.map((plugin) => (
        <a key={plugin.id} href={plugin.console} className="nav-item min-w-0">
          <ExternalLink aria-hidden className="size-[17px] shrink-0" />
          <span className="min-w-0 truncate">{plugin.name || plugin.id}</span>
        </a>
      ))}
    </div>
  )
}

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
}) {
  return (
    <header className="flex flex-wrap items-end justify-between gap-4">
      <div className="min-w-0">
        <h1 className="font-serif text-2xl text-ink md:text-3xl">{title}</h1>
        {description && <p className="mt-1 max-w-prose text-sm text-ink-soft">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </header>
  )
}

export function AppShell({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const { data: overview } = useOverview()
  const now = useNow()

  const gateway = overview?.gateway
  const gatewayOnline = overview
    ? overview.runtime.protocol === 'websocket'
      ? Boolean(gateway?.connected)
      : true
    : null

  const uptime = overview
    ? overview.now
      ? fmtDur(Math.max(0, (now - (overview.now - overview.runtime.uptime_sec * 1000)) / 1000))
      : fmtDur(overview.runtime.uptime_sec)
    : '—'

  const logout = async () => {
    try {
      await post('/logout')
      queryClient.clear()
      navigate('/login', { replace: true })
    } catch (error) {
      toast(error instanceof Error ? error.message : '登出失败', 'err')
    }
  }

  return (
    <div className="flex h-full overflow-hidden bg-background text-ink">
      <aside className="flex w-[244px] shrink-0 flex-col border-r border-line bg-surface px-4 py-5">
        <div className="flex items-center gap-3 px-1.5 pb-6 pt-0.5">
          <span className="grid size-10 shrink-0 place-items-center rounded-[1.25rem] border border-accent/25 bg-accent font-serif text-[19px] font-semibold text-accent-ink">
            A
          </span>
          <div className="min-w-0">
            <b className="block font-serif text-[17px] font-semibold leading-tight tracking-tight">Aurorix</b>
            <small className="text-[11px] tracking-[0.18em] text-ink-faint">ADMIN</small>
          </div>
        </div>

        <nav className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto pr-0.5">
          {NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) => cn('nav-item', isActive && 'bg-accent-soft text-accent-text')}
            >
              <item.icon aria-hidden className="size-[17px] shrink-0" />
              <span>{item.label}</span>
            </NavLink>
          ))}
          <PluginConsoles />
        </nav>

        <div className="mt-3 flex flex-col gap-2 border-t border-line pt-4">
          <ConnectionLamp />
          <div className="flex items-center justify-between rounded-full bg-subtle px-3.5 py-2">
            <span className="text-[11px] text-ink-faint">运行时长</span>
            <b className="font-mono text-[12px] font-medium tabular-nums">{uptime}</b>
          </div>
          <div className="flex items-center justify-between gap-2 px-1.5">
            <ThemeSwitch />
            <Button variant="ghost" size="sm" icon={LogOut} onClick={logout}>
              登出
            </Button>
          </div>
          {gatewayOnline === false && (
            <p className="px-3.5 text-[11px] text-danger-text">网关未连接，机器人当前不响应消息</p>
          )}
        </div>
      </aside>

      <main className="min-w-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-[1240px] flex-col gap-6 px-8 pb-16 pt-8">{children}</div>
      </main>
    </div>
  )
}
