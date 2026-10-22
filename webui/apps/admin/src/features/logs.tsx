import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import {
  Badge,
  Button,
  Drawer,
  EmptyState,
  Input,
  Panel,
  Select,
  SkeletonRows,
  Switch,
  Table,
  Td,
  Th,
  Tooltip,
  cn,
} from '@aurorix/webui'
import { Copy, Download, Search, X } from 'lucide-react'
import { PageHeader } from '../components/layout'
import { get, withQuery } from '../lib/api'
import { fmtTime } from '../lib/format'
import { subscribeLogs } from '../lib/live'
import { qk } from '../lib/query'
import { toast } from '../lib/toast'
import type { LogEntry, LogsView } from '../lib/types'

const LEVELS = ['debug', 'info', 'warn', 'error'] as const
const LEVEL_TONE: Record<string, 'ok' | 'warn' | 'danger' | 'neutral'> = {
  DEBUG: 'neutral',
  INFO: 'ok',
  WARN: 'warn',
  ERROR: 'danger',
}
const LIMIT = 500
const RENDER_CAP = 800
// 同来源同模式在这段时间内折成一条; 再长就不像"一次操作的连发"了。
const AGG_WINDOW = 30_000

/** 把消息里会变的部分抹掉当"模式"用 —— 不做真聚类, 够把连发的重复折起来就行。 */
function skeleton(msg: string): string {
  return msg.replace(/[0-9A-Fa-f]{12,}/g, '<id>').replace(/\d+/g, '<n>').slice(0, 120)
}

function shortId(value: string): string {
  return value.length > 12 ? `${value.slice(0, 8)}…` : value
}

type Row = { entry: LogEntry; count: number }

function foldRows(entries: LogEntry[], on: boolean): Row[] {
  if (!on) return entries.map((entry) => ({ entry, count: 1 }))
  const out: Row[] = []
  for (const entry of entries) {
    const prev = out[out.length - 1]
    if (
      prev &&
      prev.entry.scope === entry.scope &&
      prev.entry.level === entry.level &&
      prev.entry.group === entry.group &&
      prev.entry.user === entry.user &&
      skeleton(prev.entry.msg) === skeleton(entry.msg) &&
      Math.abs(prev.entry.time - entry.time) <= AGG_WINDOW
    ) {
      prev.count += 1
      continue
    }
    out.push({ entry, count: 1 })
  }
  return out
}

function lineOf(entry: LogEntry): string {
  return [
    fmtTime(entry.time),
    entry.level,
    entry.scope,
    entry.group && `group=${entry.group}`,
    entry.user && `user=${entry.user}`,
    entry.msg,
  ]
    .filter(Boolean)
    .join(' ')
}

export default function LogsPage() {
  const [params, setParams] = useSearchParams()
  // 默认 info: 进来先看该看的, DEBUG 要自己放宽。
  const minLevel = params.get('level') ?? 'info'
  const scope = params.get('scope') ?? ''
  const group = params.get('group') ?? ''
  const user = params.get('user') ?? ''
  const text = params.get('q') ?? ''
  const aggregate = params.get('agg') !== '0'

  const patch = useCallback(
    (next: Record<string, string>) => {
      const merged = new URLSearchParams(params)
      for (const [key, value] of Object.entries(next)) {
        if (value === '') merged.delete(key)
        else merged.set(key, value)
      }
      setParams(merged, { replace: true })
    },
    [params, setParams],
  )

  const [live, setLive] = useState(true)
  const [liveEntries, setLiveEntries] = useState<LogEntry[]>([])
  const [detail, setDetail] = useState<Row | null>(null)
  const [active, setActive] = useState(-1)
  const [fresh, setFresh] = useState(0)
  const [draft, setDraft] = useState(text)
  const listRef = useRef<HTMLDivElement>(null)
  const pinnedRef = useRef(true)
  const seenTopRef = useRef(0)

  useEffect(() => setDraft(text), [text])
  useEffect(() => {
    if (draft === text) return
    const timer = window.setTimeout(() => patch({ q: draft }), 300)
    return () => window.clearTimeout(timer)
  }, [draft, text, patch])

  const levelParam = minLevel === 'all' ? '' : minLevel
  const { data, isPending, isError, error } = useQuery({
    queryKey: qk.logs({ limit: LIMIT, minLevel, scope, text, group, user }),
    queryFn: () =>
      get<LogsView>(withQuery('/logs', { limit: LIMIT, min_level: levelParam, scope, group, user, q: text })),
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
    const base = data?.entries ?? []
    if (liveEntries.length === 0) return base.slice(0, RENDER_CAP)
    const seen = new Set(base.map((entry) => entry.id))
    const extra = liveEntries.filter((entry) => !seen.has(entry.id))
    return [...extra, ...base].slice(0, RENDER_CAP)
  }, [data, liveEntries])

  const rows = useMemo(() => foldRows(entries, aggregate), [entries, aggregate])

  // 新日志进来时用户正看着历史: 别把列表顶走, 在底部报个数。
  const topId = rows[0]?.entry.id ?? 0
  useEffect(() => {
    if (topId === seenTopRef.current) return
    const first = seenTopRef.current === 0
    seenTopRef.current = topId
    if (first) return
    if (pinnedRef.current) setFresh(0)
    else setFresh((n) => n + 1)
  }, [topId])

  const scrollTop = useCallback(() => {
    listRef.current?.scrollTo({ top: 0, behavior: 'smooth' })
    pinnedRef.current = true
    setFresh(0)
  }, [])

  const focusSearch = useCallback(() => {
    document.getElementById('log-search')?.focus()
  }, [])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const element = event.target as HTMLElement | null
      const tag = element?.tagName ?? ''
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') {
        if (event.key === 'Escape') element?.blur()
        return
      }
      switch (event.key) {
        case '/':
          event.preventDefault()
          focusSearch()
          break
        case 'j':
          event.preventDefault()
          setActive((index) => Math.min(index + 1, rows.length - 1))
          break
        case 'k':
          event.preventDefault()
          setActive((index) => Math.max(index - 1, 0))
          break
        case 'Enter':
          if (rows.length > 0) setDetail(rows[active >= 0 ? active : 0])
          break
        case 'g':
          scrollTop()
          break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [rows, active, scrollTop, focusSearch])

  const copyRow = async (entry: LogEntry) => {
    try {
      await navigator.clipboard.writeText(lineOf(entry))
      toast('已复制这一条')
    } catch {
      toast('复制失败, 浏览器不允许', 'err')
    }
  }

  const exportLogs = () => {
    const header = '时间\t级别\t来源\t群\t用户\t内容'
    const body = rows.map((row) =>
      [
        fmtTime(row.entry.time),
        row.entry.level,
        row.entry.scope,
        row.entry.group ?? '',
        row.entry.user ?? '',
        row.entry.msg.replace(/\n/g, '\\n'),
      ].join('\t'),
    )
    const blob = new Blob([[header, ...body].join('\n')], { type: 'text/tab-separated-values;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `aurorix-logs-${Date.now()}.tsv`
    link.click()
    URL.revokeObjectURL(url)
  }

  const summary = data?.summary
  const levelOptions = [
    { value: 'all', label: '全部级别' },
    ...LEVELS.map((level) => ({
      value: level,
      label: `${level.toUpperCase()}${summary?.levels?.[level] ? ` · ${summary.levels[level]}` : ''}`,
    })),
  ]
  const scopeOptions = [
    { value: '', label: '全部来源' },
    ...(data?.scopes ?? []).map((item) => ({
      value: item,
      label: `${item}${summary?.scopes?.[item] ? ` · ${summary.scopes[item]}` : ''}`,
    })),
  ]
  const filtering = minLevel !== 'info' || scope !== '' || group !== '' || user !== '' || text !== ''

  return (
    <>
      <PageHeader
        title="日志"
        description={`缓冲 ${data?.total ?? 0} 条, 错误 ${data?.errors ?? 0} 条。默认看 INFO 及以上, 查细节再放宽级别。`}
        actions={
          <Button variant="secondary" icon={Download} onClick={exportLogs} disabled={rows.length === 0}>
            导出
          </Button>
        }
      />

      <Panel>
        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1 text-xs text-ink-faint">
            级别
            <Select
              className="w-40"
              value={minLevel}
              onChange={(e) => patch({ level: e.target.value === 'info' ? '' : e.target.value })}
              options={levelOptions}
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-ink-faint">
            来源
            <Select
              className="w-52"
              value={scope}
              onChange={(e) => patch({ scope: e.target.value })}
              options={scopeOptions}
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-ink-faint">
            群
            <Input
              className="w-52 font-mono text-xs"
              value={group}
              placeholder="group=…"
              onChange={(e) => patch({ group: e.target.value.trim() })}
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-ink-faint">
            用户
            <Input
              className="w-52 font-mono text-xs"
              value={user}
              placeholder="user=…"
              onChange={(e) => patch({ user: e.target.value.trim() })}
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-ink-faint">
            关键词
            <span className="relative">
              <Search
                aria-hidden
                className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-faint"
              />
              <Input
                id="log-search"
                className="w-60 pl-9"
                value={draft}
                placeholder="搜内容 (按 / 聚焦)"
                onChange={(e) => setDraft(e.target.value)}
              />
            </span>
          </label>
          <div className="flex items-center gap-4 pb-2.5">
            <Tooltip label="同来源、同模式、30 秒内的重复日志折成一条">
              <label className="flex items-center gap-2 text-xs text-ink-soft">
                <Switch checked={aggregate} onChange={(next) => patch({ agg: next ? '' : '0' })} label="折叠重复" />
                折叠
              </label>
            </Tooltip>
            <label className="flex items-center gap-2 text-xs text-ink-soft">
              <Switch checked={live} onChange={setLive} label="实时推送" />
              实时
            </label>
          </div>
        </div>

        {filtering && (
          <div className="mt-3 flex flex-wrap items-center gap-2 text-xs">
            <span className="text-ink-faint">筛选中:</span>
            {minLevel !== 'info' && (
              <Badge tone="neutral">{minLevel === 'all' ? '全部级别' : `≥ ${minLevel.toUpperCase()}`}</Badge>
            )}
            {scope && <Badge tone="neutral">{scope}</Badge>}
            {group && <Badge tone="neutral">群 {shortId(group)}</Badge>}
            {user && <Badge tone="neutral">用户 {shortId(user)}</Badge>}
            {text && <Badge tone="neutral">“{text}”</Badge>}
            <Button
              variant="ghost"
              size="sm"
              icon={X}
              onClick={() => patch({ level: '', scope: '', group: '', user: '', q: '' })}
            >
              清空
            </Button>
          </div>
        )}
      </Panel>

      {isPending && !data ? (
        <SkeletonRows rows={10} cols={6} />
      ) : isError && !data ? (
        <EmptyState
          title="读取日志失败"
          description={error instanceof Error ? error.message : '请稍后重试'}
        />
      ) : rows.length === 0 ? (
        <EmptyState title="没有匹配的日志" description="换个级别、来源、群或关键词试试。" />
      ) : (
        <div className="relative">
          <div
            ref={listRef}
            onScroll={() => {
              pinnedRef.current = (listRef.current?.scrollTop ?? 0) <= 8
            }}
            className="max-h-[70vh] overflow-auto rounded-xl border border-line"
          >
            <Table>
              <thead className="sticky top-0 z-10">
                <tr>
                  <Th>时间</Th>
                  <Th>级别</Th>
                  <Th>来源</Th>
                  <Th>群</Th>
                  <Th>用户</Th>
                  <Th>内容</Th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row, index) => {
                  const entry = row.entry
                  return (
                    <tr
                      key={entry.id}
                      onClick={() => {
                        setActive(index)
                        setDetail(row)
                      }}
                      className={cn('cursor-pointer', index === active && 'bg-subtle')}
                    >
                      <Td className="whitespace-nowrap font-mono text-xs text-ink-faint">{fmtTime(entry.time)}</Td>
                      <Td>
                        <Badge tone={LEVEL_TONE[entry.level] ?? 'neutral'}>{entry.level}</Badge>
                      </Td>
                      <Td className="whitespace-nowrap font-mono text-xs text-ink-soft">{entry.scope}</Td>
                      <Td className="whitespace-nowrap font-mono text-xs">
                        {entry.group ? (
                          <button
                            type="button"
                            className="text-accent-text underline-offset-2 hover:underline"
                            title={entry.group}
                            onClick={(e) => {
                              e.stopPropagation()
                              patch({ group: entry.group ?? '' })
                            }}
                          >
                            {shortId(entry.group)}
                          </button>
                        ) : (
                          <span className="text-ink-faint">—</span>
                        )}
                      </Td>
                      <Td className="whitespace-nowrap font-mono text-xs">
                        {entry.user ? (
                          <button
                            type="button"
                            className="text-accent-text underline-offset-2 hover:underline"
                            title={entry.user}
                            onClick={(e) => {
                              e.stopPropagation()
                              patch({ user: entry.user ?? '' })
                            }}
                          >
                            {shortId(entry.user)}
                          </button>
                        ) : (
                          <span className="text-ink-faint">—</span>
                        )}
                      </Td>
                      <Td>
                        <div className="flex items-center gap-2">
                          <span className="min-w-0 flex-1 truncate">{entry.msg}</span>
                          {row.count > 1 && (
                            <Tooltip label={`30 秒内重复 ${row.count} 次, 关掉「折叠」看每条`}>
                              <Badge tone="neutral">×{row.count}</Badge>
                            </Tooltip>
                          )}
                          <Tooltip label="复制这一条">
                            <Button
                              variant="ghost"
                              size="icon"
                              icon={Copy}
                              aria-label="复制这一条"
                              onClick={(e) => {
                                e.stopPropagation()
                                copyRow(entry)
                              }}
                            />
                          </Tooltip>
                        </div>
                      </Td>
                    </tr>
                  )
                })}
              </tbody>
            </Table>
          </div>

          {fresh > 0 && (
            <button
              type="button"
              onClick={scrollTop}
              className="absolute bottom-4 left-1/2 -translate-x-1/2 rounded-full bg-ink px-4 py-2 text-xs text-background shadow-lg"
            >
              {fresh} 条新的 · 回到最新
            </button>
          )}
        </div>
      )}

      <Drawer
        open={detail !== null}
        onClose={() => setDetail(null)}
        title="日志详情"
        description={detail ? `${detail.entry.scope} · 第 ${detail.entry.id} 条` : undefined}
        width="lg"
        footer={
          <>
            <Button variant="secondary" onClick={() => detail && copyRow(detail.entry)}>
              复制全文
            </Button>
            <Button variant="secondary" onClick={() => setDetail(null)}>
              关闭
            </Button>
          </>
        }
      >
        {detail && (
          <div className="flex flex-col gap-5">
            <dl className="grid grid-cols-[72px_1fr] items-start gap-y-2 text-sm">
              <dt className="text-ink-faint">时间</dt>
              <dd className="font-mono text-xs">{fmtTime(detail.entry.time)}</dd>
              <dt className="text-ink-faint">级别</dt>
              <dd>
                <Badge tone={LEVEL_TONE[detail.entry.level] ?? 'neutral'}>{detail.entry.level}</Badge>
              </dd>
              <dt className="text-ink-faint">来源</dt>
              <dd className="font-mono text-xs">{detail.entry.scope}</dd>
              <dt className="text-ink-faint">群</dt>
              <dd className="break-all font-mono text-xs">
                {detail.entry.group ?? <span className="text-ink-faint">—</span>}
              </dd>
              <dt className="text-ink-faint">用户</dt>
              <dd className="break-all font-mono text-xs">
                {detail.entry.user ?? <span className="text-ink-faint">—</span>}
              </dd>
              <dt className="text-ink-faint">日志 ID</dt>
              <dd className="font-mono text-xs">#{detail.entry.id}</dd>
              {detail.count > 1 && (
                <>
                  <dt className="text-ink-faint">折叠</dt>
                  <dd className="text-xs text-ink-soft">
                    30 秒内重复 {detail.count} 次(表格里取的是最新那一条)
                  </dd>
                </>
              )}
            </dl>

            <div className="flex flex-col gap-2">
              <span className="text-xs text-ink-faint">内容</span>
              <pre className="whitespace-pre-wrap break-words rounded-lg border border-line bg-surface p-3 font-mono text-xs leading-relaxed">{detail.entry.msg}</pre>
            </div>

            <div className="flex flex-wrap gap-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  patch({ scope: detail.entry.scope })
                  setDetail(null)
                }}
              >
                只看这个来源
              </Button>
              {detail.entry.group && (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    patch({ group: detail.entry.group ?? '' })
                    setDetail(null)
                  }}
                >
                  只看这个群
                </Button>
              )}
              {detail.entry.user && (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    patch({ user: detail.entry.user ?? '' })
                    setDetail(null)
                  }}
                >
                  只看这个用户
                </Button>
              )}
            </div>
          </div>
        )}
      </Drawer>

      <p className="mt-3 text-xs text-ink-faint">
        快捷键: <code className="code-block inline-block px-1.5">/</code> 聚焦搜索 ·{' '}
        <code className="code-block inline-block px-1.5">j</code>/
        <code className="code-block inline-block px-1.5">k</code> 上下移动 ·{' '}
        <code className="code-block inline-block px-1.5">Enter</code> 看详情 ·{' '}
        <code className="code-block inline-block px-1.5">g</code> 回到最新
      </p>
    </>
  )
}
