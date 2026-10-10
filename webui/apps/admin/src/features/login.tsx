import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { Button, Field, Input, Switch } from '@aurorix/webui'
import { post } from '../lib/api'
import { qk, queryClient } from '../lib/query'

export default function LoginPage() {
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const [remember, setRemember] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setPending(true)
    setError('')
    try {
      await post('/login', { password, remember })
      await queryClient.refetchQueries({ queryKey: qk.me })
      navigate('/overview', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败')
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="grid h-full place-items-center bg-background px-6">
      <form onSubmit={submit} className="card texture-paper w-full max-w-md">
        <header className="mb-6 flex items-center gap-3">
          <span className="grid size-11 shrink-0 place-items-center rounded-[1.25rem] border border-accent/25 bg-accent font-serif text-[20px] font-semibold text-accent-ink">
            A
          </span>
          <div className="min-w-0">
            <h1 className="font-serif text-xl">Aurorix 管理台</h1>
            <p className="text-xs text-ink-faint">需要管理密码才能进入</p>
          </div>
        </header>

        <div className="flex flex-col gap-5">
          <Field label="管理密码" htmlFor="password" required error={error || undefined}>
            <Input
              id="password"
              type="password"
              value={password}
              autoComplete="current-password"
              autoFocus
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>

          <Field label="保持登录" hint="记住我会保持 30 天，否则关闭浏览器即失效">
            <Switch checked={remember} onChange={setRemember} label="保持登录" />
          </Field>

          <Button type="submit" loading={pending}>
            登录
          </Button>
        </div>
      </form>
    </div>
  )
}
