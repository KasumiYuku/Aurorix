import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Badge,
  Button,
  Drawer,
  EmptyState,
  Field,
  FieldList,
  FieldNumber,
  Input,
  Panel,
  Select,
  SkeletonRows,
  Switch,
  Tabs,
  Textarea,
} from '@aurorix/webui'
import { ExternalLink, Package, Save } from 'lucide-react'
import { PageHeader } from '../components/layout'
import { get, put } from '../lib/api'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { AccessConfig, AccessRule, ManagedPlugin, OkResult, PluginField } from '../lib/types'

const MODE_OPTIONS = [
  { value: 'off', label: '不限制' },
  { value: 'whitelist', label: '白名单' },
  { value: 'blacklist', label: '黑名单' },
]

const EMPTY_RULE: AccessRule = { mode: 'off', users: [], groups: [] }

function AccessRuleEditor({
  rule,
  onChange,
  path,
}: {
  rule: AccessRule
  onChange: (next: AccessRule) => void
  path: string
}) {
  return (
    <div className="flex flex-col gap-3 border-b border-line/60 py-3 last:border-b-0">
      <div className="flex flex-wrap items-center gap-3">
        <span className="min-w-[10rem] font-mono text-xs text-ink-soft">{path}</span>
        <Select
          className="max-w-[10rem]"
          value={rule.mode}
          options={MODE_OPTIONS}
          onChange={(e) => onChange({ ...rule, mode: e.target.value as AccessRule['mode'] })}
        />
      </div>
      {rule.mode !== 'off' && (
        <div className="grid gap-3 md:grid-cols-2">
          <Field label="用户">
            <FieldList value={rule.users} onChange={(users) => onChange({ ...rule, users })} placeholder="用户 openid" />
          </Field>
          <Field label="群">
            <FieldList value={rule.groups} onChange={(groups) => onChange({ ...rule, groups })} placeholder="群 openid" />
          </Field>
        </div>
      )}
    </div>
  )
}

function fieldInput(field: PluginField, value: unknown, onChange: (next: unknown) => void) {
  if (field.type === 'boolean') {
    return <Switch checked={value === true} onChange={onChange} label={field.label} />
  }
  if (field.type === 'password') {
    return (
      <Input
        type="password"
        value={typeof value === 'string' ? value : ''}
        placeholder="留空保持原值"
        autoComplete="off"
        onChange={(e) => onChange(e.target.value)}
      />
    )
  }
  if (field.type === 'number') {
    return (
      <FieldNumber
        value={typeof value === 'number' ? value : null}
        onChange={onChange}
        placeholder={field.placeholder}
      />
    )
  }
  if (field.type === 'textarea') {
    return (
      <Textarea
        value={typeof value === 'string' ? value : ''}
        placeholder={field.placeholder}
        rows={5}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  }
  return (
    <Input
      value={typeof value === 'string' ? value : ''}
      placeholder={field.placeholder}
      onChange={(e) => onChange(e.target.value)}
    />
  )
}

function PluginDetail({ plugin, onClose }: { plugin: ManagedPlugin; onClose: () => void }) {
  const [tab, setTab] = useState('config')
  const [values, setValues] = useState<Record<string, unknown>>(plugin.values)
  const [access, setAccess] = useState<AccessConfig>(plugin.access)
  const [saving, setSaving] = useState('')

  useEffect(() => {
    setValues(plugin.values)
    setAccess(plugin.access)
  }, [plugin])

  const saveConfig = async () => {
    setSaving('config')
    try {
      await put<OkResult>(`/plugins/${plugin.id}`, values)
      await queryClient.invalidateQueries({ queryKey: qk.plugins })
      toast('配置已保存并热更')
    } catch (err) {
      toast(err instanceof Error ? err.message : '保存失败', 'err')
    } finally {
      setSaving('')
    }
  }

  const saveAccess = async () => {
    setSaving('access')
    try {
      const commands: Record<string, AccessRule> = {}
      for (const [path, rule] of Object.entries(access.commands)) {
        if (rule.mode !== 'off') commands[path] = rule
      }
      await put<OkResult>(`/plugins/${plugin.id}/access`, { ...access, commands })
      await queryClient.invalidateQueries({ queryKey: qk.plugins })
      toast('访问控制已保存')
    } catch (err) {
      toast(err instanceof Error ? err.message : '保存失败', 'err')
    } finally {
      setSaving('')
    }
  }

  return (
    <Drawer
      open
      onClose={onClose}
      width="lg"
      title={plugin.name || plugin.id}
      description={plugin.description || plugin.id}
      footer={
        tab === 'config' ? (
          <Button icon={Save} loading={saving === 'config'} onClick={saveConfig} disabled={plugin.fields.length === 0}>
            保存配置
          </Button>
        ) : (
          <Button icon={Save} loading={saving === 'access'} onClick={saveAccess}>
            保存访问控制
          </Button>
        )
      }
    >
      {plugin.console && (
        <a
          href={plugin.console}
          className="mb-4 flex items-center justify-between gap-3 rounded-[1.25rem] border border-line bg-subtle/60 px-4 py-3 text-sm transition-colors duration-300 hover:bg-subtle"
        >
          <span className="flex items-center gap-2">
            <ExternalLink aria-hidden className="size-4 shrink-0 text-ink-faint" />
            该插件自带管理台
          </span>
          <span className="font-mono text-xs text-accent-text">{plugin.console}</span>
        </a>
      )}

      <Tabs
        value={tab}
        onChange={setTab}
        tabs={[
          { id: 'config', label: '配置', badge: <Badge>{plugin.fields.length}</Badge> },
          { id: 'access', label: '访问控制', badge: <Badge>{plugin.commands.length}</Badge> },
        ]}
      />

      <div className="mt-6">
        {tab === 'config' ? (
          plugin.fields.length === 0 ? (
            <EmptyState title="该插件没有可配置项" description="它没有声明 Config 字段。" />
          ) : (
            <div className="flex flex-col gap-5">
              {plugin.fields.map((field) => (
                <Field
                  key={field.key}
                  label={field.label}
                  htmlFor={`plugin-${plugin.id}-${field.key}`}
                  hint={field.description}
                  required={field.required}
                >
                  {fieldInput(field, values[field.key], (next) =>
                    setValues((prev) => ({ ...prev, [field.key]: next })),
                  )}
                </Field>
              ))}
            </div>
          )
        ) : (
          <div className="flex flex-col gap-5">
            <Field label="停用插件" hint="停用后指令与定时任务都不再响应">
              <Switch
                checked={Boolean(access.disabled)}
                onChange={(disabled) => setAccess((prev) => ({ ...prev, disabled }))}
                label="停用插件"
              />
            </Field>

            <Panel title="默认规则" description="未单独配置的指令都走这条">
              <AccessRuleEditor
                path="全部指令"
                rule={access.default ?? EMPTY_RULE}
                onChange={(next) => setAccess((prev) => ({ ...prev, default: next }))}
              />
            </Panel>

            <Panel title="指令覆盖" description="按指令路径单独设置，留「不限制」表示继承默认规则">
              {plugin.commands.length === 0 ? (
                <p className="text-sm text-ink-soft">该插件没有注册指令。</p>
              ) : (
                plugin.commands.map((path) => (
                  <AccessRuleEditor
                    key={path}
                    path={path}
                    rule={access.commands?.[path] ?? EMPTY_RULE}
                    onChange={(next) =>
                      setAccess((prev) => ({ ...prev, commands: { ...prev.commands, [path]: next } }))
                    }
                  />
                ))
              )}
            </Panel>
          </div>
        )}
      </div>
    </Drawer>
  )
}

export default function PluginsPage() {
  const [selected, setSelected] = useState<string>('')
  const { data, isPending, isError, error } = useQuery({
    queryKey: qk.plugins,
    queryFn: () => get<ManagedPlugin[]>('/plugins'),
  })

  const plugins = data ?? []
  const current = plugins.find((plugin) => plugin.id === selected)

  return (
    <>
      <PageHeader
        title="插件"
        description={`共 ${plugins.length} 个插件，配置保存即热更`}
        actions={<Badge>{plugins.reduce((sum, plugin) => sum + plugin.commands.length, 0)} 条指令</Badge>}
      />

      {isPending && !data ? (
        <SkeletonRows rows={6} cols={3} />
      ) : isError && !data ? (
        <EmptyState title="读取插件失败" description={error instanceof Error ? error.message : '请稍后重试'} />
      ) : plugins.length === 0 ? (
        <EmptyState title="没有已注册的插件" />
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {plugins.map((plugin) => (
            <div key={plugin.id} className="card texture-paper flex flex-col">
              <button
                type="button"
                onClick={() => setSelected(plugin.id)}
                className="cursor-pointer rounded-[1.5rem] text-left transition-colors duration-300 hover:bg-subtle/40"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="flex items-center gap-2 font-serif text-lg">
                      <Package aria-hidden className="size-4 shrink-0 text-ink-faint" />
                      <span className="truncate">{plugin.name || plugin.id}</span>
                    </p>
                    <p className="truncate font-mono text-xs text-ink-faint">{plugin.id}</p>
                  </div>
                  {plugin.access?.disabled && <Badge tone="warn">已停用</Badge>}
                </div>
                <p className="mt-3 line-clamp-2 min-h-[2.5rem] text-xs text-ink-soft">
                  {plugin.description || '未提供描述'}
                </p>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Badge>{plugin.commands.length} 条指令</Badge>
                  <Badge>{plugin.fields.length} 个配置项</Badge>
                </div>
              </button>
              {plugin.console && (
                <a
                  href={plugin.console}
                  className="mt-3 flex items-center gap-1.5 border-t border-line/60 pt-3 text-xs text-accent-text transition-colors duration-300 hover:underline"
                >
                  <ExternalLink aria-hidden className="size-3.5 shrink-0" />
                  打开管理台
                  <span className="font-mono text-ink-faint">{plugin.console}</span>
                </a>
              )}
            </div>
          ))}
        </div>
      )}

      {current && <PluginDetail plugin={current} onClose={() => setSelected('')} />}
    </>
  )
}
