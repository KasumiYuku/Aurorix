import { useEffect, useState, type ReactNode } from 'react'
import { NavLink, useLocation, useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, ThemeSwitch, cn } from '@aurorix/webui'
import {
  Clock,
  Command,
  ExternalLink,
  Image as ImageIcon,
  LayoutGrid,
  LogOut,
  MessagesSquare,
  Package,
  ScrollText,
  ShieldOff,
  SlidersHorizontal,
  type LucideIcon,
} from 'lucide-react'
import { get, post } from '../lib/api'
import { fmtDur } from '../lib/format'
import { useConnState } from '../lib/live'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { ManagedPlugin, Overview } from '../lib/types'

export type OverviewData = Overview & { now?: number }

type NavItem = { to: string; label: string; icon: LucideIcon }
type NavGroup = { id: string; label: string; items: NavItem[] }

const NAV_GROUPS: NavGroup[] = [
  {
    id: 'overview',
    label: '概览',
    items: [{ to: '/overview', label: '概览', icon: LayoutGrid }],
  },
  {
    id: 'business',
    label: '业务',
    items: [
      { to: '/messages', label: '消息', icon: MessagesSquare },
      { to: '/menu', label: '指令面板', icon: Command },
      { to: '/plugins', label: '插件', icon: Package },
      { to: '/bans', label: '封禁用户', icon: ShieldOff },
    ],
  },
  {
    id: 'ops',
    label: '运维',
    items: [
      { to: '/logs', label: '日志', icon: ScrollText },
      { to: '/jobs', label: '定时任务', icon: Clock },
      { to: '/assets', label: '图床', icon: ImageIcon },
    ],
  },
  {
    id: 'settings',
    label: '设置',
    items: [{ to: '/settings', label: '设置', icon: SlidersHorizontal }],
  },
]

export function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  return now
}

export function useOverview() {
  return useQuery({
    queryKey: qk.overview,
    queryFn: () => get<OverviewData>('/overview'),
    staleTime: 20_000,
  })
}

export function useUptime(): string {
  const { data: overview } = useOverview()
  const now = useNow()
  if (!overview) return '—'
  if (!overview.now) return fmtDur(overview.runtime.uptime_sec)
  return fmtDur(Math.max(0, (now - (overview.now - overview.runtime.uptime_sec * 1000)) / 1000))
}

export function useGatewayOnline(): boolean | null {
  const { data: overview } = useOverview()
  if (!overview) return null
  if (overview.runtime.protocol !== 'websocket') return true
  return Boolean(overview.gateway?.connected)
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
    <span
      className={cn(
        'flex items-center gap-1.5 text-xs',
        live ? 'text-ink-soft' : 'text-danger-ink',
      )}
      title="管理台实时通道状态"
    >
      <span className="relative flex size-2 shrink-0">
        <span className={cn('size-2 rounded-full', live ? 'bg-ok' : 'bg-danger')} />
        {live && <span className="animate-breathe absolute inset-0 rounded-full bg-ok" />}
      </span>
      {CONN_LABEL[conn] ?? conn}
    </span>
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
    <div className="mt-2 flex flex-col gap-0.5 border-t border-line pt-2">
      <span className="px-2 pb-1 text-xs text-ink-faint">插件控制台</span>
      {consoles.map((plugin) => (
        <a key={plugin.id} href={plugin.console} className="nav-item min-w-0">
          <ExternalLink aria-hidden className="size-4 shrink-0" />
          <span className="min-w-0 truncate">{plugin.name || plugin.id}</span>
        </a>
      ))}
    </div>
  )
}

function NavTree() {
  return (
    <nav className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      {NAV_GROUPS.map((group) => (
        <div key={group.id} className="flex flex-col gap-0.5 pb-2">
          <span className="px-2 pb-1 pt-2 text-xs font-medium text-ink-faint">{group.label}</span>
          {group.items.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) => cn('nav-item', isActive && 'font-medium')}
            >
              <item.icon aria-hidden className="size-4 shrink-0" />
              <span className="truncate">{item.label}</span>
            </NavLink>
          ))}
        </div>
      ))}
      <PluginConsoles />
    </nav>
  )
}

export function PageHeader({
  title,
  description,
  actions,
  meta,
}: {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  meta?: ReactNode
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-3 border-b border-line pb-3">
      <div className="min-w-0">
        <h1 className="text-xl">{title}</h1>
        {description && <p className="mt-1 max-w-[68ch] text-sm text-ink-soft">{description}</p>}
        {meta && <div className="mt-1 font-mono text-xs text-ink-faint">{meta}</div>}
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </header>
  )
}

function useBreadcrumb(): { group: string; page: string } {
  const { pathname } = useLocation()
  for (const group of NAV_GROUPS) {
    for (const item of group.items) {
      if (pathname === item.to || pathname.startsWith(item.to + '/')) {
        return { group: group.label, page: item.label }
      }
    }
  }
  return { group: '概览', page: '概览' }
}

export function AppShell({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const { group, page } = useBreadcrumb()
  const uptime = useUptime()
  const gatewayOnline = useGatewayOnline()

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
    <div className="flex h-full overflow-hidden bg-paper text-ink">
      <aside className="hidden w-60 shrink-0 flex-col border-r border-line bg-paper px-3 py-3 md:flex">
        <div className="flex items-center gap-2 px-2 pb-2">
          <span className="grid size-6 shrink-0 place-items-center rounded-md bg-accent text-xs font-semibold text-white">
            A
          </span>
          <b className="min-w-0 truncate text-sm font-semibold">Aurorix</b>
          <small className="text-xs text-ink-faint">控制台</small>
        </div>
        <NavTree />
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-11 shrink-0 items-center justify-between gap-3 border-b border-line bg-paper px-4">
          <div className="flex min-w-0 items-center gap-1.5 text-sm">
            <span className="text-ink-faint">{group}</span>
            <span aria-hidden className="text-ink-faint">
              /
            </span>
            <span className="truncate font-medium">{page}</span>
          </div>
          <div className="flex shrink-0 items-center gap-3">
            <ConnectionLamp />
            <span className="hidden font-mono text-xs text-ink-faint sm:inline" title="运行时长">
              {uptime}
            </span>
            <ThemeSwitch />
            <Button variant="ghost" size="sm" icon={LogOut} onClick={logout} title="登出">
              <span className="hidden sm:inline">登出</span>
            </Button>
          </div>
        </header>

        {gatewayOnline === false && (
          <p className="shrink-0 border-b border-line bg-sunk px-4 py-1.5 text-xs text-danger-ink">
            网关未连接，机器人当前不响应消息
          </p>
        )}

        <nav className="flex shrink-0 gap-1 overflow-x-auto border-b border-line px-2 py-1.5 md:hidden">
          {NAV_GROUPS.flatMap((g) => g.items).map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) => cn('nav-item shrink-0', isActive && 'font-medium')}
            >
              <item.icon aria-hidden className="size-4 shrink-0" />
              <span>{item.label}</span>
            </NavLink>
          ))}
        </nav>

        <main className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 px-4 py-6 md:px-8">
            {children}
          </div>
        </main>
      </div>
    </div>
  )
}
