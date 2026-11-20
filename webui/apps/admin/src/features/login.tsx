import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { Button, Card, Field, Input, Switch } from '@aurorix/webui'
import { TriangleAlert } from 'lucide-react'
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
    <div className="grid min-h-dvh place-items-center bg-paper px-4">
      <Card className="w-full max-w-sm">
        <form onSubmit={submit} className="flex flex-col gap-5">
          <header className="flex items-center gap-3">
            <span className="grid size-10 shrink-0 place-items-center rounded-md bg-accent text-base font-semibold text-white">
              A
            </span>
            <div className="min-w-0">
              <h1 className="text-lg text-ink">Aurorix 管理台</h1>
              <p className="text-xs text-ink-faint">需要管理密码才能进入</p>
            </div>
          </header>

          <Field label="管理密码" htmlFor="password" required>
            <Input
              id="password"
              type="password"
              value={password}
              autoComplete="current-password"
              autoFocus
              aria-invalid={error ? true : undefined}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>

          {error && (
            <p role="alert" className="flex items-start gap-2 text-sm text-danger-ink">
              <TriangleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
              <span className="min-w-0">{error}</span>
            </p>
          )}

          <Field label="保持登录" hint="记住我会保持 30 天，否则关闭浏览器即失效">
            <Switch checked={remember} onChange={setRemember} label="保持登录" />
          </Field>

          <Button type="submit" loading={pending} className="w-full">
            登录
          </Button>
        </form>
      </Card>
    </div>
  )
}
