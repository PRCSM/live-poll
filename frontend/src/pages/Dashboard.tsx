import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { request, send } from '../lib/api'
import { serverNow, useNow } from '../lib/time'
import type { OwnerPoll } from '../lib/types'
import { Countdown } from '../components/Countdown'
import { StatusPill } from '../components/Status'
import { Card, Counter, Icon, Ruler, Segmented } from '../components/ui'

type Page = { polls: OwnerPoll[]; nextCursor: string | null }
type Filter = 'all' | 'open' | 'closed'

const REOPEN_FOR = [
  { label: '15 min', ms: 15 * 60_000 },
  { label: '1 hour', ms: 3_600_000 },
  { label: '1 day', ms: 86_400_000 },
]

export function Dashboard() {
  const qc = useQueryClient()
  const [error, setError] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const now = useNow(5000)

  const polls = useInfiniteQuery({
    queryKey: ['my-polls'],
    queryFn: ({ pageParam, signal }) => request<Page>(`/api/my/polls${pageParam ? `?before=${pageParam}` : ''}`, { signal }),
    initialPageParam: '',
    getNextPageParam: (last) => last.nextCursor ?? undefined,
    refetchInterval: 15_000,
  })

  const onError = (err: Error) => setError(err.message)
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['my-polls'] })
    qc.invalidateQueries({ queryKey: ['my-polls-summary'] })
  }

  const setStatus = useMutation({
    mutationFn: ({ id, status, ms }: { id: string; status: 'open' | 'closed'; ms?: number }) =>
      request(`/api/my/polls/${id}`, {
        method: 'PATCH',
        body: status === 'open' ? { status, closesAt: new Date(serverNow() + (ms ?? 0)).toISOString() } : { status },
      }),
    onSuccess: refresh,
    onError,
  })

  const remove = useMutation({
    mutationFn: (id: string) => request(`/api/my/polls/${id}`, { method: 'DELETE' }),
    onSuccess: refresh,
    onError,
  })

  const exportCsv = async (p: OwnerPoll) => {
    try {
      const res = await send(`/api/my/polls/${p.id}/export.csv`)
      const url = URL.createObjectURL(await res.blob())
      const a = document.createElement('a')
      a.href = url
      a.download = `poll-${p.slug}.csv`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      onError(err as Error)
    }
  }

  const all = polls.data?.pages.flatMap((p) => p.polls) ?? []
  const shown = filter === 'all' ? all : all.filter((p) => p.status === filter)
  const openCount = all.filter((p) => p.status === 'open').length
  const totalVotes = all.reduce((n, p) => n + (p.totalVotes ?? 0), 0)

  return (
    <>
      <header className="page-head">
        <div>
          <span className="label-mono">Dashboard</span>
          <h1>Your polls</h1>
        </div>
        <div className="row">
          <Segmented<Filter>
            label="Filter polls"
            value={filter}
            onChange={setFilter}
            options={[
              { value: 'all', label: 'All' },
              { value: 'open', label: 'Open' },
              { value: 'closed', label: 'Closed' },
            ]}
          />
          <Link className="btn" to="/new">
            New poll
          </Link>
        </div>
      </header>

      {error && (
        <div className="banner" role="alert" style={{ marginBottom: 8 }}>
          {error}{' '}
          <button className="linkish" onClick={() => setError('')}>
            Dismiss
          </button>
        </div>
      )}

      {polls.isPending && (
        <div className="bento">
          <div className="skeleton" />
          <div className="skeleton" />
          <div className="skeleton" />
        </div>
      )}
      {polls.error && (
        <div className="banner" role="alert">
          {polls.error.message}
        </div>
      )}

      {polls.isSuccess && all.length === 0 && (
        <Card className="empty light">
          <div>
            <h2 style={{ fontSize: 28, fontWeight: 300 }}>No polls yet</h2>
            <p className="lede" style={{ margin: '0 auto 20px' }}>
              Create one, share the link, and its results show up here.
            </p>
            <Link className="btn" to="/new">
              Create your first poll
            </Link>
          </div>
        </Card>
      )}

      {all.length > 0 && (
        <div className="bento">
          <Card i={0} className="light">
            <div className="card-head">
              <h2 className="card-title">Overview</h2>
            </div>
            <div className="big-num">
              <Counter value={totalVotes} />
              <span className="unit">votes</span>
            </div>
            <dl className="kv" style={{ marginTop: 18 }}>
              <dt>Polls</dt>
              <dd>{all.length}</dd>
              <dt>Open</dt>
              <dd>{openCount}</dd>
            </dl>
          </Card>

          {shown.map((p, idx) => {
            const start = Date.parse(p.createdAt)
            const end = Date.parse(p.closesAt)
            const fraction = p.status === 'open' ? (now - start) / Math.max(1, end - start) : 1
            return (
              <Card key={p.id} i={idx + 1} className="poll-card" expandTo={`/p/${p.slug}/present`} expandLabel="Present full screen">
                <div className="spread">
                  <StatusPill status={p.status} />
                  <span className="label-mono">
                    <Countdown closesAt={p.closesAt} status={p.status} />
                  </span>
                </div>
                <Link className="q" to={`/p/${p.slug}`}>
                  {p.question}
                </Link>
                <div>
                  <div className="big-num" style={{ fontSize: 46 }}>
                    {p.totalVotes === null ? '—' : <Counter value={p.totalVotes} />}
                    <span className="unit">votes</span>
                  </div>
                  <Ruler fraction={fraction} ticks={36} />
                </div>
                <div className="actions">
                  <Link className="btn chip ghost" to={`/p/${p.slug}`}>
                    Open
                  </Link>
                  {p.status === 'open' ? (
                    <button className="btn chip ghost" disabled={setStatus.isPending} onClick={() => setStatus.mutate({ id: p.id, status: 'closed' })}>
                      Close
                    </button>
                  ) : (
                    <ReopenMenu disabled={setStatus.isPending} onPick={(ms) => setStatus.mutate({ id: p.id, status: 'open', ms })} />
                  )}
                  <button className="btn chip ghost" onClick={() => exportCsv(p)}>
                    CSV
                  </button>
                  <button
                    className="btn chip ghost danger"
                    disabled={remove.isPending}
                    aria-label={`Delete "${p.question}"`}
                    onClick={() => {
                      if (confirm(`Delete "${p.question}"? Votes are deleted too. This cannot be undone.`)) remove.mutate(p.id)
                    }}
                  >
                    <Icon name="close" />
                  </button>
                </div>
              </Card>
            )
          })}
        </div>
      )}

      {polls.hasNextPage && (
        <div style={{ marginTop: 12 }}>
          <button className="btn ghost" onClick={() => polls.fetchNextPage()} disabled={polls.isFetchingNextPage}>
            {polls.isFetchingNextPage ? 'Loading…' : 'Load more'}
          </button>
        </div>
      )}
    </>
  )
}

function ReopenMenu({ onPick, disabled }: { onPick: (ms: number) => void; disabled: boolean }) {
  return (
    <select
      className="btn chip ghost"
      aria-label="Reopen for"
      disabled={disabled}
      value=""
      onChange={(e) => e.target.value && onPick(Number(e.target.value))}
      style={{ width: 'auto' }}
    >
      <option value="">Reopen…</option>
      {REOPEN_FOR.map((r) => (
        <option key={r.ms} value={r.ms}>
          {r.label}
        </option>
      ))}
    </select>
  )
}
