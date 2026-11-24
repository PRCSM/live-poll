// Offset between the server clock and this device, so countdowns are right even
// when the device clock is off.
let offsetMs = 0

export function setServerTime(iso: string) {
  const server = Date.parse(iso)
  if (!Number.isNaN(server)) offsetMs = server - Date.now()
}

export function serverNow(): number {
  return Date.now() + offsetMs
}

export function formatRemaining(ms: number): string {
  if (ms <= 0) return '0s'
  const s = Math.floor(ms / 1000)
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${String(sec).padStart(2, '0')}s`
  return `${sec}s`
}

const dateFmt = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })

export function formatDate(iso: string): string {
  return dateFmt.format(new Date(iso))
}
