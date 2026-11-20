import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Badge, Button, OpsPage, OpsSection, ResourceTable, TimeAgo, Tooltip, type Column } from '@aurorix/webui'
import { Clock, Pause, Play, RefreshCw } from 'lucide-react'
import { get, post } from '../lib/api'
import { fmtDur } from '../lib/format'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { JobInfo, OkResult } from '../lib/types'

function periodText(job: JobInfo): string {
  if (job.kind === 'cron') return job.cron ?? '—'
  if (job.interval_ms) return `每 ${fmtDur(job.interval_ms / 1000)}`
  return '—'
}

export default function JobsPage() {
  const [pending, setPending] = useState('')
  const { data, isPending, isError, error, isFetching, refetch } = useQuery({
    queryKey: qk.jobs,
    queryFn: () => get<JobInfo[]>('/jobs'),
    staleTime: 20_000,
  })
  const rows = data ?? []
  const pausedCount = rows.filter((job) => job.paused).length

  const toggle = async (job: JobInfo) => {
    setPending(job.id)
    try {
      await post<OkResult>(`/jobs/${job.id}/pause`, { paused: !job.paused })
      await queryClient.invalidateQueries({ queryKey: qk.jobs })
      toast(job.paused ? '任务已恢复' : '任务已暂停')
    } catch (err) {
      toast(err instanceof Error ? err.message : '操作失败', 'err')
    } finally {
      setPending('')
    }
  }

  const columns: Column<JobInfo>[] = [
    {
      key: 'id',
      label: '任务',
      primary: true,
      render: (job) => (
        <span className="flex items-center gap-2">
          <Clock aria-hidden className="size-3.5 shrink-0 text-ink-faint" />
          <span className="truncate font-mono text-xs text-ink">{job.id}</span>
          {job.immediate && (
            <Badge tone="neutral" icon={Play}>
              启动即跑
            </Badge>
          )}
        </span>
      ),
    },
    {
      key: 'state',
      label: '状态',
      width: '7rem',
      render: (job) =>
        job.paused ? (
          <Badge tone="warn" icon={Pause}>
            已暂停
          </Badge>
        ) : (
          <Badge tone="ok" icon={Play}>
            运行中
          </Badge>
        ),
    },
    {
      key: 'plugin',
      label: '插件',
      width: '9rem',
      hideOnMobile: true,
      render: (job) => <span className="truncate font-mono text-xs text-ink-soft">{job.plugin_id || '—'}</span>,
    },
    {
      key: 'kind',
      label: '类型',
      width: '6rem',
      render: (job) => <span className="text-sm text-ink-soft">{job.kind === 'cron' ? 'Cron' : '固定间隔'}</span>,
    },
    {
      key: 'period',
      label: '周期',
      width: '8rem',
      render: (job) => <span className="truncate font-mono text-xs text-ink-soft">{periodText(job)}</span>,
    },
    {
      key: 'last_fire',
      label: '上次触发',
      width: '8rem',
      hideOnMobile: true,
      render: (job) => <TimeAgo value={job.last_fire || undefined} />,
    },
    {
      key: 'next_fire',
      label: '下次触发',
      width: '8rem',
      hideOnMobile: true,
      render: (job) => <TimeAgo value={job.paused ? undefined : job.next_fire || undefined} />,
    },
    {
      key: 'actions',
      label: '',
      width: '5.5rem',
      align: 'right',
      render: (job) => (
        <Tooltip label={job.paused ? '恢复任务' : '暂停任务'}>
          <Button
            variant="secondary"
            size="sm"
            icon={job.paused ? Play : Pause}
            loading={pending === job.id}
            onClick={() => void toggle(job)}
          >
            {job.paused ? '恢复' : '暂停'}
          </Button>
        </Tooltip>
      ),
    },
  ]

  return (
    <OpsPage
      title="定时任务"
      description="任务列表随变更实时推送，暂停仅影响本进程"
      meta={
        data ? `载入 ${rows.length} 个任务 · 其中 ${pausedCount} 个已暂停` : isPending ? '加载中' : '读取失败'
      }
      actions={
        <Button variant="secondary" size="sm" icon={RefreshCw} loading={isFetching} onClick={() => void refetch()}>
          刷新
        </Button>
      }
    >
      <OpsSection title="任务列表" hint={pausedCount > 0 ? `${pausedCount} 个已暂停` : undefined}>
        <ResourceTable
          columns={columns}
          rows={data}
          rowKey={(job) => job.id}
          loading={isPending && !data}
          error={isError && !data ? error : undefined}
          empty={{ icon: Clock, title: '当前没有定时任务', description: '插件注册 schedule.Job 后会出现在这里。' }}
        />
      </OpsSection>
    </OpsPage>
  )
}
