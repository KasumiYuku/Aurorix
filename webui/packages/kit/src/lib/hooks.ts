import { useCallback, useEffect, useRef, useState } from 'react'

export function useDebounced<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay)
    return () => window.clearTimeout(timer)
  }, [value, delay])
  return debounced
}

function comboMatches(combo: string, e: KeyboardEvent, allowInInput: boolean): boolean {
  const parts = combo.toLowerCase().split('+')
  const key = parts[parts.length - 1]
  const needMod = parts.includes('mod')
  const needShift = parts.includes('shift')
  const needAlt = parts.includes('alt')

  if (!allowInInput && !needMod && !needAlt) {
    const target = e.target as HTMLElement | null
    const tag = target?.tagName?.toLowerCase()
    if (tag === 'input' || tag === 'textarea' || tag === 'select' || target?.isContentEditable) return false
  }

  const mod = e.metaKey || e.ctrlKey
  if (needMod !== mod) return false
  if (needShift !== e.shiftKey) return false
  if (needAlt !== e.altKey) return false
  return e.key.toLowerCase() === key
}

export function useHotkey(
  combo: string,
  handler: (e: KeyboardEvent) => void,
  options?: { enabled?: boolean; allowInInput?: boolean },
) {
  const enabled = options?.enabled ?? true
  const allowInInput = options?.allowInInput ?? true
  const ref = useRef(handler)
  ref.current = handler

  useEffect(() => {
    if (!enabled) return
    const onKey = (e: KeyboardEvent) => {
      if (!comboMatches(combo, e, allowInInput)) return
      e.preventDefault()
      ref.current(e)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [combo, enabled, allowInInput])
}

export function useCopy() {
  const [copied, setCopied] = useState<string | null>(null)

  const copy = useCallback(async (text: string): Promise<boolean> => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(text)
      window.setTimeout(() => setCopied(null), 1_500)
      return true
    } catch {
      return false
    }
  }, [])

  return { copy, copied }
}

export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() =>
    typeof window === 'undefined' ? false : window.matchMedia(query).matches,
  )
  useEffect(() => {
    const mql = window.matchMedia(query)
    const onChange = () => setMatches(mql.matches)
    onChange()
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [query])
  return matches
}
