import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Badge,
  Button,
  Field,
  Input,
  OpsPage,
  OpsSection,
  ResourceTable,
  Textarea,
  TimeAgo,
  Tooltip,
  type Column,
} from '@aurorix/webui'
import { Ban, Pencil, RefreshCw, ShieldOff, Trash2 } from 'lucide-react'
import { del, get, post } from '../lib/api'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { BanEntry, BanList } from '../lib/types'

function preview(text?: string): string {
  const value = (text ?? '').trim()
  if (value === '') return ''
  return value.length > 40 ? `${value.slice(0, 40)}…` : value
}

export default function BansPage() {
  const [openid, setOpenid] = useState('')
  const [message, setMessage] = useState('')
  const [note, setNote] = useState('')
  const [saving, setSaving] = useState(false)
  const [removing, setRemoving] = useState('')

  const { data, isPending, isError, error, isFetching, refetch } = useQuery({
    queryKey: qk.bans,
    queryFn: () => get<BanList>('/bans'),
    staleTime: 10_000,
  })

  const rows = data?.entries ?? []
  const defaultMessage = data?.default ?? '您的账号因违反Bot使用协议已被封禁'
  const limit = data?.limit ?? 2000

  const clearForm = () => {
    setOpenid('')
    setMessage('')
    setNote('')
  }

  const save = async () => {
    const id = openid.trim()
    if (id === '') {
      toast('请先填 openid', 'err')
      return
    }
    setSaving(true)
    try {
      await post<{ ok: boolean }>('/bans', { openid: id, message, note })
      await queryClient.invalidateQueries({ queryKey: qk.bans })
      toast(`已封禁 ${id}`)
      clearForm()
    } catch (err) {
      toast(err instanceof Error ? err.message : '保存失败', 'err')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (id: string) => {
    setRemoving(id)
    try {
      await del<{ ok: boolean }>(`/bans/${encodeURIComponent(id)}`)
      await queryClient.invalidateQueries({ queryKey: qk.bans })
      toast(`已解封 ${id}`)
      if (openid.trim() === id) clearForm()
    } catch (err) {
      toast(err instanceof Error ? err.message : '解封失败', 'err')
    } finally {
      setRemoving('')
    }
  }

  const edit = (entry: BanEntry) => {
    setOpenid(entry.openid)
    setMessage(entry.message ?? '')
    setNote(entry.note ?? '')
  }

  const columns: Column<BanEntry>[] = [
    {
      key: 'openid',
      label: 'OpenID',
      primary: true,
      render: (entry) => <span className="truncate font-mono text-xs text-ink">{entry.openid}</span>,
    },
    {
      key: 'message',
      label: '提示文案',
      hideOnMobile: true,
      render: (entry) =>
        entry.message ? (
          <span className="flex items-center gap-2">
            <Badge tone="accent">自定义</Badge>
            <span className="truncate text-xs text-ink-soft">{preview(entry.message)}</span>
          </span>
        ) : (
          <Badge tone="neutral">默认话术</Badge>
        ),
    },
    {
      key: 'note',
      label: '备注',
      width: '12rem',
      hideOnMobile: true,
      render: (entry) => <span className="truncate text-xs text-ink-soft">{entry.note || '—'}</span>,
    },
    {
      key: 'at',
      label: '封禁时间',
      width: '8rem',
      hideOnMobile: true,
      render: (entry) => <TimeAgo value={entry.at || undefined} />,
    },
    {
      key: 'actions',
      label: '',
      width: '11rem',
      align: 'right',
      render: (entry) => (
        <span className="flex items-center justify-end gap-1.5">
          <Tooltip label="把这一行填进上面的表单">
            <Button variant="secondary" size="sm" icon={Pencil} onClick={() => edit(entry)}>
              编辑
            </Button>
          </Tooltip>
          <Tooltip label="解除封禁">
            <Button
              variant="danger"
              size="sm"
              icon={Trash2}
              loading={removing === entry.openid}
              onClick={() => void remove(entry.openid)}
            >
              解封
            </Button>
          </Tooltip>
        </span>
      ),
    },
  ]

  return (
    <OpsPage
      title="封禁用户"
      description="被封禁的用户发消息或点按钮时只会收到这里配置的文案，指令不会执行"
      meta={data ? `名单 ${rows.length} 人` : isPending ? '加载中' : '读取失败'}
      actions={
        <Button variant="secondary" size="sm" icon={RefreshCw} loading={isFetching} onClick={() => void refetch()}>
          刷新
        </Button>
      }
    >
      <OpsSection title="封禁 / 修改" hint="同一个 openid 再保存一次就是改文案">
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="OpenID" required hint="群成员 / 单聊用户 / 频道用户的统一 openid">
            <Input
              value={openid}
              onChange={(e) => setOpenid(e.target.value)}
              placeholder="例如 618F87D017231E57C5B8DCDD07761AAB"
              autoComplete="off"
              spellCheck={false}
            />
          </Field>
          <Field label="备注" hint="只给管理员看，不会发给用户">
            <Input
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="例如 2026-11-15 刷屏"
              autoComplete="off"
            />
          </Field>
        </div>
        <div className="mt-4">
          <Field label="提示文案" hint={`留空则使用默认：${defaultMessage}`}>
            <Textarea
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              rows={4}
              maxLength={limit}
              placeholder={defaultMessage}
            />
          </Field>
          <div className="mt-1 flex items-center justify-between text-xs text-ink-faint">
            <span>支持 markdown；频道里会自动降级成纯文本</span>
            <span>
              {message.length} / {limit}
            </span>
          </div>
        </div>
        <div className="mt-4 flex items-center gap-2">
          <Button variant="primary" size="sm" icon={ShieldOff} loading={saving} onClick={() => void save()}>
            保存封禁
          </Button>
          <Button variant="ghost" size="sm" onClick={clearForm}>
            清空
          </Button>
        </div>
      </OpsSection>

      <OpsSection title="封禁名单">
        <ResourceTable
          columns={columns}
          rows={data?.entries}
          rowKey={(entry) => entry.openid}
          loading={isPending && !data}
          error={isError && !data ? error : undefined}
          empty={{
            icon: Ban,
            title: '还没有封禁任何人',
            description: '在上面填 openid 并保存，被封的用户发消息时就会收到提示文案。',
          }}
        />
      </OpsSection>
    </OpsPage>
  )
}
