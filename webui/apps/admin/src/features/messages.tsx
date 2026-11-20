import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Badge, Button, DangerNote, EmptyState, Input, SkeletonRows, Textarea, cn } from '@aurorix/webui'
import { ChevronLeft, MessageSquare, MessagesSquare, Search } from 'lucide-react'
import { PageHeader } from '../components/layout'
import { get, post, withQuery } from '../lib/api'
import { qk } from '../lib/query'
import { sceneShort, showsSenderName } from '../lib/scene'
import type { InboxConversation, InboxMessage, InboxMessagesView } from '../lib/types'

const PAGE = 50

function displayName(conv: InboxConversation): string {
  return conv.name || conv.id.slice(0, 8)
}

function senderLabel(msg: InboxMessage): string {
  return msg.sender_name || msg.sender_id?.slice(0, 8) || '未知'
}

function clock(iso: string): string {
  const at = new Date(iso)
  if (Number.isNaN(at.getTime()) || at.getFullYear() < 2000) return ''
  return at.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

function normalizeQQMarkdown(raw: string): string {
  return raw
    .replace(/<qqbot-cmd-input\b[^>]*?show="([^"]*)"[^>]*?\/?>/g, '`$1`')
    .replace(/<qqbot-cmd-input\b[^>]*?text="([^"]*)"[^>]*?\/?>/g, '`$1`')
    .replace(/<qqbot-cmd-enter\b[^>]*?text="([^"]*)"[^>]*?\/?>/g, '`$1`')
    .replace(/<qqbot-at-user\b[^>]*?id="([^"]*)"[^>]*?\/?>/g, '@$1')
    .replace(/<qqbot-at-everyone\s*\/?>/g, '@全体成员')
    .replace(/<@([A-Za-z0-9]+)>/g, '@$1')
    .replace(/(!\[[^\]]*?)\s*#\d+px\s*#\d+px\]/g, '$1]')
}

const mdComponents = {
  img: (props: { src?: string; alt?: string }) => (
    <img src={props.src} alt={props.alt ?? ''} className="my-1 max-h-72 rounded object-contain" />
  ),
  a: (props: { href?: string; children?: React.ReactNode }) => (
    <a href={props.href} target="_blank" rel="noreferrer" className="text-accent underline">
      {props.children}
    </a>
  ),
  h1: (props: { children?: React.ReactNode }) => (
    <div className="mb-1 text-sm font-semibold">{props.children}</div>
  ),
  h2: (props: { children?: React.ReactNode }) => (
    <div className="mb-1 text-sm font-semibold">{props.children}</div>
  ),
  h3: (props: { children?: React.ReactNode }) => (
    <div className="mb-1 text-sm font-semibold">{props.children}</div>
  ),
  code: (props: { children?: React.ReactNode }) => (
    <code className="rounded bg-paper px-1 py-0.5 font-mono text-xs">{props.children}</code>
  ),
  pre: (props: { children?: React.ReactNode }) => (
    <pre className="my-1 overflow-x-auto rounded-md bg-paper p-2 text-xs">{props.children}</pre>
  ),
  blockquote: (props: { children?: React.ReactNode }) => (
    <blockquote className="my-1 border-l-2 border-line pl-2 text-ink-faint">{props.children}</blockquote>
  ),
}

function Bubble({ msg, showSender }: { msg: InboxMessage; showSender: boolean }) {
  const mine = msg.direction === 'out'
  return (
    <div className={cn('flex flex-col gap-1', mine ? 'items-end' : 'items-start')}>
      <div className="flex items-center gap-2 px-1 text-xs text-ink-faint">
        <span className="max-w-[180px] truncate">{mine ? '机器人' : showSender ? senderLabel(msg) : ''}</span>
        <span className="tabular-nums">{clock(msg.created_at)}</span>
      </div>
      <div
        className={cn(
          'max-w-[min(680px,85%)] rounded-lg px-3 py-2 text-sm leading-relaxed',
          mine ? 'bg-sunk text-ink' : 'border border-line bg-surface text-ink',
        )}
      >
        <Markdown remarkPlugins={[remarkGfm]} components={mdComponents}>
          {normalizeQQMarkdown(msg.content)}
        </Markdown>
      </div>
    </div>
  )
}

export default function MessagesPage() {
  const [params, setParams] = useSearchParams()
  const active = params.get('conv') ?? ''
  const [search, setSearch] = useState('')
  const [draft, setDraft] = useState('')
  const [older, setOlder] = useState<InboxMessage[]>([])
  const [hasMore, setHasMore] = useState(false)
  const [loadingOlder, setLoadingOlder] = useState(false)
  const [error, setError] = useState('')
  const scroller = useRef<HTMLDivElement>(null)
  const queryClient = useQueryClient()

  const conversations = useQuery({
    queryKey: qk.inboxConversations,
    queryFn: () => get<{ conversations: InboxConversation[] }>('/inbox/conversations'),
    refetchInterval: 30_000,
  })

  const current = useQuery({
    queryKey: qk.inboxMessages(active),
    queryFn: () => get<InboxMessagesView>(withQuery('/inbox/messages', { conv: active, limit: PAGE })),
    enabled: active !== '',
  })

  const latest = current.data?.messages ?? []
  const all = useMemo(() => [...older, ...latest], [older, latest])

  const filtered = useMemo(() => {
    const list = conversations.data?.conversations ?? []
    const needle = search.trim().toLowerCase()
    if (needle === '') return list
    return list.filter(
      (c) => displayName(c).toLowerCase().includes(needle) || c.id.toLowerCase().includes(needle),
    )
  }, [conversations.data, search])

  const activeConv = useMemo(
    () => (conversations.data?.conversations ?? []).find((c) => c.id === active),
    [conversations.data, active],
  )

  useEffect(() => {
    setOlder([])
    setHasMore(false)
    setError('')
  }, [active])

  useEffect(() => {
    if (active !== '' || !conversations.data) return
    const first = conversations.data.conversations[0]
    if (!first) return
    if (window.matchMedia('(min-width: 768px)').matches) {
      setParams({ conv: first.id }, { replace: true })
    }
  }, [active, conversations.data, setParams])

  useEffect(() => {
    if (current.data && latest.length > 0 && older.length === 0) {
      setHasMore(current.data.has_more)
      const box = scroller.current
      if (box) box.scrollTop = box.scrollHeight
    }
  }, [current.data, latest.length, older.length])

  const send = useMutation({
    mutationFn: (text: string) => post<{ ok: boolean }>('/inbox/messages', { conv: active, text }),
    onSuccess: async () => {
      setDraft('')
      setError('')
      await queryClient.invalidateQueries({ queryKey: qk.inboxMessages(active) })
      await queryClient.invalidateQueries({ queryKey: qk.inboxConversations })
      const box = scroller.current
      if (box) box.scrollTop = box.scrollHeight
    },
    onError: (err) => setError(err instanceof Error ? err.message : '发送失败'),
  })

  async function loadOlder() {
    const anchor = older[0] ?? latest[0]
    if (!anchor || loadingOlder) return
    setLoadingOlder(true)
    const box = scroller.current
    const heightBefore = box?.scrollHeight ?? 0
    try {
      const view = await get<InboxMessagesView>(
        withQuery('/inbox/messages', { conv: active, before: anchor.id, limit: PAGE }),
      )
      setOlder((prev) => [...view.messages, ...prev])
      setHasMore(view.has_more)
      requestAnimationFrame(() => {
        if (box) box.scrollTop += box.scrollHeight - heightBefore
      })
    } finally {
      setLoadingOlder(false)
    }
  }

  function submit() {
    const text = draft.trim()
    if (text === '' || send.isPending) return
    send.mutate(text)
  }

  const showSender = activeConv ? showsSenderName(activeConv.kind) : false

  return (
    <>
      <PageHeader
        title="消息"
        description="和机器人的收发记录：左边按会话分组，右边看上下文，并以机器人身份直接回复。"
        meta={`${filtered.length} 个会话${active !== '' ? ` · 当前 ${active.slice(0, 8)}` : ''}`}
      />

      <div className="grid h-[calc(100dvh-10rem)] min-h-[28rem] gap-4 md:grid-cols-[16rem_1fr]">
        <aside
          className={cn(
            'flex min-h-0 flex-col overflow-hidden rounded-lg border border-line bg-surface',
            active !== '' && 'hidden md:flex',
          )}
        >
          <div className="relative shrink-0 border-b border-line p-2">
            <Search aria-hidden className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-faint" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索会话名或 ID"
              aria-label="搜索会话"
              className="pl-9"
            />
          </div>
          <div className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-2">
            {conversations.isPending ? (
              <SkeletonRows rows={6} cols={1} />
            ) : filtered.length === 0 ? (
              <EmptyState
                icon={MessagesSquare}
                title={search === '' ? '还没有任何会话往来' : '没有匹配的会话'}
                description={search === '' ? '有人给机器人发过消息后，这里会出现会话。' : '换个关键词，或直接搜 openid。'}
              />
            ) : (
              filtered.map((conv) => (
                <Button
                  key={conv.id}
                  variant="ghost"
                  aria-current={conv.id === active ? 'page' : undefined}
                  onClick={() => setParams({ conv: conv.id })}
                  className={cn(
                    'nav-item h-auto w-full flex-col items-start justify-start gap-0.5 px-2 py-2 text-left',
                    conv.id === active && 'bg-pressed',
                  )}
                >
                  <span className="flex w-full min-w-0 items-center gap-1.5">
                    <Badge>{sceneShort(conv.kind)}</Badge>
                    <span className="min-w-0 flex-1 truncate text-sm font-medium">{displayName(conv)}</span>
                  </span>
                  <span className="w-full truncate text-xs text-ink-faint">{conv.last_preview || '还没有消息'}</span>
                </Button>
              ))
            )}
          </div>
        </aside>

        <section
          className={cn(
            'flex min-h-0 min-w-0 flex-col overflow-hidden rounded-lg border border-line bg-surface',
            active === '' && 'hidden md:flex',
          )}
        >
          {active === '' ? (
            <div className="grid flex-1 place-items-center p-4">
              <EmptyState icon={MessageSquare} title="还没有选中会话" description="从左侧列表选一个会话，就能看到往来记录。" />
            </div>
          ) : (
            <>
              <header className="flex shrink-0 items-center gap-2 border-b border-line px-3 py-2">
                <Button
                  variant="ghost"
                  size="sm"
                  icon={ChevronLeft}
                  className="md:hidden"
                  onClick={() => setParams({})}
                >
                  返回
                </Button>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{activeConv ? displayName(activeConv) : active.slice(0, 8)}</p>
                  <p className="truncate font-mono text-xs text-ink-faint">{active}</p>
                </div>
              </header>

              <div ref={scroller} className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-3">
                {hasMore && (
                  <div className="flex justify-center">
                    <Button variant="secondary" size="sm" loading={loadingOlder} onClick={loadOlder}>
                      加载更早的消息
                    </Button>
                  </div>
                )}
                {current.isPending ? (
                  <SkeletonRows rows={4} cols={2} />
                ) : all.length === 0 ? (
                  <div className="grid flex-1 place-items-center p-4">
                    <EmptyState icon={MessageSquare} title="这里还没有消息" description="在下面发一条，或者加载更早的记录。" />
                  </div>
                ) : (
                  all.map((msg) => <Bubble key={msg.id} msg={msg} showSender={showSender} />)
                )}
              </div>

              <footer className="shrink-0 border-t border-line p-2">
                {error !== '' && <DangerNote className="mb-2">{error}</DangerNote>}
                <div className="flex items-end gap-2">
                  <Textarea
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault()
                        submit()
                      }
                    }}
                    rows={2}
                    aria-label="发送内容"
                    placeholder="输入内容，Enter 发送，Shift+Enter 换行；支持 markdown"
                    className="min-h-11 flex-1 resize-y"
                  />
                  <Button onClick={submit} loading={send.isPending} disabled={draft.trim() === ''}>
                    发送
                  </Button>
                </div>
              </footer>
            </>
          )}
        </section>
      </div>
    </>
  )
}
