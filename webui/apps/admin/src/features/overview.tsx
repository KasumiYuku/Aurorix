import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  Badge,
  Button,
  Card,
  EmptyState,
  KeyValueList,
  Panel,
  SkeletonRows,
  StatCard,
  Table,
  Td,
  Th,
} from '@aurorix/webui'
import { RefreshCw, Users, Wifi, WifiOff } from 'lucide-react'
import { PageHeader, useNow, useOverview, type OverviewData } from '../components/layout'
import { post } from '../lib/api'
import { fmtBytes, fmtDur, fmtNum, fmtRelative, fmtTime } from '../lib/format'
import { qk } from '../lib/query'
import { toast } from '../lib/toast'
import type { ProfileView } from '../lib/types'

export default function OverviewPage() {
  const { data, isPending, isError, error } = useOverview()
  const queryClient = useQueryClient()
  const now = useNow()
  const [refreshing, setRefreshing] = useState(false)

  if (!data) {
    if (isPending) return <SkeletonRows rows={8} cols={3} />
    return (
      <EmptyState
        title="读取概览失败"
        description={isError && error instanceof Error ? error.message : '请稍后重试'}
      />
    )
  }

  const { runtime, counts, logs, stats, gateway, profile, assets } = data
  const uptime = data.now
    ? fmtDur(Math.max(0, (now - (data.now - runtime.uptime_sec * 1000)) / 1000))
    : fmtDur(runtime.uptime_sec)
  const gatewayOnline = runtime.protocol === 'websocket' ? Boolean(gateway?.connected) : true

  const refreshProfile = async () => {
    setRefreshing(true)
    try {
      const result = await post<ProfileView>('/profile/refresh')
      queryClient.setQueryData(qk.overview, (prev: OverviewData | undefined) =>
        prev ? { ...prev, profile: result.profile } : prev,
      )
      toast('机器人档案已刷新')
    } catch (err) {
      toast(err instanceof Error ? err.message : '刷新档案失败', 'err')
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <>
      <PageHeader
        title="概览"
        description="运行态、消息统计与网关现状，实时快照每秒刷新"
        actions={
          <Badge tone={gatewayOnline ? 'ok' : 'danger'} icon={gatewayOnline ? Wifi : WifiOff}>
            {gatewayOnline ? '网关在线' : '网关离线'}
          </Badge>
        }
      />

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="运行时长" value={uptime} />
        <StatCard label="goroutine" value={fmtNum(runtime.goroutines)} />
        <StatCard label="堆内存" value={fmtBytes(runtime.mem.heap_alloc)} hint={`系统 ${fmtBytes(runtime.mem.heap_sys)}`} />
        <StatCard
          label="错误日志"
          value={fmtNum(logs.errors)}
          hint={`累计 ${fmtNum(logs.total)} 条`}
          tone={logs.errors > 0 ? 'warn' : 'default'}
        />
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="插件" value={fmtNum(counts.plugins)} />
        <StatCard label="指令" value={fmtNum(counts.commands)} />
        <StatCard label="定时任务" value={fmtNum(counts.jobs)} />
        <StatCard label="Markdown 模板" value={fmtNum(counts.templates)} />
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="运行态" description="进程与协议">
          <KeyValueList
            items={[
              { label: '协议模式', value: runtime.protocol || '—' },
              { label: '监听端口', value: runtime.port },
              { label: 'Go 版本', value: runtime.go_version },
              { label: 'GC 次数', value: fmtNum(runtime.mem.num_gc) },
              { label: '上次 GC', value: runtime.mem.last_gc ? fmtRelative(runtime.mem.last_gc, now) : '—' },
              { label: '接收 / 发送', value: `${fmtNum(runtime.counters.recv)} / ${fmtNum(runtime.counters.sent)}` },
              { label: '按钮点击', value: fmtNum(runtime.counters.button) },
            ]}
          />
        </Card>

        <Card
          title="机器人档案"
          description="启动时拉取一次，可手动刷新"
          actions={
            <Button variant="secondary" size="sm" icon={RefreshCw} loading={refreshing} onClick={refreshProfile}>
              刷新
            </Button>
          }
        >
          {profile ? (
            <div className="flex flex-col gap-4">
              <div className="flex items-center gap-4">
                <img
                  src={profile.avatar}
                  alt=""
                  className="size-14 shrink-0 rounded-full border border-line object-cover"
                />
                <div className="min-w-0">
                  <p className="font-serif text-lg">{profile.username || '—'}</p>
                  <p className="truncate font-mono text-xs text-ink-faint">{profile.id}</p>
                </div>
              </div>
              <KeyValueList
                items={[
                  { label: '机器人', value: profile.bot ? '是' : '否' },
                  { label: 'Union OpenID', value: profile.union_openid || '—' },
                  { label: 'Union 账号', value: profile.union_user_account || '—' },
                  {
                    label: '分享链接',
                    value: profile.share_url ? (
                      <a className="text-accent-text underline" href={profile.share_url} target="_blank" rel="noreferrer">
                        打开
                      </a>
                    ) : (
                      '—'
                    ),
                  },
                ]}
              />
              {profile.welcome_msg && <p className="text-xs text-ink-soft">{profile.welcome_msg}</p>}
            </div>
          ) : (
            <EmptyState title="尚未获取到档案" description="网关连通后会自动拉取，也可以点右上角刷新。" />
          )}
        </Card>
      </div>

      <Panel title="网关" description="WebSocket 模式下的连接现状">
        {gateway ? (
          <KeyValueList
            items={[
              { label: '连接状态', value: gateway.connected ? '已连接' : '未连接' },
              { label: '会话 ID', value: gateway.session_id || '—' },
              { label: '序列号', value: gateway.seq === undefined ? '—' : fmtNum(gateway.seq) },
              { label: '心跳 ACK', value: gateway.heartbeat_ack ? '正常' : '待确认' },
              { label: '连接起始', value: gateway.since_ms ? fmtTime(gateway.since_ms) : '—' },
            ]}
          />
        ) : (
          <p className="text-sm text-ink-soft">当前为 webhook 模式，无网关会话。</p>
        )}
      </Panel>

      <Panel title="消息统计" description="进程启动以来的累计量">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard label="群" value={fmtNum(stats.groups)} />
          <StatCard label="好友" value={fmtNum(stats.peers)} />
          <StatCard label="累计接收" value={fmtNum(stats.total_recv)} />
          <StatCard label="累计发送" value={fmtNum(stats.total_sent)} />
        </div>
        <div className="mt-3 grid gap-3 sm:grid-cols-3">
          <StatCard label="今日接收" value={fmtNum(stats.today.recv)} />
          <StatCard label="今日发送" value={fmtNum(stats.today.sent)} />
          <StatCard label="今日按钮" value={fmtNum(stats.today.button)} />
        </div>
        {stats.top_groups.length > 0 && (
          <div className="mt-5">
            <Table>
              <thead>
                <tr>
                  <Th>活跃群</Th>
                  <Th>消息数</Th>
                </tr>
              </thead>
              <tbody>
                {stats.top_groups.map((group) => (
                  <tr key={group.id}>
                    <Td>
                      <span className="flex items-center gap-2">
                        <Users aria-hidden className="size-3.5 shrink-0 text-ink-faint" />
                        <span className="truncate">{group.name || group.id}</span>
                      </span>
                    </Td>
                    <Td className="font-mono tabular-nums">{fmtNum(group.msg)}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
        )}
      </Panel>

      {assets && (
        <Panel title="图床" description="已启用的上传通道">
          <div className="grid gap-3 sm:grid-cols-3">
            <StatCard label="Provider 总数" value={fmtNum(assets.providers)} />
            <StatCard label="已启用" value={fmtNum(assets.enabled)} />
            <StatCard label="直通白名单" value={fmtNum(assets.whitelist)} />
          </div>
        </Panel>
      )}
    </>
  )
}
