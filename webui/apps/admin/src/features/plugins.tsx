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
  OpsPage,
  OpsSection,
  ResourceTable,
  Select,
  Switch,
  Tabs,
  Textarea,
  type Column,
} from '@aurorix/webui'
import { CircleCheck, ExternalLink, Package, PowerOff, Save, SearchX, Settings2, Terminal, Wrench } from 'lucide-react'
import { get, put } from '../lib/api'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { AccessConfig, AccessRule, ManagedPlugin, OkResult, PluginField, PluginShowIf } from '../lib/types'

const MODE_OPTIONS = [
  { value: 'off', label: '不限制' },
  { value: 'whitelist', label: '白名单' },
  { value: 'blacklist', label: '黑名单' },
]

const ROLE_OPTIONS = [
  { value: '', label: '身份不限' },
  { value: 'admin', label: '管理员以上' },
  { value: 'owner', label: '仅主人' },
]

const MAINTENANCE_PLACEHOLDER = '当前该功能模块正在维护中，请等待维护完成再重试'
const DENY_PLACEHOLDER = '你没有权限使用该指令'

function asRule(rule?: Partial<AccessRule> | null): AccessRule {
  return {
    mode: rule?.mode || 'off',
    users: rule?.users ?? [],
    groups: rule?.groups ?? [],
    role: rule?.role ?? '',
    maintenance: { enabled: false, message: '', ...rule?.maintenance },
    deny: { ...rule?.deny },
  }
}

function AccessRuleEditor({
  rule,
  onChange,
  path,
  allowMaintenance = false,
  inheritedReply = false,
}: {
  rule: AccessRule
  onChange: (next: AccessRule) => void
  path: string
  allowMaintenance?: boolean
  inheritedReply?: boolean
}) {
  const [open, setOpen] = useState(false)
  const { users, groups } = rule
  const named = users.length + groups.length
  const scoped = rule.mode !== 'off'
  const maintenance = rule.maintenance ?? { enabled: false, message: '' }
  const setMaintenance = (patch: Partial<{ enabled: boolean; message: string }>) =>
    onChange({ ...rule, maintenance: { ...maintenance, ...patch } })
  const deny = rule.deny ?? {}
  const denyReply = deny.reply ?? (deny.message ? true : inheritedReply)
  const setDeny = (patch: { reply?: boolean; message?: string }) =>
    onChange({ ...rule, deny: { ...deny, ...patch } })
  const toggleDeny = (checked: boolean) => {
    setDeny({ reply: checked === inheritedReply ? undefined : checked, message: checked ? deny.message ?? '' : '' })
  }
  const denyLabel = rule.mode === 'whitelist' ? '白名单外提示' : '黑名单内提示'
  return (
    <div className="border-b border-line py-2 last:border-b-0">
      <div className="flex flex-wrap items-center gap-2">
        <span className="min-w-0 flex-1 truncate font-mono text-xs text-ink-soft" title={path}>
          {path}
        </span>
        <span className="w-[7.5rem]">
          <Select
            value={rule.mode}
            options={MODE_OPTIONS}
            aria-label={`${path} 的访问模式`}
            onChange={(e) => onChange({ ...rule, mode: e.target.value as AccessRule['mode'] })}
          />
        </span>
        <span className="w-[8.5rem]">
          <Select
            value={rule.role ?? ''}
            options={ROLE_OPTIONS}
            aria-label={`${path} 的身份要求`}
            onChange={(e) => onChange({ ...rule, role: e.target.value as AccessRule['role'] })}
          />
        </span>
        <span className="flex w-[6.5rem] items-center gap-1">
          {allowMaintenance && (
            <>
              <span className="text-xs text-ink-faint">维护</span>
              <Switch
                checked={maintenance.enabled}
                label="维护"
                onChange={(checked) => setMaintenance({ enabled: checked, message: checked ? maintenance.message ?? '' : '' })}
              />
            </>
          )}
        </span>
        <span className="flex w-[6.5rem] items-center gap-1">
          {scoped && (
            <>
              <span className="text-xs text-ink-faint">回话</span>
              <Switch checked={denyReply} label="拦截回话" onChange={toggleDeny} />
            </>
          )}
        </span>
        <span className="w-[4.5rem] shrink-0">
          {scoped && (
            <Button variant="ghost" size="sm" onClick={() => setOpen((prev) => !prev)}>
              名单{named > 0 ? ` ${named}` : ''}
            </Button>
          )}
        </span>
      </div>
      {allowMaintenance && maintenance.enabled && (
        <div className="mt-3">
          <Field label="维护提示语" hint="留空先回落到插件级文案，再回落到框架默认话术">
            <Textarea
              rows={2}
              value={maintenance.message ?? ''}
              placeholder={MAINTENANCE_PLACEHOLDER}
              onChange={(e) => setMaintenance({ enabled: true, message: e.target.value })}
            />
          </Field>
        </div>
      )}
      {scoped && denyReply && (
        <div className="mt-3">
          <Field
            label={denyLabel}
            hint="留空先回落到插件级文案，再回落到框架默认话术"
          >
            <Textarea
              rows={2}
              value={deny.message ?? ''}
              placeholder={DENY_PLACEHOLDER}
              onChange={(e) => setDeny({ message: e.target.value })}
            />
          </Field>
        </div>
      )}
      {scoped && open && (
        <div className="mt-3 grid gap-3 md:grid-cols-2">
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

function ruleMatches(rule: PluginShowIf, values: Record<string, unknown>): boolean {
  if (!rule.key) return true
  if (!(rule.key in values)) return true
  const current = values[rule.key]
  return (rule.values ?? []).includes(typeof current === 'string' ? current : '')
}

function fieldVisible(field: PluginField, values: Record<string, unknown>): boolean {
  const rule = field.show_if
  if (!rule) return true
  if (rule.any && rule.any.length > 0) return rule.any.some((one) => ruleMatches(one, values))
  return ruleMatches(rule, values)
}

function fieldInput(field: PluginField, value: unknown, onChange: (next: unknown) => void) {
  if (field.type === 'boolean') {
    return <Switch checked={value === true} onChange={onChange} label={field.label} />
  }
  if (field.type === 'select' && (field.options ?? []).length > 0) {
    return (
      <Select
        value={typeof value === 'string' ? value : String(field.options?.[0] ?? '')}
        options={(field.options ?? []).map((option) => ({ value: option, label: option }))}
        aria-label={field.label}
        onChange={(e) => onChange(e.target.value)}
      />
    )
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
  const [query, setQuery] = useState('')

  const setMaintenance = (patch: { enabled?: boolean; message?: string }) =>
    setAccess((prev) => ({
      ...prev,
      maintenance: { enabled: false, message: '', ...prev.maintenance, ...patch },
    }))

  const setDeny = (patch: { reply?: boolean; message?: string }) =>
    setAccess((prev) => ({ ...prev, deny: { ...prev.deny, ...patch } }))

  const denyOn = Boolean(access.deny?.reply || access.deny?.message)

  const keyword = query.trim().toLowerCase()
  const commands = plugin.commands ?? []
  const visibleCommands = keyword ? commands.filter((path) => path.toLowerCase().includes(keyword)) : commands

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
      for (const [path, rule] of Object.entries(access.commands ?? {})) {
        if (rule.mode !== 'off' || rule.role || rule.maintenance?.enabled) commands[path] = rule
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
  const fields = plugin.fields.filter((field) => fieldVisible(field, values))

  return (
    <Drawer
      open
      onClose={onClose}
      width="lg"
      title={plugin.name || plugin.id}
      description={plugin.description || plugin.id}
      footer={
        tab === 'config' ? (
          <Button icon={Save} loading={saving === 'config'} onClick={saveConfig} disabled={fields.length === 0}>
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
          className="mb-4 flex items-center justify-between gap-3 rounded-lg border border-line bg-sunk px-4 py-3 text-sm transition-colors duration-300 hover:bg-pressed"
        >
          <span className="flex items-center gap-2">
            <ExternalLink aria-hidden className="size-4 shrink-0 text-ink-faint" />
            该插件自带管理台
          </span>
          <span className="truncate font-mono text-xs text-accent">{plugin.console}</span>
        </a>
      )}

      <Tabs
        value={tab}
        onChange={setTab}
        tabs={[
          { id: 'config', label: '配置', badge: <Badge>{fields.length}</Badge> },
          { id: 'access', label: '访问控制', badge: <Badge>{commands.length}</Badge> },
        ]}
      />

      <div className="mt-6">
        {tab === 'config' ? (
          fields.length === 0 ? (
            <EmptyState
              icon={Settings2}
              title="该插件没有可配置项"
              description="它没有声明 Config 字段。"
            />
          ) : (
            <div className="flex flex-col gap-5">
              {fields.map((field) => (
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
          <>
            <OpsSection title="运行状态" hint="停用是静默的，维护是会说话的">
              <div className="grid gap-5 md:grid-cols-2">
                <Field label="停用插件" hint="完全不响应：指令、消息监听、定时任务一起停，连提示都不发">
                  <Switch
                    checked={Boolean(access.disabled)}
                    onChange={(disabled) => setAccess((prev) => ({ ...prev, disabled }))}
                    label="停用插件"
                  />
                </Field>
                <Field label="维护中" hint="管理员与主人照常可用，其他人收到下面这句提示">
                  <Switch
                    checked={Boolean(access.maintenance?.enabled)}
                    onChange={(enabled) => setMaintenance({ enabled })}
                    label="维护中"
                  />
                </Field>
                {access.maintenance?.enabled && (
                  <Field className="md:col-span-2" label="维护提示语" hint="留空用框架默认话术">
                    <Textarea
                      value={access.maintenance?.message ?? ''}
                      placeholder={MAINTENANCE_PLACEHOLDER}
                      rows={3}
                      onChange={(e) => setMaintenance({ message: e.target.value })}
                    />
                  </Field>
                )}
              </div>
            </OpsSection>


            <OpsSection
              title="名单拦截"
              hint="黑名单内、或白名单外的人触发这个插件的指令时怎么回；默认静默，一个字都不发"
            >
              <div className="grid gap-5 md:grid-cols-2">
                <Field label="拦截回话" hint="关掉就完全不吭声，对方以为机器人没收到">
                  <Switch
                    checked={denyOn}
                    onChange={(reply) => setDeny({ reply, message: reply ? access.deny?.message ?? '' : '' })}
                    label="拦截回话"
                  />
                </Field>
                {denyOn && (
                  <Field
                    className="md:col-span-2"
                    label="拦截提示语"
                    hint="留空用框架默认话术；哪条指令自己写了文案，就按那条的来"
                  >
                    <Textarea
                      value={access.deny?.message ?? ''}
                      placeholder={DENY_PLACEHOLDER}
                      rows={3}
                      onChange={(e) => setDeny({ message: e.target.value })}
                    />
                  </Field>
                )}
              </div>
            </OpsSection>
            <OpsSection
              title="默认规则"
              hint="没单独设置的指令都走这条；只做消息监听的插件（没有指令）也只有这条管它"
            >
              <AccessRuleEditor
                path="全部指令"
                rule={asRule(access.default)}
                inheritedReply={denyOn}
                onChange={(next) => setAccess((prev) => ({ ...prev, default: next }))}
              />
            </OpsSection>

            <OpsSection
              title="指令覆盖"
              hint="只给需要区别对待的指令单独设置；访问规则留第一项就跟随默认规则，维护开关只作用于这一条"
            >
              {commands.length === 0 ? (
                <EmptyState
                  icon={Terminal}
                  title="该插件没有注册指令"
                  description="它的消息监听（比如自动解析链接）只看上面的默认规则：名单里没有的群/人不会触发。"
                />
              ) : (
                <>
                  <div className="mb-3">
                    <Input
                      value={query}
                      placeholder="过滤指令…"
                      aria-label="过滤指令"
                      onChange={(e) => setQuery(e.target.value)}
                    />
                  </div>
                  {visibleCommands.length === 0 ? (
                    <EmptyState icon={SearchX} title="没有匹配的指令" description="换个关键词再试。" />
                  ) : (
                    visibleCommands.map((path) => (
                      <AccessRuleEditor
                        key={path}
                        path={path}
                        rule={asRule(access.commands?.[path])}
                        allowMaintenance
                        inheritedReply={denyOn}
                        onChange={(next) =>
                          setAccess((prev) => ({ ...prev, commands: { ...prev.commands, [path]: next } }))
                        }
                      />
                    ))
                  )}
                </>
              )}
            </OpsSection>
          </>
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
  const commandTotal = plugins.reduce((sum, plugin) => sum + (plugin.commands?.length ?? 0), 0)

  const columns: Column<ManagedPlugin>[] = [
    {
      key: 'name',
      label: '插件',
      primary: true,
      render: (plugin) => (
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="flex min-w-0 items-center gap-2">
            <Package aria-hidden className="size-4 shrink-0 text-ink-faint" />
            <span className="min-w-0 truncate text-sm text-ink">{plugin.name || plugin.id}</span>
          </span>
          <span className="truncate font-mono text-xs text-ink-faint">{plugin.id}</span>
        </div>
      ),
    },
    {
      key: 'commands',
      label: '指令',
      width: '5rem',
      align: 'right',
      render: (plugin) => <span className="text-xs tabular-nums">{plugin.commands.length}</span>,
    },
    {
      key: 'fields',
      label: '配置项',
      width: '6rem',
      align: 'right',
      render: (plugin) => <span className="text-xs tabular-nums">{plugin.fields.length}</span>,
    },
    {
      key: 'status',
      label: '状态',
      width: '8rem',
      render: (plugin) => {
        if (plugin.access?.disabled) {
          return (
            <Badge tone="warn" icon={PowerOff}>
              已停用
            </Badge>
          )
        }
        if (plugin.access?.maintenance?.enabled) {
          return (
            <Badge tone="warn" icon={Wrench}>
              维护中
            </Badge>
          )
        }
        return (
          <Badge tone="ok" icon={CircleCheck}>
            正常
          </Badge>
        )
      },
    },
    {
      key: 'actions',
      label: '操作',
      width: '7rem',
      render: (plugin) => (
        <Button variant="secondary" size="sm" icon={Settings2} onClick={() => setSelected(plugin.id)}>
          打开
        </Button>
      ),
    },
  ]

  return (
    <>
      <OpsPage
        title="插件"
        description="已注册的插件，以及它们的配置与访问控制。配置保存即热更，不用重启进程。"
        meta={`${plugins.length} 个插件 · ${commandTotal} 条指令`}
        actions={<Badge icon={Package}>{commandTotal} 条指令</Badge>}
      >
        <OpsSection title="已注册插件" hint="点一行打开详情">
          <ResourceTable
            columns={columns}
            rows={data}
            rowKey={(plugin) => plugin.id}
            loading={isPending && !data}
            error={isError && !data ? error : undefined}
            onRowClick={(plugin) => setSelected(plugin.id)}
            activeKey={selected}
            empty={{
              icon: Package,
              title: '没有已注册的插件',
              description: '插件在进程启动时注册，启用后刷新页面即可看到。',
            }}
          />
        </OpsSection>
      </OpsPage>

      {current && <PluginDetail plugin={current} onClose={() => setSelected('')} />}
    </>
  )
}
