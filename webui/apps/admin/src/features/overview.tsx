import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  AsyncState,
  Badge,
  Button,
  KeyValueList,
  KpiStrip,
  OpsPage,
  OpsSection,
  ResourceTable,
  type Column,
} from '@aurorix/webui'
import { RefreshCw, Users, Wifi, WifiOff } from 'lucide-react'
import { useNow, useOverview, type OverviewData } from '../components/layout'
import { post } from '../lib/api'
import { fmtBytes, fmtDur, fmtNum, fmtTime } from '../lib/format'
import { qk } from '../lib/query'
import { sceneShort } from '../lib/scene'
import { toast } from '../lib/toast'
import type { ConversationRow, ProfileView } from '../lib/types'

function uptimeText(data: OverviewData, now: number): string {
  if (!data.now) return fmtDur(data.runtime.uptime_sec)
  return fmtDur(Math.max(0, (now - (data.now - data.runtime.uptime_sec * 1000)) / 1000))
}

export default function OverviewPage() {
  const { data, isPending, isError, error, isFetching, refetch } = useOverview()
  const queryClient = useQueryClient()
  const now = useNow()
  const [refreshing, setRefreshing] = useState(false)

  const runtime = data?.runtime
  const gateway = data?.gateway ?? null
  const profile = data?.profile ?? null
  const gatewayOnline = runtime ? (runtime.protocol === 'websocket' ? Boolean(gateway?.connected) : true) : null

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
    <OpsPage
      title="概览"
      description="运行态、消息统计与网关现状，实时快照每秒刷新"
      meta={data ? `Go ${data.runtime.go_version} · 采样 ${data.now ? fmtTime(data.now) : '实时通道'}` : '等待实时快照'}
      actions={
        <>
          {gatewayOnline !== null && (
            <Badge tone={gatewayOnline ? 'ok' : 'danger'} icon={gatewayOnline ? Wifi : WifiOff}>
              {gatewayOnline ? '网关在线' : '网关离线'}
            </Badge>
          )}
          {profile && (
            <span
              className="flex items-center gap-2"
              title={`Union ${profile.union_openid || '—'}${profile.welcome_msg ? ` · ${profile.welcome_msg}` : ''}`}
            >
              <img src={profile.avatar} alt="" className="size-7 shrink-0 rounded-full border border-line object-cover" />
              <span className="min-w-0">
                <span className="block max-w-32 truncate text-sm text-ink">{profile.username || '—'}</span>
                <span className="block max-w-32 truncate font-mono text-xs text-ink-faint">{profile.id}</span>
              </span>
            </span>
          )}
          <Button variant="secondary" size="sm" icon={RefreshCw} loading={refreshing} onClick={refreshProfile}>
            刷新档案
          </Button>
          <Button variant="ghost" size="sm" icon={RefreshCw} loading={isFetching} onClick={() => void refetch()}>
            刷新
          </Button>
        </>
      }
      kpis={
        data
          ? [
              { label: '运行时长', value: uptimeText(data, now), hint: `${runtime?.protocol || '—'} · 端口 ${runtime?.port ?? '—'}` },
              {
                label: '累计收发',
                value: `${fmtNum(runtime?.counters.recv)} / ${fmtNum(runtime?.counters.sent)}`,
                hint: `今日 ${fmtNum(data.stats.today.recv)} / ${fmtNum(data.stats.today.sent)}`,
              },
              {
                label: '错误日志',
                value: fmtNum(data.logs.errors),
                tone: data.logs.errors > 0 ? 'warn' : 'ok',
                hint: `累计 ${fmtNum(data.logs.total)} 条`,
              },
              {
                label: 'goroutine',
                value: fmtNum(runtime?.goroutines),
                hint: `堆 ${fmtBytes(runtime?.mem.heap_alloc)} · GC ${fmtNum(runtime?.mem.num_gc)} 次`,
              },
              {
                label: '插件',
                value: fmtNum(data.counts.plugins),
                hint: `指令 ${fmtNum(data.counts.commands)} · 任务 ${fmtNum(data.counts.jobs)} · 模板 ${fmtNum(data.counts.templates)}`,
              },
            ]
          : undefined
      }
    >
      <AsyncState
        pending={isPending && !data}
        error={!data && isError ? error : undefined}
        isEmpty={!data && !isPending && !isError}
        empty={{ title: '读取概览失败', description: '请稍后重试' }}
        skeletonRows={8}
        skeletonCols={3}
        onRetry={() => void refetch()}
      >
        {data && <OverviewBody data={data} />}
      </AsyncState>
    </OpsPage>
  )
}

function OverviewBody({ data }: { data: OverviewData }) {
  const { runtime, stats, gateway, assets } = data

  const conversationColumns: Column<ConversationRow>[] = [
    {
      key: 'name',
      label: '活跃会话',
      primary: true,
      render: (conv) => (
        <span className="flex items-center gap-2">
          <span className="rounded bg-sunk px-1 py-0.5 text-xs text-ink-faint">{sceneShort(conv.scene)}</span>
          <span className="truncate text-sm text-ink">{conv.name || conv.id}</span>
        </span>
      ),
    },
    {
      key: 'msg',
      label: '消息数',
      width: '7rem',
      align: 'right',
      render: (conv) => <span className="font-mono text-xs tabular-nums">{fmtNum(conv.msg)}</span>,
    },
  ]

  return (
    <>
      <OpsSection
        title="网关"
        hint={runtime.protocol === 'websocket' ? 'WebSocket 模式下的连接现状' : 'webhook 模式，无网关会话'}
        actions={
          gateway ? (
            <Badge tone={gateway.connected ? 'ok' : 'danger'} icon={gateway.connected ? Wifi : WifiOff}>
              {gateway.connected ? '已连接' : '未连接'}
            </Badge>
          ) : undefined
        }
      >
        <KeyValueList
          items={[
            { label: '协议模式', value: runtime.protocol || '—' },
            { label: '监听端口', value: runtime.port },
            { label: '会话 ID', value: gateway?.session_id || '—', mono: true },
            { label: '序列号', value: gateway?.seq === undefined ? '—' : fmtNum(gateway.seq), mono: true },
            { label: '心跳 ACK', value: gateway ? (gateway.heartbeat_ack ? '正常' : '待确认') : '—' },
            { label: '连接起始', value: gateway?.since_ms ? fmtTime(gateway.since_ms) : '—', mono: true },
          ]}
        />
      </OpsSection>

      <OpsSection title="消息统计" hint="进程启动以来的累计量">
        <KpiStrip
          items={[
            { label: '群', value: fmtNum(stats.groups) },
            { label: '好友', value: fmtNum(stats.friends) },
            { label: '频道', value: fmtNum(stats.channels) },
            { label: '累计接收', value: fmtNum(stats.total_recv) },
            { label: '累计发送', value: fmtNum(stats.total_sent) },
            {
              label: '今日收发',
              value: `${fmtNum(stats.today.recv)} / ${fmtNum(stats.today.sent)}`,
              hint: `按钮 ${fmtNum(stats.today.button)}`,
            },
          ]}
        />
        <ResourceTable
          columns={conversationColumns}
          rows={stats.top_conversations}
          rowKey={(conv) => `${conv.scene}:${conv.id}`}
          empty={{ icon: Users, title: '还没有会话记录', description: '有消息往来后，这里按消息数列出最活跃的会话。' }}
        />
      </OpsSection>

      <OpsSection title="图床" hint={assets ? '上传通道与直通白名单' : undefined}>
        {assets ? (
          <KpiStrip
            items={[
              { label: 'Provider 总数', value: fmtNum(assets.providers) },
              { label: '已启用', value: fmtNum(assets.enabled), tone: assets.enabled > 0 ? 'ok' : 'warn' },
              { label: '直通白名单', value: fmtNum(assets.whitelist) },
            ]}
          />
        ) : (
          <p className="text-sm text-ink-soft">服务端未上报图床信息。</p>
        )}
      </OpsSection>
    </>
  )
}
