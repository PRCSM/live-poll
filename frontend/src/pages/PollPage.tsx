import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError, rememberVoterToken, request } from '../lib/api'
import { toLive, usePollView } from '../lib/pollView'
import { useAuth } from '../lib/auth'
import { useLivePoll } from '../lib/useLivePoll'
import type { PollView, VoteResponse } from '../lib/types'
import { ResultBars } from '../components/Results'
import { Countdown } from '../components/Countdown'
import { ShareBox } from '../components/Share'
import { ConnectionPill, StatusPill } from '../components/Status'
import { NotFound } from './Home'

export function PollPage() {
  const { slug = '' } = useParams()
  const view = usePollView(slug)

  if (view.isPending) return <p className="muted">Loading poll…</p>
  if (view.error) {
    if (view.error instanceof ApiError && view.error.status === 404) return <NotFound />
    return (
      <div className="banner" role="alert">
        {view.error.message}{' '}
        <button className="linkish" onClick={() => view.refetch()}>
          Try again
        </button>
      </div>
    )
  }
  return <LivePoll key={slug} slug={slug} view={view.data} />
}

function LivePoll({ slug, view }: { slug: string; view: PollView }) {
  const qc = useQueryClient()
  const { user } = useAuth()
  const location = useLocation()
  const { poll } = view
  const [ownToken, setStreamToken] = useState<string>()
  const [votedHere, setHasVoted] = useState(false)
  // A refetch (after close, reconnect or login) can change visibility and tokens.
  const streamToken = ownToken ?? view.streamToken
  const hasVoted = votedHere || view.hasVoted
  const [mine, setMine] = useState<string[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [voteError, setVoteError] = useState('')

  const refetch = useCallback(() => qc.invalidateQueries({ queryKey: ['poll', slug] }), [qc, slug])
  const live = useLivePoll(slug, streamToken, toLive(view), refetch)
  const { reset } = live

  useEffect(() => reset(toLive(view)), [view, reset])

  const s = live.state
  if (s.deleted) return <NotFound />

  const open = s.status === 'open'
  const needsLogin = poll.requireLogin && !user
  const canVote = open && !hasVoted && !needsLogin
  const shareUrl = `${window.location.origin}/p/${poll.slug}`

  const toggle = (id: string) =>
    setSelected((cur) => (poll.multiChoice ? (cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]) : [id]))

  const vote = async (e: FormEvent) => {
    e.preventDefault()
    if (!selected.length || submitting) return
    setSubmitting(true)
    setVoteError('')
    try {
      const r = await request<VoteResponse>(`/api/polls/${encodeURIComponent(slug)}/votes`, {
        method: 'POST',
        body: { optionIds: selected },
      })
      rememberVoterToken(r.voterToken)
      setHasVoted(true)
      setMine(selected)
      live.apply(r.results)
      // A new token unlocks counts on the stream when results show after voting.
      if (r.streamToken && r.resultsHidden !== s.hidden) setStreamToken(r.streamToken)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'already_voted') {
        setHasVoted(true)
        refetch()
      } else if (err instanceof ApiError && err.code === 'poll_closed') {
        refetch()
        setVoteError('This poll closed before your vote arrived.')
      } else {
        setVoteError(err instanceof Error ? err.message : 'Your vote did not go through.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <article>
      {location.state?.created && (
        <div className="banner info" style={{ marginBottom: 24 }}>
          Your poll is live. Share the link and watch the votes come in.
        </div>
      )}
      <header className="poll-head">
        <div className="meta-line">
          <StatusPill status={open ? 'open' : 'closed'} />
          <ConnectionPill connection={live.connection} />
          <Countdown closesAt={s.closesAt} status={open ? 'open' : 'closed'} />
        </div>
        <h1>{poll.question}</h1>
        {poll.multiChoice && <span className="small muted">Pick all that apply.</span>}
      </header>

      <div className="poll-grid">
        <div className="stack">
          {canVote ? (
            <form className="stack" onSubmit={vote} style={{ gap: 10 }}>
              <fieldset className="stack" style={{ gap: 10 }}>
                <legend className="sr-only">Options</legend>
                {poll.options.map((o) => (
                  <label key={o.id} className="vote-option">
                    <input
                      type={poll.multiChoice ? 'checkbox' : 'radio'}
                      name="option"
                      value={o.id}
                      checked={selected.includes(o.id)}
                      onChange={() => toggle(o.id)}
                      disabled={submitting}
                    />
                    <span>{o.text}</span>
                  </label>
                ))}
              </fieldset>
              {voteError && (
                <div className="banner" role="alert">
                  {voteError}
                </div>
              )}
              <div>
                <button className="btn" disabled={!selected.length || submitting}>
                  {submitting ? 'Sending…' : 'Vote'}
                </button>
              </div>
            </form>
          ) : (
            <>
              {needsLogin && open && !hasVoted && (
                <div className="banner info">
                  This poll needs an account to vote. <Link to={`/login?next=/p/${poll.slug}`}>Sign in</Link> or{' '}
                  <Link to={`/signup?next=/p/${poll.slug}`}>create one</Link>.
                </div>
              )}
              {hasVoted && <p className="small muted">Thanks, your vote is in.</p>}
              {voteError && (
                <div className="banner" role="alert">
                  {voteError}
                </div>
              )}
            </>
          )}

          {s.hidden || !s.counts ? (
            !canVote && (
              <div className="card">
                <strong>Results are hidden</strong>
                <p className="small muted" style={{ margin: '6px 0 0' }}>
                  {poll.resultsVisibility === 'after_vote' ? 'They appear once you vote.' : 'They appear when the poll closes.'}
                </p>
              </div>
            )
          ) : (
            (!canVote || poll.resultsVisibility === 'always') && (
              <section aria-live="polite" aria-label="Live results">
                <ResultBars options={poll.options} counts={s.counts} total={s.total} mine={mine} />
              </section>
            )
          )}
        </div>

        <aside className="sidebar">
          <div className="row" style={{ gap: 32 }}>
            <div className="stat">
              <span className="value">{s.total}</span>
              <span className="label">{s.total === 1 ? 'vote' : 'votes'}</span>
            </div>
            <div className="stat">
              <span className="value">{s.viewers}</span>
              <span className="label">watching</span>
            </div>
          </div>
          <ShareBox url={shareUrl} />
          {poll.isOwner && (
            <div className="row">
              <Link className="btn ghost small" to={`/p/${poll.slug}/present`} target="_blank" rel="noopener">
                Present full screen
              </Link>
              <Link className="btn ghost small" to="/dashboard">
                Manage
              </Link>
            </div>
          )}
        </aside>
      </div>
    </article>
  )
}
