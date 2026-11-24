import type { Connection } from '../lib/useLivePoll'
import type { PollStatus } from '../lib/types'
import { Pill } from './ui'

const connLabel: Record<Connection, string> = {
  connecting: 'Connecting',
  live: 'Live',
  reconnecting: 'Reconnecting',
  offline: 'Offline',
}

export function ConnectionPill({ connection }: { connection: Connection }) {
  const tone = connection === 'live' ? 'green' : connection === 'offline' ? 'red' : 'amber'
  return (
    <span role="status">
      <Pill tone={tone} live={connection === 'live'}>
        <span className="dot" aria-hidden="true" />
        {connLabel[connection]}
      </Pill>
    </span>
  )
}

export function StatusPill({ status }: { status: PollStatus }) {
  return status === 'open' ? <Pill tone="green">Open</Pill> : <Pill>Closed</Pill>
}
