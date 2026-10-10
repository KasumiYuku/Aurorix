//表单控件：受控组件，校验态只通过 aria-invalid 表达，文案由调用方给。
import { useState, type ComponentProps, type ReactNode } from 'react'
import { ChevronDown, Eye, EyeOff, Info, Plus, TriangleAlert, X } from 'lucide-react'
import { cn } from '../lib/cn'
import { Button } from './ui'

export const Input = ({ className, ...rest }: ComponentProps<'input'>) => (
  <input className={cn('input', className)} {...rest} />
)

export const Textarea = ({ className, ...rest }: ComponentProps<'textarea'>) => (
  <textarea className={cn('input rounded-[1.5rem] py-3', className)} {...rest} />
)

type SelectProps = ComponentProps<'select'> & { options: { value: string; label: string }[] }

export function Select({ options, className, ...rest }: SelectProps) {
  return (
    <span className="relative inline-flex w-full items-center">
      <select className={cn('input appearance-none pr-10', className)} {...rest}>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
      <ChevronDown aria-hidden className="pointer-events-none absolute right-4 size-4 text-ink-faint" />
    </span>
  )
}

type SwitchProps = {
  checked: boolean
  onChange: (next: boolean) => void
  label?: string
  disabled?: boolean
  id?: string
}

export function Switch({ checked, onChange, label, disabled, id }: SwitchProps) {
  return (
    <button
      type="button"
      id={id}
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cn(
        'inline-flex min-h-11 items-center gap-3 rounded-full transition-colors duration-300 ease-in-out',
        disabled && 'cursor-not-allowed opacity-50',
      )}
    >
      <span
        aria-hidden
        className={cn(
          'relative flex h-6 w-11 items-center rounded-full border transition-colors duration-300 ease-in-out',
          checked ? 'border-ok bg-ok/45' : 'border-line bg-subtle',
        )}
      >
        <span
          className={cn(
            'absolute size-5 rounded-full border border-line bg-surface transition-all duration-300 ease-in-out',
            checked ? 'left-[1.375rem]' : 'left-0.5',
          )}
        />
      </span>
      <span className="text-sm text-ink-soft">{checked ? '开启' : '关闭'}</span>
    </button>
  )
}

type FieldProps = {
  label: ReactNode
  htmlFor?: string
  hint?: ReactNode
  error?: string
  required?: boolean
  badge?: ReactNode
  children: ReactNode
  className?: string
}

export function Field({ label, htmlFor, hint, error, required, badge, children, className }: FieldProps) {
  return (
    <div className={cn('flex flex-col gap-2', className)}>
      <div className="flex flex-wrap items-center gap-2">
        <label htmlFor={htmlFor} className="font-serif text-sm text-ink">
          {label}
          {required && <span className="ml-1 text-danger-text">*</span>}
        </label>
        {badge}
      </div>
      {children}
      {hint && !error && <p className="text-xs text-ink-faint">{hint}</p>}
      {error && (
        <p className="flex items-center gap-1.5 text-xs text-danger-text">
          <TriangleAlert aria-hidden className="size-3.5 shrink-0" />
          {error}
        </p>
      )}
    </div>
  )
}

type FieldNumberProps = {
  value: number | null
  onChange: (next: number | null) => void
  min?: number
  max?: number
  step?: number
  placeholder?: string
  id?: string
  invalid?: boolean
}

export function FieldNumber({ value, onChange, min, max, step, placeholder, id, invalid }: FieldNumberProps) {
  return (
    <Input
      id={id}
      type="number"
      inputMode="decimal"
      value={value === null || Number.isNaN(value) ? '' : String(value)}
      min={min}
      max={max}
      step={step ?? 'any'}
      placeholder={placeholder}
      aria-invalid={invalid || undefined}
      onChange={(e) => {
        const raw = e.target.value
        if (raw === '') {
          onChange(null)
          return
        }
        const n = Number(raw)
        onChange(Number.isFinite(n) ? n : null)
      }}
    />
  )
}

type FieldDurationProps = {
  value: string
  onChange: (next: string) => void
  unit?: string
  placeholder?: string
  id?: string
  invalid?: boolean
}

//时长字段只做提示不做换算，后端接受 30s / 5m / 168h 或裸秒数
export function FieldDuration({ value, onChange, unit, placeholder, id, invalid }: FieldDurationProps) {
  return (
    <div className="flex items-center gap-3">
      <Input
        id={id}
        value={value}
        placeholder={placeholder ?? '30s / 5m / 168h'}
        aria-invalid={invalid || undefined}
        spellCheck={false}
        onChange={(e) => onChange(e.target.value)}
      />
      {unit && <span className="shrink-0 text-xs text-ink-faint">单位 {unit}</span>}
    </div>
  )
}

type FieldListProps = {
  value: string[]
  onChange: (next: string[]) => void
  placeholder?: string
  id?: string
}

export function FieldList({ value, onChange, placeholder, id }: FieldListProps) {
  const [draft, setDraft] = useState('')
  const list = value ?? []

  const add = () => {
    const item = draft.trim()
    if (!item || list.includes(item)) {
      setDraft('')
      return
    }
    onChange([...list, item])
    setDraft('')
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-2">
        {list.length === 0 && <span className="text-xs text-ink-faint">暂无条目</span>}
        {list.map((item, i) => (
          <span key={`${item}-${i}`} className="badge border border-line bg-subtle text-ink-soft">
            {item}
            <button
              type="button"
              aria-label={`移除 ${item}`}
              className="ml-1 text-ink-faint transition-colors duration-300 hover:text-danger-text"
              onClick={() => onChange(list.filter((_, idx) => idx !== i))}
            >
              <X aria-hidden className="size-3.5" />
            </button>
          </span>
        ))}
      </div>
      <div className="flex gap-2">
        <Input
          id={id}
          value={draft}
          placeholder={placeholder ?? '输入后回车添加'}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              add()
            }
          }}
        />
        <Button variant="secondary" icon={Plus} onClick={add} disabled={!draft.trim()}>
          添加
        </Button>
      </div>
    </div>
  )
}

type MultiSelectProps = {
  options: string[]
  value: string[]
  onChange: (next: string[]) => void
  labels?: Record<string, string>
}

export function MultiSelect({ options, value, onChange, labels }: MultiSelectProps) {
  return (
    <div className="flex flex-wrap gap-2">
      {options.map((option) => {
        const active = value.includes(option)
        return (
          <button
            key={option}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(active ? value.filter((item) => item !== option) : [...value, option])}
            className={cn(
              'badge cursor-pointer border transition-colors duration-300 ease-in-out',
              active
                ? 'border-accent/40 bg-accent-soft text-accent-text'
                : 'border-line bg-surface text-ink-soft hover:bg-subtle',
            )}
          >
            {labels?.[option] ?? option}
          </button>
        )
      })}
    </div>
  )
}

type FieldSecretProps = {
  value: string
  onChange?: (next: string) => void
  revealed: boolean
  onToggleReveal: () => void
  hint?: ReactNode
  id?: string
  placeholder?: string
  readOnly?: boolean
  revealing?: boolean
}

//密钥字段默认掩码；是否允许显示明文、显示后是否留痕，由调用方的 hint 讲清楚
export function FieldSecret({
  value,
  onChange,
  revealed,
  onToggleReveal,
  hint,
  id,
  placeholder,
  readOnly,
  revealing,
}: FieldSecretProps) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex gap-2">
        <Input
          id={id}
          type={revealed ? 'text' : 'password'}
          value={value}
          readOnly={readOnly}
          placeholder={placeholder}
          autoComplete="off"
          spellCheck={false}
          onChange={(e) => onChange?.(e.target.value)}
        />
        <Button
          variant="secondary"
          icon={revealed ? EyeOff : Eye}
          onClick={onToggleReveal}
          loading={revealing}
          aria-pressed={revealed}
        >
          {revealed ? '隐藏' : '显示'}
        </Button>
      </div>
      {hint && (
        <p className="flex items-center gap-1.5 text-xs text-ink-faint">
          <Info aria-hidden className="size-3.5 shrink-0" />
          {hint}
        </p>
      )}
    </div>
  )
}
