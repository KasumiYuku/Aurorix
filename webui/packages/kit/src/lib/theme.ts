//主题控制器：跟随系统 / 浅色 / 深色，落在 <html class="dark"> 与 localStorage。
export type Theme = 'auto' | 'light' | 'dark'

export const THEME_LABEL: Record<Theme, string> = {
  auto: '跟随系统',
  light: '浅色',
  dark: '深色',
}

const KEY = 'aurorix-theme'

export function readTheme(): Theme {
  const v = localStorage.getItem(KEY)
  return v === 'light' || v === 'dark' || v === 'auto' ? v : 'auto'
}

function sync(theme: Theme) {
  const dark = theme === 'dark' || (theme === 'auto' && matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
}

export function applyTheme(theme: Theme) {
  sync(theme)
  localStorage.setItem(KEY, theme)
}

export function watchSystemTheme() {
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (readTheme() === 'auto') sync('auto')
  })
}

//首屏在 React 挂载前执行，避免亮暗闪烁
export function initTheme() {
  sync(readTheme())
  watchSystemTheme()
}
