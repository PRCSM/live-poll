import { useCallback, useEffect } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../lib/api'
import { toLive, usePollView } from '../lib/pollView'
import { useLivePoll } from '../lib/useLivePoll'
import type { PollView } from '../lib/types'
import { ResultBars } from '../components/Results'
import { TimeLeft } from '../components/Countdown'
import { QR } from '../components/Share'
import { ConnectionPill, StatusPill } from '../components/Status'
import { Card, Counter, Pill } from '../components/ui'
import { NotFound } from './Home'

// Presenter view for a projector: big question and results, QR code to join.
export function Present() {
  const { slug = '' } = useParams()
  const view = usePollView(slug)
  let body
  if (view.isPending) body = <div className="skeleton" style={{ margin: 28 }} />
  else if (view.error) body = view.error instanceof ApiError && view.error.status === 404 ? <NotFound /> : <p className="banner">{view.error.message}</p>
  else body = <Board key={slug} slug={slug} view={view.data} />
  return (
    <div className="stage">
      <div className="frame present-frame">{body}</div>
    </div>
  )
}

function Board({ slug, view }: { slug: string; view: PollView }) {
  const qc = useQueryClient()
  const refetch = useCallback(() => qc.invalidateQueries({ queryKey: ['poll', slug] }), [qc, slug])
  const live = useLivePoll(slug, view.streamToken, toLive(view), refetch)
  const { reset } = live
  useEffect(() => {
    reset(toLive(view))
  }, [view, reset])

  const s = live.state
  if (s.deleted) return <NotFound />
  const url = `${window.location.origin}/p/${view.poll.slug}`
  const open = s.status === 'open'

  return (
    <>
      <section className="present-main">
        <div className="row" style={{ gap: 6 }}>
          <Link to={`/p/${slug}`} className="label-mono" style={{ textDecoration: 'none', marginRight: 8 }}>
            Tally
          </Link>
          <StatusPill status={s.status} />
          <ConnectionPill connection={live.connection} />
        </div>
        <h1 className="question">{view.poll.question}</h1>
        {s.hidden || !s.counts ? (
          <p className="lede">Results are hidden until the poll closes.</p>
        ) : (
          <ResultBars big options={view.poll.options} counts={s.counts} total={s.total} />
        )}
      </section>
      <aside className="present-side">
        {open && (
          <Card i={0} className="light">
            <div className="card-head">
              <h2 className="card-title">Scan to vote</h2>
            </div>
            <QR value={url} />
            <p className="small ink-2 mono" style={{ marginTop: 12, overflowWrap: 'anywhere' }}>
              {url.replace(/^https?:\/\//, '')}
            </p>
          </Card>
        )}
        <Card i={1}>
          <div className="card-head">
            <h2 className="card-title">Votes</h2>
            <Pill tone="green" live>
              <span className="dot" />
              {s.viewers} watching
            </Pill>
          </div>
          <div className="big-num">
            <Counter value={s.total} />
            <span className="unit">votes</span>
          </div>
        </Card>
        <Card i={2} className={`sunrise${open ? '' : ' set'}`}>
          <TimeLeft createdAt={view.poll.createdAt} closesAt={s.closesAt} status={s.status} ticks={28} />
        </Card>
      </aside>
    </>
  )
}
