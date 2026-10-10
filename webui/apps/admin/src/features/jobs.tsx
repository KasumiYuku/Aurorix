import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Badge, Button, EmptyState, Panel, SkeletonRows, Table, Td, Th, Tooltip } from '@aurorix/webui'
import { Clock, Pause, Play } from 'lucide-react'
import { PageHeader, useNow } from '../components/layout'
import { get, post } from '../lib/api'
import { fmtDur, fmtRelative } from '../lib/format'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { JobInfo, OkResult } from '../lib/types'

function periodText(job: JobInfo): string {
  if (job.kind === 'cron') return job.cron ?? '—'
  if (job.interval_ms) return `每 ${fmtDur(job.interval_ms / 1000)}`
  return '—'
}

export default function JobsPage() {
  const now = useNow()
  const [pending, setPending] = useState('')
  const { data, isPending, isError, error } = useQuery({
    queryKey: qk.jobs,
    queryFn: () => get<JobInfo[]>('/jobs'),
    staleTime: 20_000,
  })

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

  return (
    <>
      <PageHeader title="定时任务" description="任务列表随变更实时推送，暂停仅影响本进程" />

      {isPending && !data ? (
        <SkeletonRows rows={6} cols={5} />
      ) : isError && !data ? (
        <EmptyState title="读取任务失败" description={error instanceof Error ? error.message : '请稍后重试'} />
      ) : !data || data.length === 0 ? (
        <EmptyState title="当前没有定时任务" description="插件注册 schedule.Job 后会出现在这里。" />
      ) : (
        <Panel>
          <Table>
            <thead>
              <tr>
                <Th>任务</Th>
                <Th>插件</Th>
                <Th>类型</Th>
                <Th>周期</Th>
                <Th>上次触发</Th>
                <Th>下次触发</Th>
                <Th>操作</Th>
              </tr>
            </thead>
            <tbody>
              {data.map((job) => (
                <tr key={job.id}>
                  <Td>
                    <span className="flex items-center gap-2">
                      <Clock aria-hidden className="size-3.5 shrink-0 text-ink-faint" />
                      <span className="font-mono text-xs">{job.id}</span>
                      {job.paused && <Badge tone="warn">已暂停</Badge>}
                      {job.immediate && <Badge>启动即跑</Badge>}
                    </span>
                  </Td>
                  <Td className="font-mono text-xs text-ink-soft">{job.plugin_id || '—'}</Td>
                  <Td>{job.kind === 'cron' ? 'Cron' : '固定间隔'}</Td>
                  <Td className="font-mono text-xs">{periodText(job)}</Td>
                  <Td className="whitespace-nowrap text-xs text-ink-soft">
                    {job.last_fire ? fmtRelative(job.last_fire, now) : '未触发'}
                  </Td>
                  <Td className="whitespace-nowrap text-xs text-ink-soft">
                    {job.paused || !job.next_fire ? '—' : fmtRelative(job.next_fire, now)}
                  </Td>
                  <Td>
                    <Tooltip label={job.paused ? '恢复任务' : '暂停任务'}>
                      <Button
                        variant="secondary"
                        size="sm"
                        icon={job.paused ? Play : Pause}
                        loading={pending === job.id}
                        onClick={() => toggle(job)}
                      >
                        {job.paused ? '恢复' : '暂停'}
                      </Button>
                    </Tooltip>
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Panel>
      )}
    </>
  )
}
