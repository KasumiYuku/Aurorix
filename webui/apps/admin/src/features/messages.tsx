import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Button, Spinner, cn } from '@aurorix/webui'
import { get, post, withQuery } from '../lib/api'
import { qk } from '../lib/query'
import type { InboxConversation, InboxMessage, InboxMessagesView } from '../lib/types'

const PAGE = 50

/** 会话没有名字时用 openid 前 8 位兜底: 32 位十六进制贴出来谁也没法分辨。 */
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

/** QQ 的 markdown 带平台私有标签, 直接喂给渲染器会整段丢掉, 先把它们翻成人能看的文字。 */
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
    <img src={props.src} alt={props.alt ?? ''} className="my-1 max-h-72 rounded-xl object-contain" />
  ),
  a: (props: { href?: string; children?: React.ReactNode }) => (
    <a href={props.href} target="_blank" rel="noreferrer" className="text-accent-text underline">
      {props.children}
    </a>
  ),
  h1: (props: { children?: React.ReactNode }) => (
    <div className="mb-1 text-[15px] font-semibold">{props.children}</div>
  ),
  h2: (props: { children?: React.ReactNode }) => (
    <div className="mb-1 text-[15px] font-semibold">{props.children}</div>
  ),
  h3: (props: { children?: React.ReactNode }) => (
    <div className="mb-1 text-[14px] font-semibold">{props.children}</div>
  ),
  code: (props: { children?: React.ReactNode }) => (
    <code className="rounded bg-black/10 px-1 py-0.5 font-mono text-[12px]">{props.children}</code>
  ),
  pre: (props: { children?: React.ReactNode }) => (
    <pre className="my-1 overflow-x-auto rounded-lg bg-black/10 p-2 text-[12px]">{props.children}</pre>
  ),
  blockquote: (props: { children?: React.ReactNode }) => (
    <blockquote className="my-1 border-l-2 border-line pl-2 text-ink-faint">{props.children}</blockquote>
  ),
}

function Bubble({ msg, group }: { msg: InboxMessage; group: boolean }) {
  const mine = msg.direction === 'out'
  return (
    <div className={cn('flex flex-col gap-1', mine ? 'items-end' : 'items-start')}>
      <div className="flex items-center gap-2 px-1 text-[11px] text-ink-faint">
        <span className="max-w-[180px] truncate">{mine ? '机器人' : group ? senderLabel(msg) : ''}</span>
        <span>{clock(msg.created_at)}</span>
      </div>
      <div
        className={cn(
          'max-w-[min(680px,85%)] rounded-2xl px-3.5 py-2 text-[13px] leading-relaxed',
          mine ? 'bg-accent-soft text-ink' : 'bg-surface text-ink',
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
    // 群名是后台慢慢回填的, 定时拉一次让它自己长出来, 不用用户手动刷。
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

  // 宽屏右侧本来就要占一块, 空着等于把输入框藏起来; 直接进第一个会话。
  // 窄屏反过来 —— 那里列表才是首屏, 自动跳走会让用户看不见列表。
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
      // 补完历史要把视口往下推同样的高度, 否则用户正看着的那条会被挤走。
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

  const group = (activeConv?.kind ?? 'c2c') === 'group'

  return (
    <div className="absolute inset-0 flex min-h-0 flex-col p-4 md:p-6">
      <div className="flex min-h-0 flex-1 overflow-hidden rounded-2xl border border-line bg-background">
        <aside
          className={cn(
            'flex w-full shrink-0 flex-col border-r border-line bg-surface md:w-[300px]',
            active !== '' && 'hidden md:flex',
          )}
        >
          <div className="border-b border-line p-3">
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索群名或 openid"
              className="w-full rounded-lg border border-line bg-background px-3 py-2 text-[13px] outline-none focus:border-accent"
            />
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">
            {conversations.isPending ? (
              <div className="flex flex-col gap-2 p-3">
                {Array.from({ length: 6 }).map((_, i) => (
                  <div key={i} className="h-12 animate-pulse rounded-lg bg-subtle" />
                ))}
              </div>
            ) : filtered.length === 0 ? (
              <p className="p-6 text-center text-[12px] text-ink-faint">
                {search === '' ? '还没有任何会话往来' : '没有匹配的会话'}
              </p>
            ) : (
              filtered.map((conv) => (
                <button
                  key={conv.id}
                  type="button"
                  onClick={() => setParams({ conv: conv.id })}
                  className={cn(
                    'flex w-full flex-col gap-1 border-b border-line/60 px-3 py-2.5 text-left transition hover:bg-subtle',
                    conv.id === active && 'bg-accent-soft',
                  )}
                >
                  <span className="flex items-center gap-1.5">
                    <span className="rounded bg-subtle px-1 py-0.5 text-[10px] text-ink-faint">
                      {conv.kind === 'group' ? '群' : '私聊'}
                    </span>
                    <span className="min-w-0 flex-1 truncate text-[13px] font-medium">
                      {displayName(conv)}
                    </span>
                  </span>
                  <span className="truncate text-[11px] text-ink-faint">
                    {conv.last_preview || '还没有消息'}
                  </span>
                </button>
              ))
            )}
          </div>
        </aside>

        <section className={cn('flex min-w-0 flex-1 flex-col', active === '' && 'hidden md:flex')}>
          {active === '' ? (
            <div className="grid h-full place-items-center text-[13px] text-ink-faint">
              从左边选一个会话开始
            </div>
          ) : (
            <>
              <header className="flex items-center gap-2 border-b border-line bg-surface px-3 py-2.5">
                <button
                  type="button"
                  onClick={() => setParams({})}
                  className="rounded-lg px-2 py-1 text-[13px] text-ink-faint md:hidden"
                >
                  返回
                </button>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[14px] font-medium">
                    {activeConv ? displayName(activeConv) : active.slice(0, 8)}
                  </p>
                  <p className="truncate text-[11px] text-ink-faint">{active}</p>
                </div>
              </header>

              <div ref={scroller} className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
                {hasMore && (
                  <div className="flex justify-center">
                    <Button variant="ghost" onClick={loadOlder} disabled={loadingOlder}>
                      {loadingOlder ? '加载中…' : '加载更早的消息'}
                    </Button>
                  </div>
                )}
                {current.isPending ? (
                  <div className="flex flex-col gap-3">
                    {Array.from({ length: 4 }).map((_, i) => (
                      <div
                        key={i}
                        className={cn(
                          'h-14 w-[60%] animate-pulse rounded-2xl bg-subtle',
                          i % 2 === 0 ? 'self-start' : 'self-end',
                        )}
                      />
                    ))}
                  </div>
                ) : all.length === 0 ? (
                  <p className="m-auto text-[12px] text-ink-faint">这里还没有消息</p>
                ) : (
                  all.map((msg) => <Bubble key={msg.id} msg={msg} group={group} />)
                )}
              </div>

              <footer className="border-t border-line bg-surface px-3 py-2.5">
                {error !== '' && <p className="mb-2 text-[12px] text-red-500">{error}</p>}
                <div className="flex items-end gap-2">
                  <textarea
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault()
                        submit()
                      }
                    }}
                    rows={2}
                    placeholder="输入内容，Enter 发送，Shift+Enter 换行；支持 markdown"
                    className="min-h-[44px] flex-1 resize-y rounded-lg border border-line bg-background px-3 py-2 text-[13px] outline-none focus:border-accent"
                  />
                  <Button onClick={submit} disabled={send.isPending || draft.trim() === ''}>
                    {send.isPending ? <Spinner className="size-4" /> : '发送'}
                  </Button>
                </div>
              </footer>
            </>
          )}
        </section>
      </div>
    </div>
  )
}
