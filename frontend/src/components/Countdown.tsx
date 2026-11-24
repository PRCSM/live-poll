import { useEffect, useState } from 'react'
import { formatDate, formatRemaining, serverNow } from '../lib/time'
import type { PollStatus } from '../lib/types'

export function Countdown({ closesAt, status }: { closesAt: string; status: PollStatus }) {
  const end = Date.parse(closesAt)
  const [now, setNow] = useState(serverNow)

  useEffect(() => {
    if (status !== 'open') return
    const t = setInterval(() => setNow(serverNow()), 1000)
    return () => clearInterval(t)
  }, [status])

  if (status !== 'open' || end <= now) {
    return <span>Closed {formatDate(new Date(Math.min(end, now)).toISOString())}</span>
  }
  return (
    <span>
      Closes in <span className="mono">{formatRemaining(end - now)}</span>
    </span>
  )
}
