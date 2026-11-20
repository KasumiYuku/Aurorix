import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import {
  AsyncState,
  Badge,
  Button,
  Drawer,
  Input,
  JsonViewer,
  KeyValueList,
  OpsPage,
  OpsSection,
  OpsToolbar,
  Segmented,
  Switch,
  TimeAgo,
  Tooltip,
  cn,
  useDebounced,
  type BadgeTone,
  type KeyValue,
} from '@aurorix/webui'
import { ArrowUp, Copy, Download, Pause, Search, X } from 'lucide-react'
import { get, withQuery } from '../lib/api'
import { fmtTime } from '../lib/format'
import { subscribeLogs } from '../lib/live'
import { qk } from '../lib/query'
import { toast } from '../lib/toast'
import type { LogEntry, LogsView } from '../lib/types'

const LEVELS = ['DEBUG', 'INFO', 'WARN', 'ERROR'] as const
type Level = (typeof LEVELS)[number]

const LEVEL_TONE: Record<Level, BadgeTone> = {
  DEBUG: 'neutral',
  INFO: 'accent',
  WARN: 'warn',
  ERROR: 'danger',
}
const LEVEL_RANK: Record<Level, number> = { DEBUG: 0, INFO: 1, WARN: 2, ERROR: 3 }

const LIMITS = ['100', '500', '1000'] as const
const RENDER_CAP = 1000
const BUFFER_CAP = 500
const AGG_WINDOW = 30_000

const LEVEL_SEGMENTS = [
  { value: 'all', label: '全部' },
  ...LEVELS.map((level) => ({ value: level.toLowerCase(), label: level })),
]

const LIMIT_SEGMENTS = LIMITS.map((value) => ({ value, label: value }))

function levelOf(value: string): Level {
  const upper = value.toUpperCase()
  return (LEVELS as readonly string[]).includes(upper) ? (upper as Level) : 'INFO'
}

function skeleton(msg: string): string {
  return msg.replace(/[0-9A-Fa-f]{12,}/g, '<id>').replace(/\d+/g, '<n>').slice(0, 120)
}

function shortId(value: string): string {
  return value.length > 12 ? `${value.slice(0, 8)}…` : value
}

function searchText(entry: LogEntry): string {
  const fields = Object.entries(entry.fields ?? {}).map(([key, value]) => `${key}=${value}`)
  return [entry.msg, entry.level, entry.scope, entry.group ?? '', entry.user ?? '', entry.trace ?? '', ...fields]
    .filter((part) => part !== '')
    .join(' ')
    .toLowerCase()
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
  const fields = Object.entries(entry.fields ?? {}).map(([key, value]) => `${key}=${value}`)
  return [
    fmtTime(entry.time),
    entry.level,
    entry.scope,
    entry.trace && `[${entry.trace}]`,
    entry.group && `group=${entry.group}`,
    entry.user && `user=${entry.user}`,
    entry.msg,
    ...fields,
  ]
    .filter(Boolean)
    .join(' ')
}

function Highlight({ text, keyword }: { text: string; keyword: string }) {
  const needle = keyword.trim().toLowerCase()
  if (!needle || !text) return <>{text}</>
  const haystack = text.toLowerCase()
  const parts: ReactNode[] = []
  let cursor = 0
  for (;;) {
    const hit = haystack.indexOf(needle, cursor)
    if (hit < 0) {
      parts.push(text.slice(cursor))
      break
    }
    if (hit > cursor) parts.push(text.slice(cursor, hit))
    parts.push(
      <mark key={hit} className="rounded bg-warn/25 text-ink">
        {text.slice(hit, hit + needle.length)}
      </mark>,
    )
    cursor = hit + needle.length
  }
  return <>{parts}</>
}

export default function LogsPage() {
  const [params, setParams] = useSearchParams()
  const minLevel = params.get('level') ?? 'info'
  const scope = params.get('scope') ?? ''
  const group = params.get('group') ?? ''
  const user = params.get('user') ?? ''
  const trace = params.get('trace') ?? ''
  const text = params.get('q') ?? ''
  const rawLimit = params.get('limit') ?? '500'
  const limit = (LIMITS as readonly string[]).includes(rawLimit) ? rawLimit : '500'
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

  const [live, setLive] = useState<LogEntry[]>([])
  const [frozen, setFrozen] = useState(false)
  const [autoscroll, setAutoscroll] = useState(true)
  const [detail, setDetail] = useState<Row | null>(null)
  const [active, setActive] = useState(-1)
  const [fresh, setFresh] = useState(0)
  const [draft, setDraft] = useState(text)
  const keyword = useDebounced(draft, 300)
  const listRef = useRef<HTMLDivElement>(null)
  const pinnedRef = useRef(true)
  const seenTopRef = useRef(0)

  useEffect(() => setDraft(text), [text])
  useEffect(() => {
    if (keyword === text) return
    patch({ q: keyword })
  }, [keyword, text, patch])

  const levelParam = minLevel === 'all' ? '' : minLevel
  const { data, isPending, isError, error, refetch } = useQuery({
    queryKey: qk.logs({ limit: Number(limit), minLevel, scope, text, group, user, trace }),
    queryFn: () =>
      get<LogsView>(withQuery('/logs', { limit, min_level: levelParam, scope, group, user, trace, q: text })),
  })

  const frozenRef = useRef(frozen)
  frozenRef.current = frozen

  useEffect(
    () =>
      subscribeLogs((entry) => {
        if (frozenRef.current) return
        setLive((prev) => (prev.length >= BUFFER_CAP ? prev : [entry, ...prev]))
      }),
    [],
  )

  const entries = useMemo(() => {
    const base = data?.entries ?? []
    if (live.length === 0) return base
    const seen = new Set(base.map((entry) => entry.id))
    const extra: LogEntry[] = []
    for (const entry of live) {
      if (seen.has(entry.id)) continue
      seen.add(entry.id)
      extra.push(entry)
    }
    return [...extra, ...base]
  }, [data, live])

  const visible = useMemo(() => {
    const needle = keyword.trim().toLowerCase()
    const minRank = minLevel === '' || minLevel === 'all' ? -1 : (LEVEL_RANK[levelOf(minLevel)] ?? -1)
    return entries.filter((entry) => {
      if (minRank >= 0 && LEVEL_RANK[levelOf(entry.level)] < minRank) return false
      if (scope && entry.scope !== scope) return false
      if (group && entry.group !== group) return false
      if (user && entry.user !== user) return false
      if (trace && entry.trace !== trace) return false
      if (needle && !searchText(entry).includes(needle)) return false
      return true
    })
  }, [entries, keyword, minLevel, scope, group, user, trace])

  const rows = useMemo(() => foldRows(visible, aggregate), [visible, aggregate])
  const shown = useMemo(() => rows.slice(0, RENDER_CAP), [rows])
  const hidden = rows.length - shown.length

  const topId = shown[0]?.entry.id ?? 0
  useEffect(() => {
    if (topId === seenTopRef.current) return
    const first = seenTopRef.current === 0
    seenTopRef.current = topId
    if (first) return
    if (autoscroll && pinnedRef.current) {
      listRef.current?.scrollTo({ top: 0 })
      setFresh(0)
      return
    }
    setFresh((n) => n + 1)
  }, [topId, autoscroll])

  const scrollTop = useCallback(() => {
    listRef.current?.scrollTo({ top: 0, behavior: 'smooth' })
    pinnedRef.current = true
    setFresh(0)
  }, [])

  const unfreeze = useCallback(() => {
    setFrozen(false)
    void refetch()
  }, [refetch])

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
          setActive((index) => Math.min(index + 1, shown.length - 1))
          break
        case 'k':
          event.preventDefault()
          setActive((index) => Math.max(index - 1, 0))
          break
        case 'Enter':
          if (shown.length > 0) setDetail(shown[active >= 0 ? active : 0])
          break
        case 'g':
          scrollTop()
          break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [shown, active, scrollTop, focusSearch])

  const copyRow = async (entry: LogEntry) => {
    try {
      await navigator.clipboard.writeText(lineOf(entry))
      toast('已复制这一条')
    } catch {
      toast('复制失败, 浏览器不允许', 'err')
    }
  }

  const exportLogs = () => {
    const header = '时间\t级别\t来源\t链路\t群\t用户\t内容\t字段'
    const body = rows.map((row) =>
      [
        fmtTime(row.entry.time),
        row.entry.level,
        row.entry.scope,
        row.entry.trace ?? '',
        row.entry.group ?? '',
        row.entry.user ?? '',
        row.entry.msg.replace(/\n/g, '\\n'),
        JSON.stringify(row.entry.fields ?? {}),
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

  const applyFilter = (next: Record<string, string>) => {
    patch(next)
    setDetail(null)
  }

  const clearFilters = () => {
    setDraft('')
    patch({ level: '', scope: '', group: '', user: '', trace: '', q: '' })
  }

  const summary = data?.summary
  const filtering = minLevel !== 'info' || scope !== '' || group !== '' || user !== '' || trace !== '' || text !== ''

  return (
    <OpsPage
      title="日志"
      description={`缓冲 ${data?.total ?? 0} 条, 错误 ${data?.errors ?? 0} 条。级别筛的是下限, 查细节再放宽。`}
      meta={`显示 ${shown.length} 条${hidden > 0 ? ` (共 ${rows.length} 条, 只显示前 ${RENDER_CAP})` : ''}${live.length > 0 ? ` · 实时 ${live.length}` : ''}`}
    >
      <OpsToolbar
        right={
          <>
            {summary &&
              LEVELS.map((level) => (
                <Badge key={level} tone={LEVEL_TONE[level]}>
                  {level} {summary.levels?.[level.toLowerCase()] ?? 0}
                </Badge>
              ))}
          </>
        }
      >
        <Segmented
          ariaLabel="日志级别"
          options={LEVEL_SEGMENTS}
          value={minLevel}
          onChange={(next) => patch({ level: next })}
        />
        <div className="relative min-w-56 flex-1">
          <Search aria-hidden className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-ink-faint" />
          <Input
            id="log-search"
            aria-label="日志关键字"
            className="pl-8"
            value={draft}
            placeholder="搜内容、来源、字段 (按 / 聚焦)"
            onChange={(e) => setDraft(e.target.value)}
          />
        </div>
        <Segmented
          ariaLabel="条数"
          options={LIMIT_SEGMENTS}
          value={limit as (typeof LIMITS)[number]}
          onChange={(next) => patch({ limit: next === '500' ? '' : next })}
        />
        <Tooltip label="新日志进来时列表自动跳回顶部">
          <span className="flex items-center gap-2 text-xs text-ink-soft">
            <Switch checked={autoscroll} onChange={setAutoscroll} label="自动滚动到最新" />
            自动滚动
          </span>
        </Tooltip>
        <Tooltip label="停止追加新日志，解冻后重新取最新快照">
          <span className="flex items-center gap-2 text-xs text-ink-soft">
            <Switch checked={frozen} onChange={(next) => (next ? setFrozen(true) : unfreeze())} label="冻结追加" />
            冻结追加
          </span>
        </Tooltip>
        {frozen && (
          <Badge tone="warn" icon={Pause}>
            已冻结
          </Badge>
        )}
        {(scope || group || user || trace) && (
          <>
            {scope && <Badge tone="neutral">来源 {scope}</Badge>}
            {group && <Badge tone="neutral">群 {shortId(group)}</Badge>}
            {user && <Badge tone="neutral">用户 {shortId(user)}</Badge>}
            {trace && <Badge tone="neutral">链路 {trace}</Badge>}
            <Button variant="ghost" size="sm" icon={X} onClick={clearFilters}>
              清空
            </Button>
          </>
        )}
      </OpsToolbar>

      <OpsSection
        title="日志流"
        hint={filtering ? '筛选已生效' : undefined}
        actions={
          <>
            <Tooltip label="同来源、同模式、30 秒内的重复日志折成一条">
              <span className="flex items-center gap-2 text-xs text-ink-soft">
                <Switch checked={aggregate} onChange={(next) => patch({ agg: next ? '' : '0' })} label="折叠重复" />
                折叠重复
              </span>
            </Tooltip>
            <Button variant="secondary" size="sm" icon={Download} onClick={exportLogs} disabled={rows.length === 0}>
              导出 TSV
            </Button>
          </>
        }
      >
        <AsyncState
          pending={isPending && !data}
          error={isError && !data ? error : undefined}
          isEmpty={rows.length === 0}
          skeletonRows={10}
          skeletonCols={3}
          onRetry={() => void refetch()}
          empty={{
            title: '没有匹配的日志',
            description: '换个级别、来源、群或关键词试试。',
            action: filtering ? (
              <Button variant="secondary" onClick={clearFilters}>
                清空筛选
              </Button>
            ) : undefined,
          }}
        >
          <div className="relative">
            <div
              ref={listRef}
              onScroll={() => {
                const pinned = (listRef.current?.scrollTop ?? 0) <= 8
                pinnedRef.current = pinned
                if (pinned && fresh !== 0) setFresh(0)
              }}
              className="max-h-[36rem] overflow-y-auto"
            >
              <ul className="divide-y divide-line">
                {shown.map((row, index) => (
                  <LogRow
                    key={row.entry.id}
                    row={row}
                    keyword={keyword}
                    active={index === active}
                    onOpen={() => {
                      setActive(index)
                      setDetail(row)
                    }}
                    onCopy={() => void copyRow(row.entry)}
                  />
                ))}
              </ul>
            </div>
            {fresh > 0 && (
              <div className="pointer-events-none absolute inset-x-0 bottom-4 flex justify-center">
                <Button size="sm" icon={ArrowUp} className="pointer-events-auto" onClick={scrollTop}>
                  {fresh} 条新的 · 回到最新
                </Button>
              </div>
            )}
          </div>
        </AsyncState>
      </OpsSection>

      <Drawer
        open={detail !== null}
        onClose={() => setDetail(null)}
        title="日志详情"
        description={detail ? `${detail.entry.scope} · 第 ${detail.entry.id} 条` : undefined}
        width="lg"
        footer={
          <>
            <Button variant="secondary" onClick={() => detail && void copyRow(detail.entry)}>
              复制全文
            </Button>
            <Button variant="secondary" onClick={() => setDetail(null)}>
              关闭
            </Button>
          </>
        }
      >
        {detail && <LogDetail row={detail} onCopy={() => void copyRow(detail.entry)} onFilter={applyFilter} />}
      </Drawer>

      <p className="mt-3 text-xs text-ink-faint">
        快捷键: <code className="code-block inline-block px-1.5">/</code> 聚焦搜索 ·{' '}
        <code className="code-block inline-block px-1.5">j</code>/
        <code className="code-block inline-block px-1.5">k</code> 上下移动 ·{' '}
        <code className="code-block inline-block px-1.5">Enter</code> 看详情 ·{' '}
        <code className="code-block inline-block px-1.5">g</code> 回到最新 ·{' '}
        <code className="code-block inline-block px-1.5">Esc</code> 关闭详情
      </p>
    </OpsPage>
  )
}

function LogRow({
  row,
  keyword,
  active,
  onOpen,
  onCopy,
}: {
  row: Row
  keyword: string
  active: boolean
  onOpen: () => void
  onCopy: () => void
}) {
  const entry = row.entry
  const level = levelOf(entry.level)
  const fields = entry.fields ?? {}

  const parts: string[] = [entry.scope]
  if (entry.group) parts.push(`群 ${shortId(entry.group)}`)
  if (entry.user) parts.push(`用户 ${shortId(entry.user)}`)
  if (entry.trace) parts.push(entry.trace)
  if (fields.path) parts.push(fields.path)
  if (fields.status) parts.push(fields.status)
  if (fields.duration_ms) parts.push(`${fields.duration_ms}ms`)

  return (
    <li>
      <div
        role="button"
        aria-current={active ? 'true' : undefined}
        onClick={onOpen}
        className={cn(
          'flex w-full cursor-pointer flex-col gap-1 px-2 py-2 text-left transition-colors duration-150 hover:bg-sunk',
          active && 'bg-pressed',
        )}
      >
        <span className="flex min-w-0 items-center gap-2">
          <span className="shrink-0 font-mono text-xs text-ink-faint">{fmtTime(entry.time)}</span>
          <span className="min-w-0 flex-1 truncate font-mono text-sm text-ink">
            <Highlight text={entry.msg} keyword={keyword} />
          </span>
          {row.count > 1 && (
            <Tooltip label={`30 秒内重复 ${row.count} 次, 关掉「折叠」看每条`}>
              <Badge tone="neutral">×{row.count}</Badge>
            </Tooltip>
          )}
          <Badge tone={LEVEL_TONE[level]}>{level}</Badge>
          <TimeAgo value={entry.time} className="shrink-0 text-xs" />
          <Tooltip label="复制这一条">
            <Button
              variant="ghost"
              size="icon"
              icon={Copy}
              aria-label="复制这一条"
              onClick={(e) => {
                e.stopPropagation()
                onCopy()
              }}
            />
          </Tooltip>
        </span>
        <span className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1 font-mono text-xs text-ink-faint">
          {parts.map((part, index) => (
            <Fragment key={`${index}-${part}`}>
              {index > 0 && <span aria-hidden>·</span>}
              <span className="min-w-0 truncate">{part}</span>
            </Fragment>
          ))}
        </span>
      </div>
    </li>
  )
}

function LogDetail({
  row,
  onCopy,
  onFilter,
}: {
  row: Row
  onCopy: () => void
  onFilter: (next: Record<string, string>) => void
}) {
  const entry = row.entry
  const level = levelOf(entry.level)
  const fields = entry.fields ?? {}
  const hasFields = Object.keys(fields).length > 0

  const items: KeyValue[] = [
    { label: '级别', value: <Badge tone={LEVEL_TONE[level]}>{level}</Badge> },
    { label: '来源', value: entry.scope, mono: true },
    { label: '时间', value: fmtTime(entry.time), mono: true, copy: String(entry.time) },
    { label: '链路', value: entry.trace ?? '—', mono: true, copy: entry.trace },
    { label: '群', value: entry.group ?? '—', mono: true, copy: entry.group },
    { label: '用户', value: entry.user ?? '—', mono: true, copy: entry.user },
    { label: '日志 ID', value: `#${entry.id}`, mono: true },
  ]
  if (row.count > 1) {
    items.push({ label: '折叠', value: `30 秒内重复 ${row.count} 次(取最新那一条)` })
  }

  return (
    <div className="flex flex-col gap-5">
      <KeyValueList items={items} />

      <div className="flex flex-col gap-2">
        <span className="text-xs text-ink-faint">内容</span>
        <JsonViewer value={entry.msg} label="内容" maxHeight="16rem" />
      </div>

      {hasFields && (
        <div className="flex flex-col gap-2">
          <span className="text-xs text-ink-faint">字段</span>
          <JsonViewer value={fields} label="字段" maxHeight="16rem" />
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <Button variant="secondary" size="sm" onClick={() => onFilter({ scope: entry.scope })}>
          只看这个来源
        </Button>
        {entry.group && (
          <Button variant="secondary" size="sm" onClick={() => onFilter({ group: entry.group ?? '' })}>
            只看这个群
          </Button>
        )}
        {entry.user && (
          <Button variant="secondary" size="sm" onClick={() => onFilter({ user: entry.user ?? '' })}>
            只看这个用户
          </Button>
        )}
        {entry.trace && (
          <Button variant="secondary" size="sm" onClick={() => onFilter({ trace: entry.trace ?? '', level: 'all' })}>
            只看这条链路
          </Button>
        )}
        <Button variant="ghost" size="sm" icon={Copy} onClick={onCopy}>
          复制这一条
        </Button>
      </div>
    </div>
  )
}
