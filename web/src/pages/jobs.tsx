import { createSignal, For, Show } from 'solid-js'
import { api, JobInfo } from '../api'
import { Badge, Card, Empty, SkeletonRows, Switch, fmtTime } from '../ui'
import { useLiveJobs } from '../live'

export default function JobsPage() {
  const jobs = useLiveJobs()
  const [busyId, setBusyId] = createSignal('')

  const toggle = async (job: JobInfo) => {
    setBusyId(job.id)
    try {
      await api(`/api/jobs/${encodeURIComponent(job.id)}/pause`, { method: 'POST', body: JSON.stringify({ paused: !job.paused }) })
    } catch {
    } finally {
      setBusyId('')
    }
  }

  const scheduleText = (job: JobInfo) => {
    if (job.kind === 'interval') {
      const ms = job.interval_ms ?? 0
      if (ms % 3600000 === 0) return `每 ${ms / 3600000} 小时`
      if (ms % 60000 === 0) return `每 ${ms / 60000} 分钟`
      return `每 ${ms / 1000} 秒`
    }
    return job.cron ?? ''
  }

  return (
    <Show when={jobs()} fallback={<SkeletonRows rows={5} />}>
      <div class="px-rise">
        <h2 class="mb-6 font-display text-[26px] font-semibold leading-tight tracking-tight text-foreground">定时任务</h2>
        <Card>
          <div class="overflow-x-auto">
            <table class="w-full text-[13px]">
              <thead>
                <tr class="text-left text-[11.5px] font-medium text-muted-foreground-2">
                  <th class="px-5 py-3">任务</th>
                  <th class="px-5 py-3">插件</th>
                  <th class="px-5 py-3">调度</th>
                  <th class="px-5 py-3">上次触发</th>
                  <th class="px-5 py-3">下次触发</th>
                  <th class="px-5 py-3">状态</th>
                </tr>
              </thead>
              <tbody>
                <For each={jobs()} fallback={<tr><td colspan={6}><Empty text="暂无已注册的定时任务" /></td></tr>}>
                  {(job) => (
                    <tr class="border-t border-card-divider transition-colors duration-300 hover:bg-muted-hover">
                      <td class="px-5 py-3">
                        <b class="font-medium text-foreground">{job.id}</b>
                        {job.immediate && <span class="text-muted-foreground-2"> 启动即触发</span>}
                      </td>
                      <td class="px-5 py-3 font-mono text-xs text-muted-foreground">{job.plugin_id || '—'}</td>
                      <td class="px-5 py-3 font-mono text-xs text-muted-foreground">{scheduleText(job)}</td>
                      <td class="px-5 py-3 text-muted-foreground-2">{job.last_fire ? fmtTime(job.last_fire) : '从未触发'}</td>
                      <td class="px-5 py-3 text-muted-foreground">{job.kind === 'cron' && job.next_fire ? fmtTime(job.next_fire) : <span class="text-muted-foreground-2">—</span>}</td>
                      <td class="px-5 py-3">
                        <span class="flex items-center gap-2.5">
                          {job.paused ? <Badge tone="warn">已暂停</Badge> : <Badge tone="ok">运行中</Badge>}
                          <Switch checked={!job.paused} disabled={busyId() === job.id} onChange={() => toggle(job)} />
                        </span>
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </Card>
      </div>
    </Show>
  )
}
