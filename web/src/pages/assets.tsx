import { createSignal, For, Show } from 'solid-js'
import { api, AssetsView } from '../api'
import { Badge, Button, Card, SkeletonRows, Switch, toast } from '../ui'

export default function AssetsPage() {
  type Provider = AssetsView['providers'][number]
  const [view, setView] = createSignal<AssetsView | null>(null)
  const [whitelist, setWhitelist] = createSignal('')
  const [providers, setProviders] = createSignal<Provider[]>([])
  const [busy, setBusy] = createSignal(false)

  const load = async () => {
    try {
      const v = await api<AssetsView>('/api/assets')
      setView(v)
      setWhitelist((v.whitelist ?? []).join('\n'))
      setProviders(v.providers)
    } catch (err) {
      toast((err as Error).message, 'err')
    }
  }
  load()

  const patchProvider = (name: string, patch: Partial<Provider>) =>
    setProviders((list) => list.map((p) => (p.name === name ? { ...p, ...patch } : p)))

  const patchConfig = (name: string, key: string, v: unknown) =>
    setProviders((list) => list.map((p) => (p.name === name ? { ...p, config: { ...p.config, [key]: v } } : p)))

  const save = async () => {
    setBusy(true)
    try {
      const body = {
        whitelist: whitelist().split(/\r?\n/).map((s) => s.trim()).filter(Boolean),
        providers: providers().map((p) => ({
          name: p.name,
          enabled: p.enabled,
          priority: p.priority,
          config: p.config,
        })),
      }
      await api('/api/assets', { method: 'PUT', body: JSON.stringify(body) })
      toast('图床配置已保存并即时生效')
      load()
    } catch (err) {
      toast((err as Error).message, 'err')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Show when={view()} fallback={<SkeletonRows rows={6} />}>
      <div class="px-rise">
        <h2 class="mb-6 font-display text-[26px] font-semibold leading-tight tracking-tight text-foreground">图床</h2>

        <Card title="白名单域名前缀" actions={<span class="text-xs text-muted-foreground-2">命中的图片 URL 直通，不经过图床上传</span>}>
          <div class="px-6 py-5">
            <textarea
              class="w-full rounded-[1.4rem] border border-line-3 bg-field px-4 py-3 font-mono text-xs leading-relaxed text-foreground outline-none transition-all duration-300 placeholder:text-muted-foreground-2 focus:border-primary-600 focus:ring-2 focus:ring-primary-500/30"
              rows={4}
              placeholder={'每行一个域名前缀，如 https://q.qlogo.cn'}
              value={whitelist()}
              onInput={(e) => setWhitelist(e.currentTarget.value)}
            />
          </div>
        </Card>

        <Card
          title="Provider"
          actions={<Button variant="primary" onClick={save} disabled={busy()}>{busy() ? '保存中…' : '保存全部'}</Button>}
        >
          <div class="grid grid-cols-1 gap-5 p-5 md:grid-cols-2">
            <For each={providers()}>
              {(p) => (
                <div class={`px-blob flex flex-col gap-3 border border-line-2 bg-card p-5 shadow-sm transition-all duration-500 hover:border-primary-500/30 ${p.enabled ? '' : 'opacity-60'}`}>
                  <div class="flex items-center justify-between gap-2">
                    <div class="flex min-w-0 items-center gap-2">
                      <b class="truncate text-[13.5px] font-semibold text-foreground">{p.name}</b>
                      <Badge tone={p.enabled ? 'ok' : 'dim'}>{p.enabled ? '启用' : '禁用'}</Badge>
                      {p.has_secrets && <Badge tone="accent">密钥已配置</Badge>}
                    </div>
                    <Switch checked={p.enabled} onChange={(v) => patchProvider(p.name, { enabled: v })} />
                  </div>
                  <div class="flex items-center gap-2 text-[12.5px] text-muted-foreground">
                    <span>优先级</span>
                    <button
                      class="inline-grid h-7 w-7 cursor-pointer place-items-center rounded-full border border-line-3 bg-card text-muted-foreground transition-all duration-300 hover:border-line-4 hover:text-foreground active:translate-y-px"
                      onClick={() => patchProvider(p.name, { priority: Math.max(0, p.priority - 10) })}
                    >
                      −
                    </button>
                    <input
                      class="h-7 w-16 rounded-full border border-line-3 bg-field text-center font-mono text-xs text-foreground outline-none transition-colors duration-300 focus:border-primary-600 focus:ring-2 focus:ring-primary-500/30"
                      type="number"
                      value={p.priority}
                      onInput={(e) => patchProvider(p.name, { priority: Number(e.currentTarget.value) || 0 })}
                    />
                    <button
                      class="inline-grid h-7 w-7 cursor-pointer place-items-center rounded-full border border-line-3 bg-card text-muted-foreground transition-all duration-300 hover:border-line-4 hover:text-foreground active:translate-y-px"
                      onClick={() => patchProvider(p.name, { priority: p.priority + 10 })}
                    >
                      +
                    </button>
                  </div>
                  <For each={p.schema}>
                    {(f) => (
                      <label class="flex flex-col gap-1.5 text-[12.5px] text-muted-foreground">
                        <span>
                          {f.label}
                          {f.required ? ' *' : ''}
                        </span>
                        <SchemaInput field={f} value={p.config[f.key]} onChange={(v) => patchConfig(p.name, f.key, v)} />
                      </label>
                    )}
                  </For>
                </div>
              )}
            </For>
          </div>
        </Card>
      </div>
    </Show>
  )
}

function SchemaInput(props: { field: { key: string; label: string; description?: string; type: string; required?: boolean; default?: unknown }; value: unknown; onChange: (v: unknown) => void }) {
  const f = props.field
  const inputCls =
    'h-10 w-full rounded-full border border-line-3 bg-field px-5 text-[13px] text-foreground outline-none transition-all duration-300 placeholder:text-muted-foreground-2 focus:border-primary-600 focus:ring-2 focus:ring-primary-500/30'
  if (f.type === 'boolean') {
    return <Switch checked={!!props.value} onChange={props.onChange} />
  }
  if (f.type === 'number') {
    return (
      <input
        class={inputCls}
        type="number"
        placeholder={f.description || ''}
        value={props.value == null ? String(f.default ?? '') : String(props.value)}
        onInput={(e) => props.onChange(Number(e.currentTarget.value))}
      />
    )
  }
  return (
    <input
      class={inputCls}
      type={f.type === 'password' ? 'password' : 'text'}
      placeholder={f.type === 'password' ? (props.value ? '已配置，留空不修改' : f.required ? '必填' : '选填') : f.description || ''}
      value={f.type === 'password' ? '' : String(props.value ?? '')}
      onInput={(e) => props.onChange(e.currentTarget.value)}
    />
  )
}