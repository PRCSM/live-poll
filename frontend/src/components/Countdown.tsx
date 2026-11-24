import { formatDate, formatRemaining, useNow } from '../lib/time'
import type { PollStatus } from '../lib/types'
import { Ruler } from './ui'

export function Countdown({ closesAt, status }: { closesAt: string; status: PollStatus }) {
  const end = Date.parse(closesAt)
  const now = useNow(1000, status === 'open')
  if (status !== 'open' || end <= now) return <span>Closed {formatDate(new Date(Math.min(end, now)).toISOString())}</span>
  return (
    <span>
      Closes in <span className="mono">{formatRemaining(end - now)}</span>
    </span>
  )
}

/** Time left as a big readout over a tick strip that fills as the poll runs. */
export function TimeLeft({ createdAt, closesAt, status, ticks }: { createdAt: string; closesAt: string; status: PollStatus; ticks?: number }) {
  const start = Date.parse(createdAt)
  const end = Date.parse(closesAt)
  const now = useNow(1000, status === 'open')
  const open = status === 'open' && end > now
  const fraction = open ? (now - start) / Math.max(1, end - start) : 1
  return (
    <div className="stack" style={{ gap: 10 }}>
      <div className="big-num" style={{ fontSize: 'clamp(40px, 4.4vw, 56px)' }}>
        {open ? formatRemaining(end - now) : 'Closed'}
        {open && <span className="unit">left</span>}
      </div>
      <Ruler fraction={fraction} ticks={ticks} tag={open ? 'now' : 'end'} />
      <div className="spread label-mono">
        <span>Opened {new Date(start).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}</span>
        <span>{open ? 'Closes' : 'Closed'} {formatDate(new Date(Math.min(end, now)).toISOString())}</span>
      </div>
    </div>
  )
}
