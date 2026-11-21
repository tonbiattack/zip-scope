// Formatting helpers shared by the list and grid.

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '—'
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[i]}`
}

export function formatDate(ms: number): string {
  if (!ms) return '—'
  const d = new Date(ms)
  if (Number.isNaN(d.getTime())) return '—'
  const pad = (x: number) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`
  return `${(ms / 1000).toFixed(1)} s`
}

// Natural comparison matching the Go side: digit runs compare numerically and
// case is folded, with a case-sensitive tie-break.
export function naturalCompare(a: string, b: string): number {
  const re = /(\d+)|(\D+)/g
  const ax = a.toLowerCase().match(re) ?? []
  const bx = b.toLowerCase().match(re) ?? []
  const n = Math.min(ax.length, bx.length)
  for (let i = 0; i < n; i++) {
    const x = ax[i]
    const y = bx[i]
    const xd = /^\d+$/.test(x)
    const yd = /^\d+$/.test(y)
    if (xd && yd) {
      const xi = Number.parseInt(x, 10)
      const yi = Number.parseInt(y, 10)
      if (xi !== yi) return xi - yi
      if (x.length !== y.length) return x.length - y.length
      continue
    }
    if (x !== y) return x < y ? -1 : 1
  }
  if (ax.length !== bx.length) return ax.length - bx.length
  if (a === b) return 0
  return a < b ? -1 : 1
}

// statusLabel maps a backend status to a short Japanese label.
export function statusLabel(status: string): string {
  switch (status) {
    case 'ok':
      return ''
    case 'empty':
      return '画像なし'
    case 'corrupt':
      return '破損'
    case 'encrypted':
      return '暗号化'
    case 'missing':
      return '見つかりません'
    default:
      return 'エラー'
  }
}

// errorKindLabel maps an image error kind to a placeholder caption.
export function errorKindLabel(kind: string): string {
  switch (kind) {
    case 'encrypted':
      return '暗号化'
    case 'corrupt':
      return '破損'
    case 'unsupported':
      return '非対応'
    case 'too-large':
      return 'サイズ超過'
    case 'too-many-pixels':
      return '画素数超過'
    case 'missing':
      return 'なし'
    default:
      return '読込失敗'
  }
}
