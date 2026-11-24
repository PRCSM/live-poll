import { useCallback, useEffect } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../lib/api'
import { useLivePoll } from '../lib/useLivePoll'
import type { PollView } from '../lib/types'
import { ResultBars } from '../components/Results'
import { Countdown } from '../components/Countdown'
import { QR } from '../components/Share'
import { ConnectionPill, Wordmark } from '../components/Status'
import { NotFound } from './Home'
import { toLive, usePollView } from '../lib/pollView'

// Presenter view: built for a projector. Large type, QR code to join, no chrome.
export function Present() {
  const { slug = '' } = useParams()
  const view = usePollView(slug)
  if (view.isPending) return <p className="shell muted">Loading…</p>
  if (view.error) {
    if (view.error instanceof ApiError && view.error.status === 404) return <div className="shell"><NotFound /></div>
    return <p className="shell banner">{view.error.message}</p>
  }
  return <Board key={slug} slug={slug} view={view.data} />
}

function Board({ slug, view }: { slug: string; view: PollView }) {
  const qc = useQueryClient()
  const refetch = useCallback(() => qc.invalidateQueries({ queryKey: ['poll', slug] }), [qc, slug])
  const live = useLivePoll(slug, view.streamToken, toLive(view), refetch)
  const { reset } = live
  useEffect(() => reset(toLive(view)), [view, reset])

  const s = live.state
  if (s.deleted) return <div className="shell"><NotFound /></div>
  const url = `${window.location.origin}/p/${view.poll.slug}`
  const open = s.status === 'open'

  return (
    <div className="present">
      <section>
        <div className="meta-line" style={{ marginBottom: 16 }}>
          <Link to={`/p/${slug}`} className="wordmark" style={{ fontSize: 18 }}>
            <Wordmark />
          </Link>
          <ConnectionPill connection={live.connection} />
          <Countdown closesAt={s.closesAt} status={s.status} />
        </div>
        <h1>{view.poll.question}</h1>
        {s.hidden || !s.counts ? (
          <p className="lede">Results are hidden until the poll closes. {s.total} votes so far.</p>
        ) : (
          <ResultBars big options={view.poll.options} counts={s.counts} total={s.total} />
        )}
      </section>
      <aside>
        {open && (
          <>
            <QR value={url} />
            <div className="join">
              Vote at
              <br />
              {url.replace(/^https?:\/\//, '')}
            </div>
          </>
        )}
        <div className="row" style={{ gap: 32 }}>
          <div className="stat">
            <span className="value">{s.total}</span>
            <span className="label">votes</span>
          </div>
          <div className="stat">
            <span className="value">{s.viewers}</span>
            <span className="label">watching</span>
          </div>
        </div>
      </aside>
    </div>
  )
}
