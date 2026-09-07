import { createSignal, Show } from 'solid-js'
import { post } from '../api'
import { Button, Checkbox, Icon } from '../ui'

export default function Login({ onAuthed }: { onAuthed: () => void }) {
  const [password, setPassword] = createSignal('')
  const [remember, setRemember] = createSignal(false)
  const [busy, setBusy] = createSignal(false)
  const [error, setError] = createSignal('')

  const submit = async (e: SubmitEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await post('/api/login', { password: password(), remember: remember() })
      onAuthed() // 重新拉取鉴权态, 驱动 Shell 渲染, 不再整页 reload
      location.hash = '#/overview'
    } catch (err) {
      setError((err as Error).message || '登录失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div class="relative flex h-full items-center justify-center overflow-hidden bg-background-2 p-6">
      {/* 背景装饰: 大号衬线字 + 墨绿晕光, 纯 CSS 无图 */}
      <div aria-hidden="true" class="pointer-events-none absolute inset-0 select-none overflow-hidden">
        <span class="font-display absolute -right-6 top-[8%] rotate-[6deg] text-[clamp(120px,22vw,340px)] font-semibold leading-none tracking-tight text-primary-500/10">
          P
        </span>
        <span class="font-display absolute -left-10 bottom-[4%] -rotate-[5deg] text-[clamp(80px,14vw,220px)] font-semibold leading-none tracking-tight text-primary-500/8">
          Aurorix
        </span>
        <div class="absolute left-1/2 top-[-220px] h-[480px] w-[480px] -translate-x-1/2 rounded-full bg-primary-500/15 blur-[110px]" />
      </div>

      <form
        class="px-pop relative w-full max-w-[400px] border border-line-3 bg-card px-9 py-10 shadow-md"
        style={{ 'border-radius': '2.2rem 2rem 2.4rem 1.9rem' }}
        onSubmit={submit}
      >
        <div class="mb-8 flex items-center gap-3.5">
          <span class="px-blob-sm grid h-12 w-12 shrink-0 place-items-center border border-primary-400/25 bg-primary-600 text-[21px] font-semibold text-primary-foreground shadow-sm">
            P
          </span>
          <div class="min-w-0">
            <h1 class="font-display text-[22px] font-semibold leading-tight tracking-tight text-foreground">Aurorix</h1>
            <small class="text-[10.5px] tracking-[0.2em] text-muted-foreground-2">DEEP FOREST</small>
          </div>
        </div>

        <label class="mb-4 block">
          <span class="mb-2 block text-[12.5px] font-medium text-muted-foreground">管理密码</span>
          <input
            type="password"
            value={password()}
            autocomplete="current-password"
            placeholder="admin_password"
            onInput={(e) => setPassword(e.currentTarget.value)}
            autofocus
            class="h-11 w-full rounded-full border border-line-3 bg-field px-5 text-[13.5px] text-foreground outline-none transition-all duration-300 placeholder:text-muted-foreground-2 focus:border-primary-600 focus:ring-2 focus:ring-primary-500/30"
          />
        </label>

        <label class="mb-7 flex cursor-pointer select-none items-center gap-2.5" onClick={() => setRemember(!remember())}>
          <Checkbox checked={remember()} onChange={setRemember} />
          <span class="text-[12.5px] text-muted-foreground">记住密码</span>
        </label>

        <Show when={error()}>
          <div class="mb-5 flex items-center gap-2.5 rounded-full border border-red-200 bg-red-50 px-4.5 py-2.5 text-[12.5px] text-red-600 dark:border-red-400/25 dark:bg-red-400/10 dark:text-red-300">
            <Icon name="alert" size={15} class="shrink-0" />
            <span class="truncate">{error()}</span>
          </div>
        </Show>

        <Button variant="primary" type="submit" disabled={busy() || !password()} class="h-11 w-full rounded-full px-6 text-[14px]">
          {busy() ? '验证中…' : '进入终端'}
        </Button>
      </form>
    </div>
  )
}
