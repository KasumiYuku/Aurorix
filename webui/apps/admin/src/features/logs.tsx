import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Badge, Button, EmptyState, Input, Panel, Select, SkeletonRows, Switch, Table, Td, Th } from '@aurorix/webui'
import { Download } from 'lucide-react'
import { PageHeader, useNow } from '../components/layout'
import { get, withQuery } from '../lib/api'
import { fmtClock } from '../lib/format'
import { subscribeLogs } from '../lib/live'
import { qk } from '../lib/query'
import type { LogEntry, LogsView } from '../lib/types'

const LEVELS = ['debug', 'info', 'warn', 'error']
const LEVEL_RANK: Record<string, number> = { debug: 0, info: 1, warn: 2, error: 3 }
const LIMIT = 500
const RENDER_CAP = 800

const LEVEL_TONE: Record<string, 'ok' | 'warn' | 'danger' | 'neutral'> = {
  DEBUG: 'neutral',
  INFO: 'ok',
  WARN: 'warn',
  ERROR: 'danger',
}

function matchEntry(entry: LogEntry, minLevel: string, scope: string, text: string): boolean {
  if (minLevel !== '' && (LEVEL_RANK[entry.level.toLowerCase()] ?? 0) < (LEVEL_RANK[minLevel] ?? 0)) return false
  if (scope !== '' && entry.scope !== scope) return false
  if (text !== '' && !entry.msg.toLowerCase().includes(text.toLowerCase())) return false
  return true
}

export default function LogsPage() {
  const now = useNow()
  const [minLevel, setMinLevel] = useState('')
  const [scope, setScope] = useState('')
  const [text, setText] = useState('')
  const [live, setLive] = useState(true)
  const [liveEntries, setLiveEntries] = useState<LogEntry[]>([])

  const [queryText, setQueryText] = useState('')
  useEffect(() => {
    const timer = window.setTimeout(() => setQueryText(text), 300)
    return () => window.clearTimeout(timer)
  }, [text])

  const { data, isPending, isError, error } = useQuery({
    queryKey: qk.logs({ limit: LIMIT, minLevel, scope, text: queryText }),
    queryFn: () => get<LogsView>(withQuery('/logs', { limit: LIMIT, min_level: minLevel, scope, q: queryText })),
  })

  useEffect(() => {
    if (!live) {
      setLiveEntries([])
      return
    }
    return subscribeLogs((entry) => {
      setLiveEntries((prev) => (prev.length >= RENDER_CAP ? prev : [entry, ...prev]))
    })
  }, [live])

  const entries = useMemo(() => {
    const base = (data?.entries ?? []).filter((entry) => matchEntry(entry, minLevel, scope, queryText))
    if (liveEntries.length === 0) return base.slice(0, RENDER_CAP)
    const seen = new Set(base.map((entry) => entry.id))
    const extra = liveEntries.filter(
      (entry) => !seen.has(entry.id) && matchEntry(entry, minLevel, scope, queryText),
    )
    return [...extra, ...base].slice(0, RENDER_CAP)
  }, [data, liveEntries, minLevel, scope, queryText])

  const scopes = data?.scopes ?? []

  const exportLogs = () => {
    const text = entries
      .map((entry) => `${new Date(entry.time).toISOString()} ${entry.level.padEnd(5)} ${entry.scope} ${entry.msg}`)
      .join('\n')
    const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }))
    const link = document.createElement('a')
    link.href = url
    link.download = `aurorix-logs-${Date.now()}.txt`
    link.click()
    URL.revokeObjectURL(url)
  }

  return (
    <>
      <PageHeader
        title="日志"
        description={`环形缓冲内共 ${data?.total ?? 0} 条，其中错误 ${data?.errors ?? 0} 条`}
        actions={
          <Button variant="secondary" icon={Download} onClick={exportLogs} disabled={entries.length === 0}>
            导出
          </Button>
        }
      />

      <Panel title="筛选">
        <div className="grid gap-3 md:grid-cols-[160px_200px_1fr_auto]">
          <Select
            value={minLevel}
            onChange={(e) => setMinLevel(e.target.value)}
            options={[{ value: '', label: '全部级别' }, ...LEVELS.map((level) => ({ value: level, label: level }))]}
          />
          <Select
            value={scope}
            onChange={(e) => setScope(e.target.value)}
            options={[{ value: '', label: '全部来源' }, ...scopes.map((item) => ({ value: item, label: item }))]}
          />
          <Input placeholder="搜索内容" value={text} onChange={(e) => setText(e.target.value)} />
          <Switch checked={live} onChange={setLive} label="实时推送" />
        </div>
      </Panel>

      {isPending && !data ? (
        <SkeletonRows rows={10} cols={4} />
      ) : isError && !data ? (
        <EmptyState title="读取日志失败" description={error instanceof Error ? error.message : '请稍后重试'} />
      ) : entries.length === 0 ? (
        <EmptyState title="没有匹配的日志" description="换个级别、来源或关键字试试。" />
      ) : (
        <Table>
          <thead>
            <tr>
              <Th>时间</Th>
              <Th>级别</Th>
              <Th>来源</Th>
              <Th>内容</Th>
            </tr>
          </thead>
          <tbody>
            {entries.map((entry) => (
              <tr key={entry.id}>
                <Td className="whitespace-nowrap font-mono text-xs text-ink-faint">
                  {entry.time ? fmtClock(entry.time) : fmtClock(now)}
                </Td>
                <Td>
                  <Badge tone={LEVEL_TONE[entry.level] ?? 'neutral'}>{entry.level}</Badge>
                </Td>
                <Td className="whitespace-nowrap font-mono text-xs text-ink-soft">{entry.scope}</Td>
                <Td className="whitespace-pre-wrap break-words">{entry.msg}</Td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
    </>
  )
}
