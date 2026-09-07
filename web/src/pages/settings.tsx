import { createEffect, createSignal, For, onCleanup, Show } from 'solid-js'
import { api, CoreField } from '../api'
import { Button, Card, Field, Icon, Modal, MultiSelect, Select, SkeletonRows, Switch, toast } from '../ui'

export default function SettingsPage() {
  const [fields, setFields] = createSignal<CoreField[] | null>(null)
  const [busy, setBusy] = createSignal(false)
  const [restartNotice, setRestartNotice] = createSignal<string[] | null>(null)
  const [overlay, setOverlay] = createSignal<null | { kind: 'restart' | 'stop'; text: string }>(null)

  const load = async () => {
    try {
      const v = await api<{ fields: CoreField[] }>('/api/config')
      setFields(v.fields)
    } catch (err) {
      toast((err as Error).message, 'err')
    }
  }
  load()

  const [values, setValues] = createSignal<Record<string, unknown>>({})
  const [dirty, setDirty] = createSignal<Record<string, boolean>>({})

  const set = (key: string, v: unknown) => {
    setValues((cur) => ({ ...cur, [key]: v }))
    setDirty((cur) => ({ ...cur, [key]: true }))
  }

  const editable = (f: CoreField) => f.kind !== 'note'

  const display = (f: CoreField): unknown => {
    if (f.key in values()) return values()[f.key]
    return f.value
  }

  // ---------- 保存 ----------

  const toPayload = (f: CoreField): unknown => {
    const v = display(f)
    switch (f.kind) {
      case 'intlist':
        return String(v ?? '')
          .split(/\r?\n/)
          .map((s) => s.trim())
          .filter(Boolean)
      case 'strlist':
        return String(v ?? '')
          .split(/\r?\n/)
          .map((s) => s.trim())
          .filter(Boolean)
      case 'multiselect':
        return Array.isArray(v) ? (v as string[]) : []
      case 'number':
        return v === '' ? undefined : Number(v)
      default:
        return v
    }
  }

  const save = async (hotOnly: boolean) => {
    const group = fields()!.filter((f) => editable(f) && f.hot === hotOnly && dirty()[f.key])
    if (!group.length) {
      toast(hotOnly ? '没有需要保存的即时生效项' : '没有需要保存的重启生效项', 'info')
      return
    }
    setBusy(true)
    try {
      const payload: Record<string, unknown> = {}
      for (const f of group) {
        const v = toPayload(f)
        if (v !== undefined && !(f.kind === 'secret' && v === '')) payload[f.key] = v
      }
      const res = await api<{ restart_needed: boolean; restart_fields?: string[] }>('/api/config', {
        method: 'PUT',
        body: JSON.stringify({ values: payload }),
      })
      const touched = group.map((f) => f.label)
      if (res.restart_needed) {
        setRestartNotice(res.restart_fields ?? [])
        toast(`已保存：${touched.join('、')} 需重启生效`, 'info')
      } else {
        toast(`已保存并即时生效：${touched.join('、')}`)
      }
      setDirty({})
      load()
    } catch (err) {
      toast((err as Error).message, 'err')
    } finally {
      setBusy(false)
    }
  }

  // ---------- 重启/停止 ----------

  const [confirm, setConfirm] = createSignal<null | { kind: 'restart' | 'stop' }>(null)
  const [countdown, setCountdown] = createSignal(0)

  createEffect(() => {
    if (!confirm()) return
    setCountdown(3)
    const t = setInterval(() => setCountdown((n) => (n > 0 ? n - 1 : 0)), 1000)
    onCleanup(() => clearInterval(t))
  })

  const openConfirm = (kind: 'restart' | 'stop') => {
    setConfirm({ kind })
    setCountdown(3)
  }

  const run = async (kind: 'restart' | 'stop') => {
    setConfirm(null)
    setOverlay({ kind, text: kind === 'restart' ? '正在重启服务…' : '正在停止服务…' })
    try {
      await api(`/api/system/${kind}`, { method: 'POST', body: '{}' })
    } catch (err) {
      setOverlay(null)
      toast((err as Error).message, 'err')
      return
    }
    if (kind === 'restart') {
      // 轮询等待新进程就绪后刷新
      const iv = setInterval(async () => {
        try {
          await api('/api/me')
          clearInterval(iv)
          location.reload()
        } catch {
          /* 进程尚未就绪 */
        }
      }, 2000)
      setTimeout(() => clearInterval(iv), 120000)
      onCleanup(() => clearInterval(iv))
    } else {
      setOverlay({ kind, text: '服务已停止。请手动重新启动进程。' })
    }
  }

  return (
    <Show when={fields()} fallback={<SkeletonRows rows={8} />}>
      <div class="px-rise">
        <h2 class="mb-6 font-display text-[26px] font-semibold leading-tight tracking-tight text-foreground">设置</h2>

        <Show when={restartNotice()}>
          <div class="mb-5 flex items-center gap-2.5 rounded-full border border-amber-300/50 bg-amber-50 px-5 py-3 text-[13px] text-amber-800 dark:border-amber-400/20 dark:bg-amber-400/10 dark:text-amber-300">
            <Icon name="refresh" size={16} class="shrink-0" />
            <span>已保存：{restartNotice()!.length} 项参数需重启后生效</span>
            <Button variant="primary" onClick={() => openConfirm('restart')} class="ml-auto">立即重启</Button>
          </div>
        </Show>

        <Card
          title="即时生效"
          actions={<Button variant="primary" disabled={busy()} onClick={() => save(true)}>{busy() ? '保存中…' : '保存'}</Button>}
        >
          <div>
            <For each={fields()!.filter((f) => editable(f) && f.hot)}>
              {(f) => (
                <Field label={f.label} desc={f.desc}>
                  <FieldControl field={f} value={display(f)} onChange={(v) => set(f.key, v)} />
                </Field>
              )}
            </For>
          </div>
        </Card>

        <Card
          title="需重启生效"
          actions={<Button disabled={busy()} onClick={() => save(false)}>{busy() ? '保存中…' : '保存'}</Button>}
        >
          <div>
            <For each={fields()!.filter((f) => editable(f) && !f.hot)}>
              {(f) => (
                <Field label={f.label} desc={f.desc}>
                  <FieldControl field={f} value={display(f)} onChange={(v) => set(f.key, v)} />
                </Field>
              )}
            </For>
            <For each={fields()!.filter((f) => f.kind === 'note')}>
              {(f) => (
                <Field label={f.label} desc={f.desc}>
                  <pre class="max-h-[260px] w-full overflow-auto whitespace-pre-wrap break-all rounded-[1.4rem] border border-dashed border-line-4 bg-field p-4 font-mono text-xs leading-relaxed text-muted-foreground">
                    {JSON.stringify(f.value, null, 2)}
                  </pre>
                </Field>
              )}
            </For>
          </div>
        </Card>

        <Card title="危险操作">
          <div class="flex items-center justify-between gap-4 border-b border-card-divider px-6 py-4 last:border-b-0">
            <div>
              <b class="text-[13.5px] font-semibold text-foreground">重启服务</b>
              <p class="mt-0.5 max-w-[620px] text-xs text-muted-foreground-2">优雅关闭后自动重新拉起进程（PID 会变化）。由 systemd 等守护管理时请设置 AURORIX_SUPERVISED=1</p>
            </div>
            <Button variant="danger" onClick={() => openConfirm('restart')}>
              <Icon name="refresh" size={15} /> 重启
            </Button>
          </div>
          <div class="flex items-center justify-between gap-4 border-t border-card-divider px-6 py-4">
            <div>
              <b class="text-[13.5px] font-semibold text-foreground">停止服务</b>
              <p class="mt-0.5 text-xs text-muted-foreground-2">退出进程，需要你手动再次启动</p>
            </div>
            <Button variant="danger" onClick={() => openConfirm('stop')}>
              <Icon name="power" size={15} /> 停止
            </Button>
          </div>
        </Card>
      </div>

      <Modal
        open={!!confirm()}
        title={confirm()?.kind === 'restart' ? '确认重启' : '确认停止'}
        onClose={() => setConfirm(null)}
        footer={
          <>
            <Button onClick={() => setConfirm(null)}>取消</Button>
            <Button
              variant="danger"
              disabled={countdown() > 0}
              onClick={() => run(confirm()!.kind)}
            >
              {countdown() > 0 ? `${confirm()?.kind === 'restart' ? '重启' : '停止'} (${countdown()})` : `确认${confirm()?.kind === 'restart' ? '重启' : '停止'}`}
            </Button>
          </>
        }
      >
        <p>
          {confirm()?.kind === 'restart'
            ? '将优雅关闭当前进程并以相同参数重新拉起。执行期间请求会短暂中断。'
            : '将优雅关闭当前进程。执行后管理台将不可访问。'}
        </p>
      </Modal>

      <Show when={overlay()}>
        <div class="px-fade fixed inset-0 z-[95] flex flex-col items-center justify-center gap-4 bg-background text-muted-foreground">
          <span class="h-8 w-8 animate-spin rounded-full border-[3px] border-line-5 border-t-primary-600" />
          <b class="text-sm font-medium text-foreground">{overlay()!.text}</b>
        </div>
      </Show>
    </Show>
  )
}

function FieldControl(props: { field: CoreField; value: unknown; onChange: (v: unknown) => void }) {
  const f = props.field
  if (f.kind === 'bool') {
    return <Switch checked={!!props.value} onChange={props.onChange} />
  }
  if (f.kind === 'multiselect') {
    const selected = (Array.isArray(props.value) ? props.value : []) as string[]
    return <MultiSelect options={f.options ?? []} values={selected} onChange={(v) => props.onChange(v)} />
  }
  if (f.kind === 'select') {
    return (
      <Select
        options={f.options ?? []}
        value={props.value == null ? undefined : String(props.value)}
        onChange={(v) => props.onChange(v)}
      />
    )
  }
  if (f.kind === 'intlist' || f.kind === 'strlist') {
    const lines = Array.isArray(props.value) ? props.value.join('\n') : ''
    return (
      <textarea
        class="w-full rounded-[1.4rem] border border-line-3 bg-field px-4 py-3 font-mono text-xs leading-relaxed text-foreground outline-none transition-all duration-300 placeholder:text-muted-foreground-2 focus:border-primary-600 focus:ring-2 focus:ring-primary-500/30"
        rows={4}
        value={lines}
        placeholder={f.kind === 'intlist' ? '每行一个数值' : '每行一个事件名'}
        onInput={(e) => props.onChange(e.currentTarget.value)}
      />
    )
  }
  if (f.kind === 'secret') {
    return (
      <input
        class="h-10 w-full max-w-[480px] rounded-full border border-line-3 bg-field px-5 text-[13px] text-foreground outline-none transition-all duration-300 placeholder:text-muted-foreground-2 focus:border-primary-600 focus:ring-2 focus:ring-primary-500/30"
        type="password"
        autocomplete="new-password"
        placeholder={f.set ? '已配置，留空则保留' : '未配置'}
        onInput={(e) => props.onChange(e.currentTarget.value)}
      />
    )
  }
  return (
    <input
      class="h-10 w-full max-w-[480px] rounded-full border border-line-3 bg-field px-5 text-[13px] text-foreground outline-none transition-all duration-300 placeholder:text-muted-foreground-2 focus:border-primary-600 focus:ring-2 focus:ring-primary-500/30"
      type={f.kind === 'number' ? 'number' : 'text'}
      value={props.value == null ? '' : String(props.value)}
      onInput={(e) => props.onChange(e.currentTarget.value)}
    />
  )
}