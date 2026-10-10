import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Badge,
  Button,
  EmptyState,
  Field,
  FieldList,
  FieldNumber,
  Input,
  Panel,
  SkeletonRows,
  Switch,
  cn,
} from '@aurorix/webui'
import { Save } from 'lucide-react'
import { PageHeader } from '../components/layout'
import { get, put } from '../lib/api'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { AssetsConfigField, AssetsView, OkResult } from '../lib/types'

interface ProviderDraft {
  enabled: boolean
  priority: number
  config: Record<string, unknown>
}

function schemaInput(
  field: AssetsConfigField,
  value: unknown,
  onChange: (next: unknown) => void,
) {
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
    return <FieldNumber value={typeof value === 'number' ? value : null} onChange={onChange} />
  }
  if (field.type === 'boolean') {
    return <Switch checked={value === true} onChange={onChange} label={field.label} />
  }
  return <Input value={typeof value === 'string' ? value : ''} onChange={(e) => onChange(e.target.value)} />
}

export default function AssetsPage() {
  const [whitelist, setWhitelist] = useState<string[]>([])
  const [drafts, setDrafts] = useState<Record<string, ProviderDraft>>({})
  const [saving, setSaving] = useState(false)

  const { data, isPending, isError, error } = useQuery({
    queryKey: qk.assets,
    queryFn: () => get<AssetsView>('/assets'),
  })

  useEffect(() => {
    if (!data) return
    setWhitelist(data.whitelist ?? [])
    setDrafts(
      Object.fromEntries(
        data.providers.map((provider) => [
          provider.name,
          { enabled: provider.enabled, priority: provider.priority, config: { ...provider.config } },
        ]),
      ),
    )
  }, [data])

  const patch = (name: string, next: Partial<ProviderDraft>) =>
    setDrafts((prev) => ({ ...prev, [name]: { ...prev[name], ...next } as ProviderDraft }))

  const save = async () => {
    if (!data) return
    setSaving(true)
    try {
      await put<OkResult>('/assets', {
        whitelist,
        providers: data.providers.map((provider) => ({
          name: provider.name,
          enabled: drafts[provider.name]?.enabled ?? provider.enabled,
          priority: drafts[provider.name]?.priority ?? provider.priority,
          config: drafts[provider.name]?.config ?? {},
        })),
      })
      await queryClient.invalidateQueries({ queryKey: qk.assets })
      toast('图床配置已保存并热更')
    } catch (err) {
      toast(err instanceof Error ? err.message : '保存失败', 'err')
    } finally {
      setSaving(false)
    }
  }

  const enabledCount = data
    ? data.providers.filter((provider) => drafts[provider.name]?.enabled ?? provider.enabled).length
    : 0

  return (
    <>
      <PageHeader
        title="图床"
        description="上传通道按优先级依次尝试，命中白名单的地址直通不经图床"
        actions={
          <>
            <Badge tone="ok">
              {enabledCount} / {data?.providers.length ?? 0} 已启用
            </Badge>
            <Button icon={Save} loading={saving} onClick={save} disabled={!data}>
              保存
            </Button>
          </>
        }
      />

      {isPending && !data ? (
        <SkeletonRows rows={6} cols={3} />
      ) : isError && !data ? (
        <EmptyState title="读取图床配置失败" description={error instanceof Error ? error.message : '请稍后重试'} />
      ) : !data ? null : (
        <>
          <Panel title="直通白名单" description="匹配到的图片 URL 直接返回，不重复上传">
            <FieldList value={whitelist} onChange={setWhitelist} placeholder="https://example.com" />
          </Panel>

          <div className="flex flex-col gap-4">
            {data.providers.map((provider) => {
              const draft = drafts[provider.name]
              if (!draft) return null
              const schema = provider.schema ?? []
              return (
                <Panel
                  key={provider.name}
                  title={
                    <span className="flex items-center gap-2">
                      <span className="font-mono text-base">{provider.name}</span>
                      {!provider.configured && <Badge tone="warn">未配置</Badge>}
                      {provider.has_secrets && <Badge tone="ok">密钥已存</Badge>}
                    </span>
                  }
                >
                  <div className="grid gap-4 md:grid-cols-[auto_160px_1fr]">
                    <Field label="启用">
                      <Switch
                        checked={draft.enabled}
                        onChange={(enabled) => patch(provider.name, { enabled })}
                        label={`启用 ${provider.name}`}
                      />
                    </Field>
                    <Field label="优先级" hint="数字越大越先尝试">
                      <FieldNumber
                        value={draft.priority}
                        onChange={(priority) => patch(provider.name, { priority: priority ?? 0 })}
                      />
                    </Field>
                    <div className={cn('grid gap-4', schema.length > 1 && 'md:grid-cols-2')}>
                      {schema.map((field) => (
                        <Field
                          key={field.key}
                          label={field.label || field.key}
                          htmlFor={`assets-${provider.name}-${field.key}`}
                          hint={field.description}
                          required={field.required}
                        >
                          {schemaInput(field, draft.config[field.key], (next) =>
                            patch(provider.name, { config: { ...draft.config, [field.key]: next } }),
                          )}
                        </Field>
                      ))}
                    </div>
                  </div>
                </Panel>
              )
            })}
          </div>
        </>
      )}
    </>
  )
}
