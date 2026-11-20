import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Badge,
  Button,
  ConfirmDialog,
  DangerNote,
  EmptyState,
  FieldList,
  FieldNumber,
  HotBadge,
  Input,
  MultiSelect,
  OpsPage,
  OpsSection,
  OpsToolbar,
  Select,
  SkeletonRows,
  Switch,
  Textarea,
  cn,
} from '@aurorix/webui'
import { RotateCw, Save, Search } from 'lucide-react'
import { get, post, put } from '../lib/api'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { ConfigGroup, ConfigSaveResult, ConfigView, CoreField, Overview } from '../lib/types'

type Draft = Record<string, unknown>
type GroupSlice = ConfigGroup & { fields: CoreField[] }

const OTHER_GROUP: ConfigGroup = { id: '__other__', label: '其他', desc: '后端没有标注分组的字段' }

function toList(value: unknown): string[] {
  return Array.isArray(value) ? value.map((item) => String(item)) : []
}

function noteText(field: CoreField): string {
  const raw = typeof field.value === 'string' ? field.value.trim() : ''
  return raw || field.desc || '—'
}

function controlId(field: CoreField): string | undefined {
  return field.kind === 'multiselect' ? undefined : `core-${field.key}`
}

function widget(field: CoreField, value: unknown, onChange: (next: unknown) => void): ReactNode {
  const id = controlId(field)
  switch (field.kind) {
    case 'note':
      return <p className="max-w-[68ch] text-xs text-ink-soft">{noteText(field)}</p>
    case 'bool':
      return <Switch id={id} checked={value === true} onChange={onChange} label={field.label} />
    case 'secret':
      return (
        <Input
          id={id}
          type="password"
          value={typeof value === 'string' ? value : ''}
          placeholder={field.set ? '已配置，留空保持原值' : '未配置，填写后保存'}
          autoComplete="off"
          spellCheck={false}
          onChange={(e) => onChange(e.target.value)}
        />
      )
    case 'number':
      return (
        <FieldNumber
          id={id}
          value={typeof value === 'number' ? value : null}
          onChange={onChange}
          placeholder={field.placeholder}
        />
      )
    case 'select':
      return (
        <Select
          id={id}
          value={String(value ?? '')}
          options={(field.options ?? []).map((option) => ({
            value: option,
            label: option === '' ? '默认（不发该字段）' : option,
          }))}
          onChange={(e) => onChange(e.target.value)}
        />
      )
    case 'multiselect':
      return <MultiSelect options={field.options ?? []} value={toList(value)} onChange={onChange} />
    case 'textarea':
      return (
        <Textarea
          id={id}
          rows={6}
          className="font-mono text-sm"
          placeholder={field.placeholder}
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value)}
        />
      )
    case 'textlist':
      return (
        <Textarea
          id={id}
          rows={6}
          className="font-mono text-sm"
          placeholder={field.placeholder}
          value={toList(value).join('\n')}
          onChange={(e) => onChange(e.target.value.split('\n'))}
        />
      )
    case 'intlist':
    case 'strlist':
      return <FieldList id={id} value={toList(value)} onChange={onChange} />
    default:
      return (
        <Input
          id={id}
          value={typeof value === 'string' ? value : ''}
          placeholder={field.placeholder}
          spellCheck={false}
          onChange={(e) => onChange(e.target.value)}
        />
      )
  }
}

function sliceGroups(fields: CoreField[], groups: ConfigGroup[]): GroupSlice[] {
  const known = new Set(groups.map((group) => group.id))
  const buckets = new Map<string, CoreField[]>()
  for (const group of groups) buckets.set(group.id, [])
  const orphan: CoreField[] = []
  for (const field of fields) {
    if (field.group && known.has(field.group)) buckets.get(field.group)?.push(field)
    else orphan.push(field)
  }
  const slices = groups.map((group) => ({ ...group, fields: buckets.get(group.id) ?? [] }))
  if (orphan.length > 0) slices.push({ ...OTHER_GROUP, fields: orphan })
  return slices
}

function matched(field: CoreField, keyword: string): boolean {
  if (keyword === '') return true
  return [field.key, field.label, field.desc ?? ''].some((text) => text.toLowerCase().includes(keyword))
}

function FieldRow({
  field,
  value,
  onChange,
}: {
  field: CoreField
  value: unknown
  onChange: (next: unknown) => void
}) {
  if (field.kind === 'note') {
    return (
      <div className="border-b border-line py-3 last:border-b-0">
        <p className="text-sm font-medium text-ink">{field.label}</p>
        {widget(field, value, onChange)}
      </div>
    )
  }

  return (
    <div className="grid gap-3 border-b border-line py-3 last:border-b-0 md:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <label htmlFor={controlId(field)} className="text-sm font-medium text-ink">
            {field.label}
          </label>
          <HotBadge hot={field.hot} />
        </div>
        {field.desc && (
          <p className="mt-1 line-clamp-2 text-xs text-ink-soft" title={field.desc}>
            {field.desc}
          </p>
        )}
      </div>
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <div className="min-w-0 max-w-md flex-1">{widget(field, value, onChange)}</div>
        {field.unit && <span className="shrink-0 text-xs text-ink-faint">{field.unit}</span>}
      </div>
    </div>
  )
}

const RESTART_POLL_MS = 500
const RESTART_TIMEOUT_MS = 30_000

async function waitRestart(timeoutMs = RESTART_TIMEOUT_MS): Promise<boolean> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, RESTART_POLL_MS))
    try {
      const res = await fetch('/admin/api/me', { credentials: 'same-origin' })
      if (res.status === 401) return true
    } catch {
    }
  }
  return false
}

export default function SettingsPage() {
  const [draft, setDraft] = useState<Draft>({})
  const [result, setResult] = useState<ConfigSaveResult | null>(null)
  const [saving, setSaving] = useState(false)
  const [query, setQuery] = useState('')
  const [activeId, setActiveId] = useState('')

  const { data, isPending, isError, error } = useQuery({
    queryKey: qk.config,
    queryFn: () => get<ConfigView>('/config'),
  })

  const { data: overview } = useQuery({
    queryKey: qk.overview,
    queryFn: () => get<Overview>('/overview'),
  })
  const supervised = overview?.control?.supervised ?? false
  const [confirmRestart, setConfirmRestart] = useState(false)
  const [restarting, setRestarting] = useState(false)

  const doRestart = async () => {
    setRestarting(true)
    try {
      await post('/system/restart')
    } catch (err) {
      setRestarting(false)
      setConfirmRestart(false)
      toast(err instanceof Error ? err.message : '重启请求失败', 'err')
      return
    }
    setConfirmRestart(false)
    if (!(await waitRestart())) {
      setRestarting(false)
      toast('重启超时，进程可能没起来，请 SSH 检查', 'err')
      return
    }
    window.location.reload()
  }

  const save = async () => {
    if (Object.keys(draft).length === 0) {
      toast('没有需要保存的改动', 'info')
      return
    }
    setSaving(true)
    try {
      const saved = await put<ConfigSaveResult>('/config', { values: draft })
      setDraft({})
      setResult(saved)
      await queryClient.invalidateQueries({ queryKey: qk.config })
      toast(saved.restart_needed ? '已保存，部分项需重启生效' : '已保存并热更', saved.restart_needed ? 'info' : 'ok')
    } catch (err) {
      toast(err instanceof Error ? err.message : '保存失败', 'err')
    } finally {
      setSaving(false)
    }
  }

  const fields = data?.fields ?? []
  const slices = sliceGroups(fields, data?.groups ?? [])
  const active = slices.find((slice) => slice.id === activeId) ?? slices[0]
  const keyword = query.trim().toLowerCase()
  const searching = keyword !== ''
  const visible = searching
    ? slices
        .map((slice) => ({ ...slice, fields: slice.fields.filter((field) => matched(field, keyword)) }))
        .filter((slice) => slice.fields.length > 0)
    : active
      ? [active]
      : []
  const hits = visible.reduce((sum, slice) => sum + slice.fields.length, 0)
  const changed = Object.keys(draft).length
  const maintenanceOn =
    ('bot_maintenance' in draft ? draft.bot_maintenance : fields.find((f) => f.key === 'bot_maintenance')?.value) === true

  const applyField = (field: CoreField, next: unknown) => {
    setDraft((prev) => ({ ...prev, [field.key]: next }))
  }

  return (
    <OpsPage
      title="设置"
      description="按功能域分组；标「热」的保存即生效，标「重启」的要重启进程。改动按稀疏补丁提交，没动的字段不会被写回。"
      actions={
        <>
          {changed > 0 && <Badge tone="warn">{changed} 项待保存</Badge>}
          <Button icon={Save} loading={saving} disabled={changed === 0} onClick={save}>
            保存
          </Button>
        </>
      }
    >
      {result?.restart_needed && (
        <DangerNote>
          已保存。以下项要重启进程才生效：
          <span className="font-mono">{(result.restart_fields ?? []).join('、') || '—'}</span>
        </DangerNote>
      )}

      {isPending && !data ? (
        <SkeletonRows rows={8} cols={2} />
      ) : isError && !data ? (
        <EmptyState title="读取配置失败" description={error instanceof Error ? error.message : '请稍后重试'} />
      ) : (
        <>
          <OpsToolbar
            right={
              <span>
                {searching ? `${hits} 项命中` : `共 ${fields.length} 项 · ${slices.length} 组`}
              </span>
            }
          >
            <div className="relative w-full max-w-sm min-w-56">
              <Search
                aria-hidden
                className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-faint"
              />
              <Input
                className="pl-8"
                value={query}
                spellCheck={false}
                placeholder="搜索名称 / key / 说明"
                aria-label="搜索配置项"
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
          </OpsToolbar>

          <div className="md:grid md:grid-cols-[11rem_minmax(0,1fr)] md:items-start md:gap-8">
            <div className="mb-4 md:hidden">
              <Select
                aria-label="配置分组"
                value={active?.id ?? ''}
                options={slices.map((slice) => ({
                  value: slice.id,
                  label: `${slice.label}（${slice.fields.length}）`,
                }))}
                onChange={(e) => {
                  setActiveId(e.target.value)
                  setQuery('')
                }}
              />
            </div>

            <nav
              aria-label="配置分组"
              className="hidden md:sticky md:top-0 md:flex md:flex-col md:gap-0.5"
            >
              {slices.map((slice) => {
                const on = !searching && slice.id === active?.id
                const cold = slice.fields.filter((field) => field.restart).length
                return (
                  <Button
                    key={slice.id}
                    variant="ghost"
                    size="sm"
                    aria-current={on ? 'true' : undefined}
                    className={cn(
                      'h-auto w-full flex-col items-stretch gap-0.5 py-1.5 text-left',
                      on && 'bg-pressed font-medium text-ink',
                    )}
                    onClick={() => {
                      setActiveId(slice.id)
                      setQuery('')
                    }}
                  >
                    <span className="flex w-full items-center justify-between gap-2">
                      <span className="min-w-0 truncate">{slice.label}</span>
                      <span className="shrink-0 font-mono text-xs text-ink-faint">{slice.fields.length}</span>
                    </span>
                    {cold > 0 && <span className="text-xs text-warn-ink">{cold} 项需重启</span>}
                  </Button>
                )
              })}
            </nav>

            <section className="min-w-0">
              {visible.length === 0 ? (
                <EmptyState
                  title={searching ? '没有匹配的配置项' : '没有可配置项'}
                  description={
                    searching ? '换个关键词试试，比如「端口」「图片」或字段 key。' : '后端没有下发任何核心配置字段。'
                  }
                />
              ) : (
                visible.map((slice) => {
                  const cold = slice.fields.filter((field) => field.restart).length
                  return (
                    <div key={slice.id} className="mb-6 last:mb-0">
                      <header className="border-b border-line pb-2">
                        <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                          <h2 className="text-base">{slice.label}</h2>
                          <span className="flex items-center gap-2 text-xs text-ink-faint">
                            <span>{slice.fields.length} 项</span>
                            {cold > 0 && <span className="text-warn-ink">{cold} 项需重启</span>}
                          </span>
                        </div>
                        {slice.desc && (
                          <p className="mt-1 max-w-[68ch] text-xs text-ink-soft">{slice.desc}</p>
                        )}
                      </header>
                      {slice.id === 'maintenance' && maintenanceOn && (
                        <DangerNote className="mt-2">
                          当前处于全局维护中：机器人不会响应任何指令与事件。
                        </DangerNote>
                      )}
                      {slice.fields.map((field) => (
                        <FieldRow
                          key={field.key}
                          field={field}
                          value={field.key in draft ? draft[field.key] : field.value}
                          onChange={(next) => applyField(field, next)}
                        />
                      ))}
                    </div>
                  )
                })
              )}
            </section>
          </div>
        </>
      )}

      <OpsSection title="进程控制" hint="重启会中断当前会话，完成后需要重新登录">
        <div className="flex flex-wrap items-center gap-3">
          <Button
            icon={RotateCw}
            loading={restarting}
            disabled={!supervised || restarting}
            onClick={() => setConfirmRestart(true)}
          >
            {restarting ? '重启中…' : '重启进程'}
          </Button>
          {supervised ? (
            <Badge tone="ok">外部守护接管重启</Badge>
          ) : (
            <Badge tone="warn">未声明受外部守护</Badge>
          )}
        </div>
        {!supervised && (
          <p className="mt-3 max-w-prose text-xs text-ink-faint">
            这台进程没有声明 <span className="font-mono">AURORIX_SUPERVISED=1</span>
            ，面板重启会和 run_loop / watchdog 之类的守护各拉一个实例、抢端口互杀，所以这里禁用。
            请在守护脚本里导出该变量，或手动重启进程。
          </p>
        )}
      </OpsSection>

      <ConfirmDialog
        open={confirmRestart}
        onClose={() => setConfirmRestart(false)}
        onConfirm={doRestart}
        title="重启进程"
        description="重启会断开当前会话，完成后需要重新登录。确认现在重启？"
        confirmLabel="重启"
        danger
        pending={restarting}
      />
    </OpsPage>
  )
}
