import { useState } from 'react'
import { Monitor, Moon, Sun } from 'lucide-react'
import { cn } from '../lib/cn'
import { THEME_LABEL, applyTheme, readTheme, type Theme } from '../lib/theme'
import { Tooltip } from './ui'

const ICON: Record<Theme, typeof Sun> = { auto: Monitor, light: Sun, dark: Moon }
const ORDER: Theme[] = ['auto', 'light', 'dark']

//外观切换：跟随系统 / 浅色 / 深色。
export function ThemeSwitch({ className }: { className?: string }) {
  const [theme, setTheme] = useState<Theme>(readTheme)
  return (
    <div className={cn('flex shrink-0 items-center gap-1 rounded-full border border-line p-1', className)}>
      {ORDER.map((item) => {
        const Icon = ICON[item]
        return (
          <Tooltip key={item} label={THEME_LABEL[item]}>
            <button
              type="button"
              aria-label={THEME_LABEL[item]}
              aria-pressed={theme === item}
              onClick={() => {
                applyTheme(item)
                setTheme(item)
              }}
              className={cn(
                'flex size-7 cursor-pointer items-center justify-center rounded-full transition-colors duration-300',
                theme === item ? 'bg-accent-soft text-accent-text' : 'text-ink-faint hover:text-ink',
              )}
            >
              <Icon aria-hidden className="size-3.5" />
            </button>
          </Tooltip>
        )
      })}
    </div>
  )
}
