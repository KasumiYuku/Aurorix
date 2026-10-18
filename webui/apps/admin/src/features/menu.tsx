import { useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Badge,
  Button,
  ConfirmDialog,
  DangerNote,
  Drawer,
  EmptyState,
  Field,
  Input,
  Modal,
  Panel,
  Select,
  SkeletonRows,
  Spinner,
  Switch,
  Table,
  Tabs,
  Td,
  Th,
  Tooltip,
} from '@aurorix/webui'
import { ArrowDown, ArrowUp, Copy, GripVertical, History, Pencil, Plus, RotateCcw, Trash2, TriangleAlert } from 'lucide-react'
import { PageHeader } from '../components/layout'
import { del, get, post, put } from '../lib/api'
import { fmtTime } from '../lib/format'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type {
  MenuLimits,
  MenuMeta,
  MenuPanels,
  MenuScopePanels,
  MenuSnapshot,
  MenuSnapshots,
  PanelItem,
  PanelItemType,
  PanelRecord,
  PanelRestoreResult,
  PanelScope,
  PanelSyncResult,
  PanelSyncTarget,
  PanelWriteResult,
} from '../lib/types'

// 与后端 lib/api/panel.go 同一口径: 非 ASCII 记 2。不是字符数 —— 21 个汉字按字符数没超,
// 按显示宽度就超了, 而服务端只回一句含糊的 30013。
function displayWidth(text: string): number {
  let width = 0
  for (const ch of text) width += (ch.codePointAt(0) ?? 0) < 128 ? 1 : 2
  return width
}

function widthText(text: string, limit: number) {
  const width = displayWidth(text)
  return { width, limit, over: width > limit }
}

function itemError(item: PanelItem, limits: MenuLimits): string | null {
  if (item.name.trim() === '') return '名字不能为空'
  if (item.desc.trim() === '') return '描述不能为空'
  const name = widthText(item.name, limits.name_width)
  if (name.over) return `名字显示宽度 ${name.width}, 超过 ${name.limit}(中文与全角各计 2)`
  const desc = widthText(item.desc, limits.desc_width)
  if (desc.over) return `描述显示宽度 ${desc.width}, 超过 ${desc.limit}(中文与全角各计 2)`
  if (item.type === 'link' && !item.link?.startsWith('https://')) return '链接类型必须以 https:// 开头'
  return null
}

/** 只读展示: 指定群/指定人的面板当前不支持编辑, 但必须让人看见它占着名额。 */
function isEditable(record: PanelRecord): boolean {
  return record.target_type === 'all'
}

type Draft = { scope: PanelScope; record: PanelRecord | null; items: PanelItem[]; remark: string }

function emptyItem(): PanelItem {
  return { name: '', desc: '', type: 'command' }
}

function PanelEditor({
  draft,
  limits,
  saving,
  onChange,
  onCancel,
  onSave,
}: {
  draft: Draft
  limits: MenuLimits
  saving: boolean
  onChange: (next: Draft) => void
  onCancel: () => void
  onSave: () => void
}) {
  const dragFrom = useRef<number | null>(null)
  const [dragging, setDragging] = useState<number | null>(null)

  const errors = draft.items.map((item) => itemError(item, limits))
  const overLimit = draft.items.length > limits.max_items
  const blocked = errors.some((err) => err !== null) || overLimit

  const patch = (index: number, next: Partial<PanelItem>) => {
    onChange({ ...draft, items: draft.items.map((item, i) => (i === index ? { ...item, ...next } : item)) })
  }

  const move = (from: number, to: number) => {
    if (to < 0 || to >= draft.items.length || from === to) return
    const items = [...draft.items]
    const [moved] = items.splice(from, 1)
    items.splice(to, 0, moved)
    onChange({ ...draft, items })
  }

  const remove = (index: number) => {
    onChange({ ...draft, items: draft.items.filter((_, i) => i !== index) })
  }

  return (
    <Drawer
      open
      onClose={onCancel}
      title={draft.record ? '编辑指令面板' : '新建指令面板'}
      description={
        draft.record
          ? `保存会整份覆盖服务端内容, 不可撤销 —— 后端会先把改动前的版本留成快照。`
          : '面板内容整份提交, 没有按条目增删的接口。'
      }
      width="lg"
      footer={
        <>
          <span className="mr-auto text-xs text-ink-faint">
            指令项 {draft.items.length}/{limits.max_items}
          </span>
          <Button variant="secondary" onClick={onCancel} disabled={saving}>
            取消
          </Button>
          <Button onClick={onSave} loading={saving} disabled={blocked}>
            保存
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        <Field
          label="备注"
          hint={`只给自己看, 不展示给用户。上限 ${limits.remark_len} 个字符。`}
          error={
            draft.remark.length > limits.remark_len ? `备注 ${draft.remark.length} 个字符, 超过 ${limits.remark_len}` : undefined
          }
        >
          <Input
            value={draft.remark}
            maxLength={limits.remark_len}
            placeholder="例如: Okamikko 指令面板"
            onChange={(e) => onChange({ ...draft, remark: e.target.value })}
          />
        </Field>

        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="font-serif text-sm text-ink">指令项</h3>
            <span className="text-xs text-ink-faint">{limits.display_note}</span>
          </div>

          {draft.items.length === 0 && (
            <p className="rounded-lg border border-dashed border-line px-4 py-6 text-center text-sm text-ink-faint">
              还没有指令项, 加一条吧
            </p>
          )}

          <ul className="flex flex-col gap-2">
            {draft.items.map((item, index) => {
              const name = widthText(item.name, limits.name_width)
              const desc = widthText(item.desc, limits.desc_width)
              return (
                <li
                  key={index}
                  draggable
                  onDragStart={() => {
                    dragFrom.current = index
                    setDragging(index)
                  }}
                  onDragEnd={() => {
                    dragFrom.current = null
                    setDragging(null)
                  }}
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={(e) => {
                    e.preventDefault()
                    if (dragFrom.current !== null) move(dragFrom.current, index)
                    dragFrom.current = null
                    setDragging(null)
                  }}
                  className={`rounded-lg border border-line bg-surface p-3 transition-opacity ${
                    dragging === index ? 'opacity-50' : ''
                  }`}
                >
                  <div className="flex items-start gap-2">
                    <span className="mt-2.5 cursor-grab text-ink-faint" title="拖动排序" aria-hidden>
                      <GripVertical className="size-4" />
                    </span>
                    <div className="flex min-w-0 flex-1 flex-col gap-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <Input
                          value={item.name}
                          aria-label={`第 ${index + 1} 项名字`}
                          aria-invalid={name.over || undefined}
                          placeholder="名字(用户在菜单里看到)"
                          onChange={(e) => patch(index, { name: e.target.value })}
                          className={name.over ? 'text-danger-text' : ''}
                        />
                        <span className={`shrink-0 text-xs tabular-nums ${name.over ? 'text-danger-text' : 'text-ink-faint'}`}>
                          {name.width}/{name.limit}
                        </span>
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <Input
                          value={item.desc}
                          aria-label={`第 ${index + 1} 项描述`}
                          aria-invalid={desc.over || undefined}
                          placeholder="描述"
                          onChange={(e) => patch(index, { desc: e.target.value })}
                          className={desc.over ? 'text-danger-text' : ''}
                        />
                        <span className={`shrink-0 text-xs tabular-nums ${desc.over ? 'text-danger-text' : 'text-ink-faint'}`}>
                          {desc.width}/{desc.limit}
                        </span>
                      </div>
                      <div className="flex flex-wrap items-center gap-3">
                        <Select
                          aria-label={`第 ${index + 1} 项类型`}
                          value={item.type}
                          options={[
                            { value: 'command', label: '指令' },
                            { value: 'link', label: '链接' },
                          ]}
                          onChange={(e) => {
                            const type = e.target.value as PanelItemType
                            patch(index, { type, link: type === 'link' ? (item.link ?? '') : undefined })
                          }}
                          className="max-w-32"
                        />
                        {item.type === 'link' && (
                          <Input
                            value={item.link ?? ''}
                            aria-label={`第 ${index + 1} 项链接`}
                            placeholder="https://…"
                            onChange={(e) => patch(index, { link: e.target.value })}
                          />
                        )}
                        <Switch
                          checked={Boolean(item.only_admin)}
                          onChange={(next) => patch(index, { only_admin: next })}
                          label={`第 ${index + 1} 项仅管理员可见`}
                        />
                        <span className="text-xs text-ink-faint">仅管理员可见</span>
                      </div>
                      {errors[index] && (
                        <p className="flex items-center gap-1.5 text-xs text-danger-text">
                          <TriangleAlert aria-hidden className="size-3.5 shrink-0" />
                          {errors[index]}
                        </p>
                      )}
                    </div>
                    <div className="flex shrink-0 flex-col gap-1">
                      <Tooltip label="上移">
                        <Button
                          variant="ghost"
                          size="icon"
                          icon={ArrowUp}
                          aria-label="上移"
                          disabled={index === 0}
                          onClick={() => move(index, index - 1)}
                        />
                      </Tooltip>
                      <Tooltip label="下移">
                        <Button
                          variant="ghost"
                          size="icon"
                          icon={ArrowDown}
                          aria-label="下移"
                          disabled={index === draft.items.length - 1}
                          onClick={() => move(index, index + 1)}
                        />
                      </Tooltip>
                      <Tooltip label="删除这一项">
                        <Button
                          variant="ghost"
                          size="icon"
                          icon={Trash2}
                          aria-label="删除这一项"
                          onClick={() => remove(index)}
                        />
                      </Tooltip>
                    </div>
                  </div>
                </li>
              )
            })}
          </ul>

          <div className="flex items-center gap-3">
            <Button
              variant="secondary"
              icon={Plus}
              disabled={draft.items.length >= limits.max_items}
              onClick={() => onChange({ ...draft, items: [...draft.items, emptyItem()] })}
            >
              添加指令项
            </Button>
            {draft.items.length >= limits.max_items && (
              <span className="text-xs text-ink-faint">已达上限 {limits.max_items} 条</span>
            )}
          </div>
        </div>
      </div>
    </Drawer>
  )
}

function SnapshotsModal({
  open,
  onClose,
  onRestore,
  pending,
}: {
  open: boolean
  onClose: () => void
  onRestore: (snapshot: MenuSnapshot) => void
  pending: string
}) {
  const { data, isPending } = useQuery({
    queryKey: qk.menuSnapshots,
    queryFn: () => get<MenuSnapshots>('/menu/snapshots'),
    enabled: open,
    staleTime: 0,
  })

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="改动快照"
      description="每次覆盖或删除之前, 后端都会把当时的版本留一份。回滚就是把快照内容写回服务端。"
      width="lg"
      footer={<Button variant="secondary" onClick={onClose}>关闭</Button>}
    >
      {isPending && !data ? (
        <SkeletonRows rows={4} cols={4} />
      ) : !data || data.snapshots.length === 0 ? (
        <EmptyState title="还没有快照" description="第一次覆盖或删除面板后, 这里会出现改动前的版本。" />
      ) : (
        <div className="flex flex-col gap-3">
          <p className="text-xs text-ink-faint">
            目录 <code className="code-block inline-block px-2 py-0.5">{data.dir}</code>，每个面板保留最近 20 份。
          </p>
          <Table>
            <thead>
              <tr>
                <Th>时间</Th>
                <Th>动作</Th>
                <Th>场景</Th>
                <Th>面板</Th>
                <Th>条目</Th>
                <Th>操作</Th>
              </tr>
            </thead>
            <tbody>
              {data.snapshots.map((snapshot) => (
                <tr key={snapshot.file}>
                  <Td className="whitespace-nowrap text-xs">{fmtTime(Date.parse(snapshot.saved_at))}</Td>
                  <Td>
                    <Badge tone={snapshot.action === 'delete' ? 'danger' : 'neutral'}>
                      {snapshot.action === 'delete' ? '删除前' : '覆盖前'}
                    </Badge>
                  </Td>
                  <Td className="text-xs text-ink-soft">{snapshot.scope}</Td>
                  <Td className="font-mono text-xs">{snapshot.panel_id}</Td>
                  <Td className="text-xs tabular-nums">{snapshot.panel.items.length}</Td>
                  <Td>
                    <Button
                      variant="secondary"
                      size="sm"
                      icon={RotateCcw}
                      loading={pending === snapshot.file}
                      onClick={() => onRestore(snapshot)}
                    >
                      回滚
                    </Button>
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </div>
      )}
    </Modal>
  )
}

type SyncChoice = { on: boolean; panelId: string }

function SyncModal({
  source,
  scopes,
  total,
  maxPerBot,
  saving,
  onClose,
  onSync,
}: {
  source: PanelRecord
  scopes: MenuScopePanels[]
  total: number
  maxPerBot: number
  saving: boolean
  onClose: () => void
  onSync: (targets: PanelSyncTarget[]) => Promise<PanelSyncResult | null>
}) {
  const others = useMemo(() => scopes.filter((item) => item.scope !== source.scope), [scopes, source.scope])
  const [choice, setChoice] = useState<Record<string, SyncChoice>>(() => {
    const init: Record<string, SyncChoice> = {}
    for (const item of scopes) {
      if (item.scope === source.scope) continue
      const first = item.panels.filter(isEditable)[0]
      init[item.scope] = { on: true, panelId: first?.panel_id ?? '' }
    }
    return init
  })
  const [result, setResult] = useState<PanelSyncResult | null>(null)

  const picked = others.filter((item) => choice[item.scope]?.on)
  const creating = picked.filter((item) => !choice[item.scope].panelId).length
  const overLimit = creating > 0 && total + creating > maxPerBot

  const submit = async () => {
    const targets: PanelSyncTarget[] = picked.map((item) => ({
      scope: item.scope,
      panel_id: choice[item.scope].panelId || undefined,
    }))
    setResult(await onSync(targets))
  }

  const labelOf = (scope: PanelScope) => others.find((item) => item.scope === scope)?.label ?? scope

  return (
    <Modal
      open
      onClose={onClose}
      title="同步到其他场景"
      description={`用这份面板的 ${source.panel.items.length} 条指令项覆盖别处。没选中的场景不动, 覆盖前会自动留快照。`}
      width="lg"
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={saving}>
            {result ? '关闭' : '取消'}
          </Button>
          <Button onClick={submit} loading={saving} disabled={result !== null || picked.length === 0 || overLimit}>
            同步
          </Button>
        </>
      }
    >
      {result ? (
        <div className="flex flex-col gap-2">
          {result.results.map((item) => (
            <div key={item.scope} className="flex flex-wrap items-center gap-2 text-sm">
              <span className="w-16 shrink-0 text-ink-soft">{labelOf(item.scope)}</span>
              {item.error ? (
                <Badge tone="danger">失败</Badge>
              ) : (
                <Badge tone="neutral">{item.mode === 'created' ? '已新建' : '已覆盖'}</Badge>
              )}
              <span className="break-all text-xs text-ink-faint">{item.error ?? item.panel_id}</span>
            </div>
          ))}
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          {others.map((item) => {
            const now = choice[item.scope]
            const writable = item.panels.filter(isEditable)
            const allReadOnly = item.panels.length > 0 && writable.length === 0
            return (
              <div key={item.scope} className="flex flex-wrap items-start gap-3">
                <Switch
                  checked={now?.on ?? false}
                  onChange={(next) =>
                    setChoice((prev) => ({ ...prev, [item.scope]: { ...prev[item.scope], on: next } }))
                  }
                  label={`同步到${item.label}`}
                />
                <div className="min-w-40 flex-1">
                  <div className="text-sm">{item.label}</div>
                  <div className="text-xs text-ink-faint">
                    {allReadOnly
                      ? `${item.panels.length} 份都只对指定对象生效, 这里动不了`
                      : item.panels.length === 0
                        ? '还没有面板, 会新建一份(占 1 个名额)'
                        : `共 ${item.panels.length} 份, 选一份覆盖`}
                  </div>
                </div>
                {now?.on && writable.length > 0 && (
                  <Select
                    className="w-56"
                    value={now.panelId}
                    onChange={(e) =>
                      setChoice((prev) => ({
                        ...prev,
                        [item.scope]: { ...prev[item.scope], panelId: e.target.value },
                      }))
                    }
                    options={writable.map((record) => ({
                      value: record.panel_id,
                      label: `${record.panel.remark || record.panel_id} · ${record.panel.items.length} 条`,
                    }))}
                  />
                )}
              </div>
            )
          })}
          <p className="text-xs text-ink-faint">
            当前 {total}/{maxPerBot} 个{creating > 0 ? `, 本次会新建 ${creating} 份` : ''}。
            {overLimit && ' 名额不够, 先删掉不用的面板, 或关掉几个场景。'}
          </p>
        </div>
      )}
    </Modal>
  )
}

export default function MenuPage() {
  const [scope, setScope] = useState<PanelScope>('group')
  const [draft, setDraft] = useState<Draft | null>(null)
  const [saving, setSaving] = useState(false)
  const [pending, setPending] = useState('')
  const [snapshotsOpen, setSnapshotsOpen] = useState(false)
  const [syncSource, setSyncSource] = useState<PanelRecord | null>(null)
  const [confirm, setConfirm] = useState<
    | { kind: 'delete'; record: PanelRecord }
    | { kind: 'save'; draft: Draft }
    | { kind: 'restore'; snapshot: MenuSnapshot }
    | null
  >(null)

  const meta = useQuery({
    queryKey: qk.menuMeta,
    queryFn: () => get<MenuMeta>('/menu/meta'),
    staleTime: 5 * 60_000,
  })
  const panels = useQuery({
    queryKey: qk.menu,
    queryFn: () => get<MenuPanels>('/menu/panels'),
    staleTime: 5_000,
  })

  const limits = meta.data?.limits
  const scopeTabs = useMemo(
    () =>
      (panels.data?.scopes ?? meta.data?.scopes.map((item) => ({ ...item, panels: [] })) ?? []).map((item) => ({
        id: item.scope,
        label: item.label,
        badge: (
          <span className="rounded-full bg-subtle px-2 text-xs tabular-nums text-ink-soft">{item.panels.length}</span>
        ),
      })),
    [panels.data, meta.data],
  )

  const current = panels.data?.scopes.find((item) => item.scope === scope)
  const total = panels.data?.total ?? 0
  const maxPerBot = panels.data?.max_per_bot ?? limits?.max_per_bot ?? 0
  const atLimit = maxPerBot > 0 && total >= maxPerBot

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: qk.menu })
    await queryClient.invalidateQueries({ queryKey: qk.menuSnapshots })
  }

  const runSave = async (target: Draft) => {
    setSaving(true)
    try {
      if (target.record) {
        const res = await put<PanelWriteResult>(`/menu/panels/${target.record.panel_id}`, {
          panel: { items: target.items, remark: target.remark },
        })
        toast(res.snapshot ? `已覆盖, 改动前的版本留在 ${res.snapshot}` : '已覆盖')
      } else {
        const res = await post<PanelWriteResult>('/menu/panels', {
          scope: target.scope,
          panel: { items: target.items, remark: target.remark },
        })
        toast(`已新建面板 ${res.panel_id ?? ''}`.trim())
      }
      await refresh()
      setDraft(null)
    } catch (err) {
      toast(err instanceof Error ? err.message : '保存失败', 'err')
    } finally {
      setSaving(false)
      setConfirm(null)
    }
  }

  const runDelete = async (record: PanelRecord) => {
    setPending(record.panel_id)
    try {
      const res = await del<PanelWriteResult>(`/menu/panels/${record.panel_id}`)
      toast(res.snapshot ? `已删除, 内容留在 ${res.snapshot}, 可在快照里回滚` : '已删除')
      await refresh()
    } catch (err) {
      toast(err instanceof Error ? err.message : '删除失败', 'err')
    } finally {
      setPending('')
      setConfirm(null)
    }
  }

  const runRestore = async (snapshot: MenuSnapshot) => {
    setPending(snapshot.file)
    try {
      const res = await post<PanelRestoreResult>(`/menu/snapshots/${snapshot.file}/restore`)
      toast(
        res.mode === 'recreated'
          ? `面板已被删过, 已按快照重建, 新 id ${res.panel_id}`
          : `已回滚 ${res.panel_id}`,
      )
      await refresh()
    } catch (err) {
      toast(err instanceof Error ? err.message : '回滚失败', 'err')
    } finally {
      setPending('')
      setConfirm(null)
    }
  }

  const runSync = async (source: PanelRecord, targets: PanelSyncTarget[]) => {
    setSaving(true)
    try {
      const res = await post<PanelSyncResult>('/menu/sync', { from: source.panel_id, targets })
      await refresh()
      return res
    } catch (err) {
      toast(err instanceof Error ? err.message : '同步失败', 'err')
      return null
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <PageHeader
        title="指令面板"
        description={
          limits
            ? `用户在输入框旁看到的命令菜单, 存在 QQ 服务端。面板 ${total}/${maxPerBot} 个(四个场景合计), 每个面板最多 ${limits.max_items} 条。`
            : '用户在输入框旁看到的命令菜单, 存在 QQ 服务端。'
        }
        actions={
          <div className="flex items-center gap-2">
            <Button variant="secondary" icon={History} onClick={() => setSnapshotsOpen(true)}>
              快照
            </Button>
            <Tooltip
              label={
                atLimit
                  ? `面板总数已达上限 ${maxPerBot}(四个场景合计), 先删掉不用的`
                  : `在「${scopeTabs.find((tab) => tab.id === scope)?.label ?? scope}」下新建面板`
              }
            >
              <Button
                icon={Plus}
                disabled={atLimit || !limits}
                onClick={() =>
                  setDraft({ scope, record: null, items: [emptyItem()], remark: '' })
                }
              >
                新建面板
              </Button>
            </Tooltip>
          </div>
        }
      />

      <div className="mt-5 flex flex-col gap-4">
        <DangerNote>
          面板住在 QQ 服务端: 保存是整份覆盖, 删除不可撤销, 而且立即对所有用户生效。每次改动前后端都会自动留一份快照,
          误操作可以回滚。
        </DangerNote>

        <Tabs tabs={scopeTabs} value={scope} onChange={(id) => setScope(id as PanelScope)} />

        {panels.isPending && !panels.data ? (
          <SkeletonRows rows={4} cols={6} />
        ) : panels.isError && !panels.data ? (
          <EmptyState
            title="读取面板失败"
            description={panels.error instanceof Error ? panels.error.message : '请稍后重试'}
          />
        ) : !current || current.panels.length === 0 ? (
          <EmptyState
            title={`「${current?.label ?? scope}」还没有面板`}
            description="这个场景下用户不会看到任何命令菜单。点右上角新建一个。"
          />
        ) : (
          <Panel>
            <Table>
              <thead>
                <tr>
                  <Th>备注</Th>
                  <Th>面板 ID</Th>
                  <Th>条目</Th>
                  <Th>版本</Th>
                  <Th>更新时间</Th>
                  <Th>操作</Th>
                </tr>
              </thead>
              <tbody>
                {current.panels.map((record) => {
                  const editable = isEditable(record)
                  return (
                    <tr key={record.panel_id}>
                      <Td>
                        <span className="flex flex-wrap items-center gap-2">
                          <span>{record.panel.remark || '—'}</span>
                          {!editable && (
                            <Tooltip label="只对指定群或指定用户生效, 暂不支持在这里编辑">
                              <Badge tone="warn">指定对象 · 只读</Badge>
                            </Tooltip>
                          )}
                          {record.panel.items.length >= (limits?.max_items ?? 20) && <Badge tone="warn">已满</Badge>}
                        </span>
                      </Td>
                      <Td className="font-mono text-xs text-ink-soft">{record.panel_id}</Td>
                      <Td className="text-xs tabular-nums">
                        {record.panel.items.length}/{limits?.max_items ?? 20}
                      </Td>
                      <Td className="text-xs tabular-nums">v{record.version}</Td>
                      <Td className="whitespace-nowrap text-xs text-ink-soft">
                        {record.updated_at ? fmtTime(Date.parse(record.updated_at)) : '—'}
                      </Td>
                      <Td>
                        <span className="flex items-center gap-2">
                          <Button
                            variant="secondary"
                            size="sm"
                            icon={Pencil}
                            disabled={!editable || !limits}
                            onClick={() =>
                              setDraft({
                                scope: record.scope,
                                record,
                                items: record.panel.items.map((item) => ({ ...item })),
                                remark: record.panel.remark ?? '',
                              })
                            }
                          >
                            编辑
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            icon={Copy}
                            disabled={!editable || !limits}
                            onClick={() => setSyncSource(record)}
                          >
                            同步
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            icon={Trash2}
                            disabled={!editable}
                            loading={pending === record.panel_id}
                            onClick={() => setConfirm({ kind: 'delete', record })}
                          >
                            删除
                          </Button>
                        </span>
                      </Td>
                    </tr>
                  )
                })}
              </tbody>
            </Table>
          </Panel>
        )}
      </div>

      {draft && limits && (
        <PanelEditor
          draft={draft}
          limits={limits}
          saving={saving}
          onChange={setDraft}
          onCancel={() => setDraft(null)}
          onSave={() => setConfirm({ kind: 'save', draft })}
        />
      )}

      <SnapshotsModal
        open={snapshotsOpen}
        onClose={() => setSnapshotsOpen(false)}
        pending={pending}
        onRestore={(snapshot) => setConfirm({ kind: 'restore', snapshot })}
      />

      <ConfirmDialog
        open={confirm?.kind === 'save'}
        onClose={() => setConfirm(null)}
        onConfirm={() => confirm?.kind === 'save' && runSave(confirm.draft)}
        title="整份覆盖服务端面板"
        description={
          confirm?.kind === 'save' ? (
            <>
              将用 {confirm.draft.items.length} 条指令项覆盖
              <code className="code-block mx-1 inline-block px-2 py-0.5">
                {confirm.draft.record?.panel_id ?? '新面板'}
              </code>
              ，改动前的内容会自动留成快照。保存后立即对所有用户生效。
            </>
          ) : undefined
        }
        confirmLabel="覆盖保存"
        danger
        pending={saving}
      />

      <ConfirmDialog
        open={confirm?.kind === 'delete'}
        onClose={() => setConfirm(null)}
        onConfirm={() => confirm?.kind === 'delete' && runDelete(confirm.record)}
        title="删除指令面板"
        description={
          confirm?.kind === 'delete' ? (
            <>
              将删除 <code className="code-block mx-1 inline-block px-2 py-0.5">{confirm.record.panel_id}</code>
              （{confirm.record.panel.remark || '无备注'}，{confirm.record.panel.items.length} 条）。服务端删除不可撤销,
              但内容会留在快照里, 之后可以重建。
            </>
          ) : undefined
        }
        confirmLabel="删除"
        requireText="删除"
        danger
        pending={pending !== ''}
      />

      <ConfirmDialog
        open={confirm?.kind === 'restore'}
        onClose={() => setConfirm(null)}
        onConfirm={() => confirm?.kind === 'restore' && runRestore(confirm.snapshot)}
        title="回滚到这个快照"
        description={
          confirm?.kind === 'restore' ? (
            <>
              把 {fmtTime(Date.parse(confirm.snapshot.saved_at))} 的版本写回
              <code className="code-block mx-1 inline-block px-2 py-0.5">{confirm.snapshot.panel_id}</code>
              。面板若已被删除, 会按快照重建并拿到新的 id。
            </>
          ) : undefined
        }
        confirmLabel="回滚"
        danger
        pending={pending !== ''}
      />

      {syncSource && (
        <SyncModal
          source={syncSource}
          scopes={panels.data?.scopes ?? []}
          total={total}
          maxPerBot={maxPerBot}
          saving={saving}
          onClose={() => setSyncSource(null)}
          onSync={(targets) => runSync(syncSource, targets)}
        />
      )}

      {saving && (
        <span className="sr-only" role="status">
          <Spinner /> 保存中
        </span>
      )}
    </>
  )
}
