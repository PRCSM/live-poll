import { useCallback, useEffect, useState, type CSSProperties, type FormEvent } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError, rememberVoterToken, request } from '../lib/api'
import { toLive, usePollView } from '../lib/pollView'
import { useAuth } from '../lib/auth'
import { useLivePoll } from '../lib/useLivePoll'
import type { PollView, VoteResponse } from '../lib/types'
import { ResultBars } from '../components/Results'
import { TimeLeft } from '../components/Countdown'
import { CopyLink, QR } from '../components/Share'
import { ConnectionPill, StatusPill } from '../components/Status'
import { Card, Counter, Pill, Segmented } from '../components/ui'
import { NotFound } from './Home'

export function PollPage() {
  const { slug = '' } = useParams()
  const view = usePollView(slug)

  if (view.isPending) {
    return (
      <div className="bento poll-grid">
        <div className="skeleton a-q" />
        <div className="skeleton a-votes" />
        <div className="skeleton a-res" />
      </div>
    )
  }
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
  const [mode, setMode] = useState<'pct' | 'count'>('pct')

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
  const showResults = !s.hidden && !!s.counts

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
    <div className="bento poll-grid">
      <Card i={0} className="a-q light">
        <div className="card-head">
          <div className="row" style={{ gap: 6 }}>
            <StatusPill status={open ? 'open' : 'closed'} />
            <ConnectionPill connection={live.connection} />
            {poll.multiChoice && <Pill>Multi-choice</Pill>}
          </div>
          {location.state?.created && <Pill tone="amber">Created · share it</Pill>}
        </div>
        <h1 className="question">{poll.question}</h1>

        {canVote ? (
          <form className="stack" onSubmit={vote} style={{ gap: 12, marginTop: 16 }}>
            <fieldset className="vote-list">
              <legend className="sr-only">Options</legend>
              {poll.options.map((o, i) => (
                <label key={o.id} className={`vote-option${poll.multiChoice ? ' multi' : ''}`} style={{ '--i': i } as CSSProperties}>
                  <input
                    type={poll.multiChoice ? 'checkbox' : 'radio'}
                    name="option"
                    value={o.id}
                    checked={selected.includes(o.id)}
                    onChange={() => toggle(o.id)}
                    disabled={submitting}
                  />
                  <span className="mark" aria-hidden="true" />
                  <span>{o.text}</span>
                </label>
              ))}
            </fieldset>
            {voteError && (
              <div className="banner" role="alert">
                {voteError}
              </div>
            )}
            <div className="spread">
              <span className="small muted">{poll.multiChoice ? 'Pick all that apply.' : 'Pick one.'}</span>
              <button className="btn" disabled={!selected.length || submitting}>
                {submitting ? 'Sending…' : 'Cast vote'}
              </button>
            </div>
          </form>
        ) : (
          <div className="stack" style={{ gap: 10, marginTop: 8 }}>
            {needsLogin && open && !hasVoted && (
              <div className="banner info">
                This poll needs an account to vote. <Link to={`/login?next=/p/${poll.slug}`}>Sign in</Link> or{' '}
                <Link to={`/signup?next=/p/${poll.slug}`}>create one</Link>.
              </div>
            )}
            {hasVoted && <span className="small ink-2">Your vote is in. Results update live.</span>}
            {!open && !hasVoted && <span className="small ink-2">Voting has ended.</span>}
            {voteError && (
              <div className="banner" role="alert">
                {voteError}
              </div>
            )}
          </div>
        )}
      </Card>

      <Card i={1} className="a-votes">
        <div className="card-head">
          <h2 className="card-title">Votes</h2>
          <Pill tone="green" live>
            <span className="dot" />
            {s.viewers} watching
          </Pill>
        </div>
        <div className="big-num">
          <Counter value={s.total} />
          <span className="unit">{s.total === 1 ? 'vote' : 'votes'}</span>
        </div>
        <dl className="kv" style={{ marginTop: 18 }}>
          <dt>Results</dt>
          <dd>{poll.resultsVisibility === 'always' ? 'Live' : poll.resultsVisibility === 'after_vote' ? 'After vote' : 'After close'}</dd>
          <dt>Voting</dt>
          <dd>{poll.requireLogin ? 'Signed in' : 'Per device'}</dd>
        </dl>
      </Card>

      <Card i={2} className="a-res" expandTo={poll.isOwner ? `/p/${poll.slug}/present` : undefined} expandLabel="Present full screen">
        <div className="card-head">
          <h2 className="card-title">Results</h2>
          {showResults && (
            <Segmented
              label="Show results as"
              value={mode}
              onChange={setMode}
              options={[
                { value: 'pct', label: '%' },
                { value: 'count', label: 'Count' },
              ]}
            />
          )}
        </div>
        {showResults ? (
          <section aria-live="polite" aria-label="Live results">
            <ResultBars options={poll.options} counts={s.counts!} total={s.total} mine={mine} mode={mode} />
          </section>
        ) : (
          <div className="empty" style={{ minHeight: 200 }}>
            <div>
              <div className="big-num" style={{ justifyContent: 'center', fontSize: 40 }}>
                Hidden
              </div>
              <p className="small muted">{poll.resultsVisibility === 'after_vote' ? 'Results appear once you vote.' : 'Results appear when the poll closes.'}</p>
            </div>
          </div>
        )}
      </Card>

      <Card i={3} className={`a-time sunrise${open ? '' : ' set'}`}>
        <div className="card-head">
          <h2 className="card-title">Time</h2>
        </div>
        <TimeLeft createdAt={poll.createdAt} closesAt={s.closesAt} status={open ? 'open' : 'closed'} ticks={34} />
      </Card>

      <Card i={4} className="a-share">
        <div className="card-head">
          <h2 className="card-title">Share</h2>
          {poll.isOwner && (
            <Link className="btn chip ghost" to={`/p/${poll.slug}/present`} target="_blank" rel="noopener">
              Present
            </Link>
          )}
        </div>
        <div className="stack" style={{ gap: 12 }}>
          <CopyLink url={shareUrl} />
          <QR value={shareUrl} />
        </div>
      </Card>
    </div>
  )
}
