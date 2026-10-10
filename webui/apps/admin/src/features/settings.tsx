import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  FieldList,
  FieldNumber,
  HotBadge,
  Input,
  MultiSelect,
  Panel,
  Select,
  SkeletonRows,
  Switch,
  Textarea,
} from '@aurorix/webui'
import { RotateCw, Save, TriangleAlert } from 'lucide-react'
import { PageHeader } from '../components/layout'
import { get, post, put } from '../lib/api'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { ConfigSaveResult, ConfigView, CoreField, Overview } from '../lib/types'

function toList(value: unknown): string[] {
  return Array.isArray(value) ? value.map((item) => String(item)) : []
}

function widget(field: CoreField, value: unknown, onChange: (next: unknown) => void) {
  switch (field.kind) {
    case 'bool':
      return <Switch checked={value === true} onChange={onChange} label={field.label} />
    case 'secret':
      return (
        <Input
          type="password"
          value={typeof value === 'string' ? value : ''}
          placeholder={field.set ? '已配置，留空保持原值' : '未配置'}
          autoComplete="off"
          onChange={(e) => onChange(e.target.value)}
        />
      )
    case 'number':
      return <FieldNumber value={typeof value === 'number' ? value : null} onChange={onChange} />
    case 'select':
      return (
        <Select
          value={String(value ?? '')}
          options={(field.options ?? []).map((option) => ({ value: option, label: option }))}
          onChange={(e) => onChange(e.target.value)}
        />
      )
    case 'multiselect':
      return <MultiSelect options={field.options ?? []} value={toList(value)} onChange={onChange} />
    case 'textlist':
      // 多行自由编辑: 数组 ↔ 按行拆分的文本; 空行交给后端 trim 后丢弃
      return (
        <Textarea
          rows={6}
          className="font-mono text-sm"
          placeholder={field.placeholder}
          value={toList(value).join('\n')}
          onChange={(e) => onChange(e.target.value.split('\n'))}
        />
      )
    case 'intlist':
    case 'strlist':
      return <FieldList value={toList(value)} onChange={onChange} />
    default:
      return (
        <Input value={typeof value === 'string' ? value : ''} onChange={(e) => onChange(e.target.value)} />
      )
  }
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
  const [draft, setDraft] = useState<Record<string, unknown>>({})
  const [result, setResult] = useState<ConfigSaveResult | null>(null)
  const [saving, setSaving] = useState(false)

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
  const hot = fields.filter((field) => field.hot && !field.restart)
  const restart = fields.filter((field) => field.restart)
  const other = fields.filter((field) => !field.hot && !field.restart)

  const renderGroup = (title: string, description: string, group: CoreField[]) => {
    if (group.length === 0) return null
    return (
      <Panel key={title} title={title} description={description}>
        <div className="grid gap-5 md:grid-cols-2">
          {group.map((field) => (
            <Field
              key={field.key}
              label={field.label}
              htmlFor={`core-${field.key}`}
              hint={field.desc}
              badge={<HotBadge hot={field.hot} />}
              className={field.kind === 'multiselect' || field.kind === 'textlist' ? 'md:col-span-2' : undefined}
            >
              {widget(
                field,
                field.key in draft ? draft[field.key] : field.value,
                (next) => setDraft((prev) => ({ ...prev, [field.key]: next })),
              )}
            </Field>
          ))}
        </div>
      </Panel>
    )
  }

  const changed = Object.keys(draft).length

  return (
    <>
      <PageHeader
        title="设置"
        description="核心配置热更与需重启项分离，改动按稀疏补丁提交"
        actions={
          <>
            {changed > 0 && <Badge tone="warn">{changed} 项待保存</Badge>}
            <Button icon={Save} loading={saving} onClick={save} disabled={!data || changed === 0}>
              保存
            </Button>
          </>
        }
      />

      {result?.restart_needed && (
        <p className="flex items-start gap-2 rounded-[1.5rem] border border-warn/40 bg-warn/10 p-4 text-sm text-ink-soft">
          <TriangleAlert aria-hidden className="mt-0.5 size-4 shrink-0 text-warn-text" />
          <span>
            已保存。以下项需要重启进程才会生效：
            <span className="font-mono text-xs"> {(result.restart_fields ?? []).join('、') || '—'}</span>
          </span>
        </p>
      )}

      {isPending && !data ? (
        <SkeletonRows rows={8} cols={2} />
      ) : isError && !data ? (
        <EmptyState title="读取配置失败" description={error instanceof Error ? error.message : '请稍后重试'} />
      ) : (
        <>
          {renderGroup('保存即生效', '改完点保存立刻热更，无需重启', hot)}
          {renderGroup('需重启生效', '改完需重启进程才会应用', restart)}
          {renderGroup('其他', '未标注生效方式', other)}
        </>
      )}

      <Panel
        title="进程控制"
        description="改了「需重启生效」的项后在这里重启；重启会中断当前会话，需要重新登录"
      >
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
      </Panel>

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
    </>
  )
}
