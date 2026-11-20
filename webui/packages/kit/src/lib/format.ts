
export function formatNumber(n: number | null | undefined): string {
  if (n === null || n === undefined || !Number.isFinite(n)) return '—'
  return n.toLocaleString('zh-CN')
}

export function formatCompact(n: number | null | undefined, digits = 1): string {
  if (n === null || n === undefined || !Number.isFinite(n)) return '—'
  const abs = Math.abs(n)
  if (abs < 1000) return Number.isInteger(n) ? String(n) : n.toFixed(digits)
  const units: [number, string][] = [
    [1e12, 'T'],
    [1e9, 'G'],
    [1e6, 'M'],
    [1e3, 'K'],
  ]
  for (const [base, unit] of units) {
    if (abs >= base) {
      const v = n / base
      return `${v.toFixed(v >= 100 ? 0 : digits)}${unit}`
    }
  }
  return String(n)
}

export function formatBytes(bytes: number | null | undefined): string {
  if (bytes === null || bytes === undefined || !Number.isFinite(bytes)) return '—'
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(Math.abs(bytes)) / Math.log(1024)), units.length - 1)
  const v = bytes / 1024 ** i
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function formatDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || !Number.isFinite(seconds)) return '—'
  const s = Math.max(0, Math.floor(seconds))
  if (s < 60) return `${s} 秒`
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (d > 0) return h > 0 ? `${d} 天 ${h} 小时` : `${d} 天`
  if (h > 0) return m > 0 ? `${h} 小时 ${m} 分` : `${h} 小时`
  return `${m} 分`
}

export function formatGoDuration(raw: unknown): string {
  if (raw === null || raw === undefined) return '—'
  return String(raw)
}

export function formatPercent(ratio: number | null | undefined, digits = 1): string {
  if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) return '—'
  return `${(ratio * 100).toFixed(digits)}%`
}

export function toDate(input: string | number | null | undefined): Date | null {
  if (input === null || input === undefined || input === '') return null
  if (typeof input === 'number') {
    return new Date(input < 1e11 ? input * 1000 : input)
  }
  const d = new Date(input)
  return Number.isNaN(d.getTime()) ? null : d
}

export function formatDateTime(input: string | number | null | undefined): string {
  const d = toDate(input)
  if (!d) return '—'
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
}

export function formatTimeOnly(input: string | number | null | undefined): string {
  const d = toDate(input)
  if (!d) return '—'
  return d.toLocaleTimeString('zh-CN', { hour12: false })
}

export function formatDate(input: string | number | null | undefined): string {
  const d = toDate(input)
  if (!d) return '—'
  return d.toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' })
}

export function formatRelative(input: string | number | null | undefined): string {
  const d = toDate(input)
  if (!d) return '—'
  const diffMs = Date.now() - d.getTime()
  const future = diffMs < 0
  const sec = Math.floor(Math.abs(diffMs) / 1000)
  const phrase = (() => {
    if (sec < 5) return '刚刚'
    if (sec < 60) return `${sec} 秒`
    if (sec < 3600) return `${Math.floor(sec / 60)} 分钟`
    if (sec < 86400) return `${Math.floor(sec / 3600)} 小时`
    if (sec < 2592000) return `${Math.floor(sec / 86400)} 天`
    if (sec < 31536000) return `${Math.floor(sec / 2592000)} 个月`
    return `${Math.floor(sec / 31536000)} 年`
  })()
  if (phrase === '刚刚') return phrase
  return future ? `${phrase}后` : `${phrase}前`
}

export function truncate(text: string | null | undefined, max = 80): string {
  if (!text) return ''
  return text.length > max ? `${text.slice(0, max)}…` : text
}

export function formatRating(v: number | null | undefined): string {
  if (v === null || v === undefined || !Number.isFinite(v)) return '—'
  return v.toFixed(2)
}

export function formatUptime(seconds: number | null | undefined): string {
  return formatDuration(seconds)
}
