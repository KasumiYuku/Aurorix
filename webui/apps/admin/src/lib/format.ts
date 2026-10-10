// 展示格式化：时间/时长/字节/数字，统一空值占位。
const EMPTY = '—'

export function fmtClock(ms: number): string {
  if (!ms) return EMPTY
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function fmtTime(ms: number): string {
  if (!ms) return EMPTY
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** 相对时间：未来用「后」，过去用「前」。 */
export function fmtRelative(ms: number, now = Date.now()): string {
  if (!ms) return EMPTY
  const delta = ms - now
  const suffix = delta >= 0 ? '后' : '前'
  return `${fmtDur(Math.abs(delta) / 1000)}${suffix}`
}

export function fmtDur(sec: number): string {
  if (!Number.isFinite(sec) || sec < 0) return EMPTY
  const total = Math.floor(sec)
  const d = Math.floor(total / 86400)
  const h = Math.floor((total % 86400) / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (d > 0) return `${d}天${h}小时`
  if (h > 0) return `${h}小时${m}分`
  if (m > 0) return `${m}分${s}秒`
  return `${s}秒`
}

export function fmtBytes(n: number | undefined): string {
  if (n === undefined || !Number.isFinite(n)) return EMPTY
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = n
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${unit === 0 ? value : value.toFixed(1)} ${units[unit]}`
}

export function fmtNum(n: number | undefined): string {
  if (n === undefined || !Number.isFinite(n)) return EMPTY
  return n.toLocaleString('zh-CN')
}
