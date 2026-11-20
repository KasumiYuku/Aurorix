import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  AsyncState,
  Badge,
  Button,
  Field,
  FieldList,
  FieldNumber,
  Input,
  OpsPage,
  OpsSection,
  Switch,
} from '@aurorix/webui'
import { CircleCheck, CircleSlash, ImageOff, KeyRound, Save, TriangleAlert } from 'lucide-react'
import { get, put } from '../lib/api'
import { qk, queryClient } from '../lib/query'
import { toast } from '../lib/toast'
import type { AssetsConfigField, AssetsProvider, AssetsView, OkResult } from '../lib/types'

interface ProviderDraft {
  enabled: boolean
  priority: number
  config: Record<string, unknown>
}

function schemaInput(field: AssetsConfigField, value: unknown, onChange: (next: unknown) => void) {
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

  const { data, isPending, isError, error, refetch } = useQuery({
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
    <OpsPage
      title="图床"
      description="上传通道按优先级依次尝试，命中白名单的地址直通不经图床"
      meta={data ? `已启用 ${enabledCount} / ${data.providers.length} · 白名单 ${whitelist.length} 条` : '等待配置加载'}
      actions={
        <>
          <Badge tone={enabledCount > 0 ? 'ok' : 'warn'} icon={enabledCount > 0 ? CircleCheck : CircleSlash}>
            {enabledCount > 0 ? '有通道可用' : '全部关闭'}
          </Badge>
          <Button icon={Save} loading={saving} onClick={save} disabled={!data}>
            保存
          </Button>
        </>
      }
    >
      <AsyncState
        pending={isPending && !data}
        error={!data && isError ? error : undefined}
        isEmpty={!data && !isPending && !isError}
        empty={{ icon: ImageOff, title: '读取图床配置失败', description: '请稍后重试' }}
        skeletonRows={6}
        skeletonCols={3}
        onRetry={() => void refetch()}
      >
        {data && (
          <>
            <OpsSection title="直通白名单" hint="匹配到的图片 URL 直接返回，不重复上传">
              <FieldList value={whitelist} onChange={setWhitelist} placeholder="https://example.com" />
            </OpsSection>
            {data.providers.map((provider) => (
              <ProviderSection
                key={provider.name}
                provider={provider}
                draft={drafts[provider.name]}
                onChange={(next) => patch(provider.name, next)}
              />
            ))}
          </>
        )}
      </AsyncState>
    </OpsPage>
  )
}

function ProviderSection({
  provider,
  draft,
  onChange,
}: {
  provider: AssetsProvider
  draft: ProviderDraft | undefined
  onChange: (next: Partial<ProviderDraft>) => void
}) {
  if (!draft) return null
  const schema = provider.schema ?? []

  return (
    <OpsSection
      title={provider.name}
      hint={
        <span className="flex items-center gap-2">
          {provider.configured ? (
            <Badge tone="ok" icon={CircleCheck}>
              已配置
            </Badge>
          ) : (
            <Badge tone="warn" icon={TriangleAlert}>
              未配置
            </Badge>
          )}
          {provider.has_secrets && (
            <Badge tone="neutral" icon={KeyRound}>
              密钥已存
            </Badge>
          )}
          <span>优先级 {draft.priority}</span>
        </span>
      }
      actions={
        <Switch
          checked={draft.enabled}
          onChange={(enabled) => onChange({ enabled })}
          label={`启用 ${provider.name}`}
        />
      }
    >
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Field label="优先级" htmlFor={`assets-${provider.name}-priority`} hint="数字越大越先尝试">
          <FieldNumber
            id={`assets-${provider.name}-priority`}
            value={draft.priority}
            onChange={(priority) => onChange({ priority: priority ?? 0 })}
          />
        </Field>
        {schema.map((field) => (
          <Field
            key={field.key}
            label={field.label || field.key}
            htmlFor={`assets-${provider.name}-${field.key}`}
            hint={field.description}
            required={field.required}
          >
            {schemaInput(field, draft.config[field.key], (next) =>
              onChange({ config: { ...draft.config, [field.key]: next } }),
            )}
          </Field>
        ))}
      </div>
    </OpsSection>
  )
}
