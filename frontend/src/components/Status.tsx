import type { Connection } from '../lib/useLivePoll'
import type { PollStatus } from '../lib/types'

const connLabel: Record<Connection, string> = {
  connecting: 'Connecting',
  live: 'Live',
  reconnecting: 'Reconnecting',
  offline: 'Offline',
}

export function ConnectionPill({ connection }: { connection: Connection }) {
  return (
    <span className={`pill conn-${connection}`} role="status">
      <span className="dot" aria-hidden="true" />
      {connLabel[connection]}
    </span>
  )
}

export function StatusPill({ status }: { status: PollStatus }) {
  return <span className={`pill ${status}`}>{status === 'open' ? 'Open' : 'Closed'}</span>
}

export function Wordmark() {
  return (
    <>
      <svg viewBox="0 0 32 32" aria-hidden="true">
        <rect width="32" height="32" rx="6" fill="#16130f" />
        <rect x="7" y="9" width="18" height="4" fill="#d9412b" />
        <rect x="7" y="15" width="12" height="4" fill="#f5f1e8" />
        <rect x="7" y="21" width="7" height="4" fill="#f5f1e8" opacity=".5" />
      </svg>
      Tally
    </>
  )
}
