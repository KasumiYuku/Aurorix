import { useState } from 'react'
import { Monitor, Moon, Sun } from 'lucide-react'
import { cn } from '../lib/cn'
import { THEME_LABEL, applyTheme, readTheme, type Theme } from '../lib/theme'
import { Tooltip } from './ui'

const ICON: Record<Theme, typeof Sun> = { auto: Monitor, light: Sun, dark: Moon }
const ORDER: Theme[] = ['auto', 'light', 'dark']

export function ThemeSwitch({ className }: { className?: string }) {
  const [theme, setTheme] = useState<Theme>(readTheme)
  return (
    <div className={cn('flex shrink-0 items-center gap-0.5 rounded-md border border-line bg-sunk p-0.5', className)}>
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
                'flex size-6 cursor-pointer items-center justify-center rounded transition-colors duration-150 ease-out',
                theme === item ? 'bg-surface text-ink' : 'text-ink-faint hover:text-ink',
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
