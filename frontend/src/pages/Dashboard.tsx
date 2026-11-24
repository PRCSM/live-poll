import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { request, send } from '../lib/api'
import { formatDate, serverNow } from '../lib/time'
import type { OwnerPoll } from '../lib/types'
import { Countdown } from '../components/Countdown'
import { StatusPill } from '../components/Status'

type Page = { polls: OwnerPoll[]; nextCursor: string | null }

const REOPEN_FOR = [
  { label: '15 min', ms: 15 * 60_000 },
  { label: '1 hour', ms: 3_600_000 },
  { label: '1 day', ms: 86_400_000 },
]

export function Dashboard() {
  const qc = useQueryClient()
  const [error, setError] = useState('')
  const polls = useInfiniteQuery({
    queryKey: ['my-polls'],
    queryFn: ({ pageParam, signal }) =>
      request<Page>(`/api/my/polls${pageParam ? `?before=${pageParam}` : ''}`, { signal }),
    initialPageParam: '',
    getNextPageParam: (last) => last.nextCursor ?? undefined,
  })

  const onError = (err: Error) => setError(err.message)
  const refresh = () => qc.invalidateQueries({ queryKey: ['my-polls'] })

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

  return (
    <section>
      <div className="row" style={{ justifyContent: 'space-between', marginBottom: 24 }}>
        <h1 style={{ margin: 0 }}>My polls</h1>
        <Link className="btn" to="/new">
          New poll
        </Link>
      </div>
      {error && (
        <div className="banner" role="alert" style={{ marginBottom: 16 }}>
          {error}{' '}
          <button className="linkish" onClick={() => setError('')}>
            Dismiss
          </button>
        </div>
      )}
      {polls.isPending && <p className="muted">Loading…</p>}
      {polls.error && (
        <div className="banner" role="alert">
          {polls.error.message}
        </div>
      )}
      {polls.isSuccess && all.length === 0 && (
        <div className="card">
          <h2>No polls yet</h2>
          <p className="muted">Create one, share the link, and the results show up here.</p>
          <Link className="btn" to="/new">
            Create your first poll
          </Link>
        </div>
      )}
      {all.length > 0 && (
        <ul className="poll-list">
          {all.map((p) => (
            <li key={p.id}>
              <div className="stack" style={{ gap: 6 }}>
                <Link className="q" to={`/p/${p.slug}`}>
                  {p.question}
                </Link>
                <div className="meta-line">
                  <StatusPill status={p.status} />
                  <span className="mono">{p.totalVotes ?? '—'} votes</span>
                  <Countdown closesAt={p.closesAt} status={p.status} />
                  <span>Created {formatDate(p.createdAt)}</span>
                </div>
              </div>
              <div className="actions">
                <Link className="btn ghost small" to={`/p/${p.slug}/present`} target="_blank" rel="noopener">
                  Present
                </Link>
                {p.status === 'open' ? (
                  <button className="btn ghost small" disabled={setStatus.isPending} onClick={() => setStatus.mutate({ id: p.id, status: 'closed' })}>
                    Close now
                  </button>
                ) : (
                  <ReopenMenu disabled={setStatus.isPending} onPick={(ms) => setStatus.mutate({ id: p.id, status: 'open', ms })} />
                )}
                <button className="btn ghost small" onClick={() => exportCsv(p)}>
                  Export CSV
                </button>
                <button
                  className="btn ghost small danger"
                  disabled={remove.isPending}
                  onClick={() => {
                    if (confirm(`Delete "${p.question}"? Votes are deleted too. This cannot be undone.`)) remove.mutate(p.id)
                  }}
                >
                  Delete
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {polls.hasNextPage && (
        <button className="btn ghost" style={{ marginTop: 20 }} onClick={() => polls.fetchNextPage()} disabled={polls.isFetchingNextPage}>
          {polls.isFetchingNextPage ? 'Loading…' : 'Load more'}
        </button>
      )}
    </section>
  )
}

function ReopenMenu({ onPick, disabled }: { onPick: (ms: number) => void; disabled: boolean }) {
  return (
    <select
      className="btn ghost small"
      aria-label="Reopen for"
      disabled={disabled}
      value=""
      onChange={(e) => e.target.value && onPick(Number(e.target.value))}
      style={{ width: 'auto' }}
    >
      <option value="">Reopen for…</option>
      {REOPEN_FOR.map((r) => (
        <option key={r.ms} value={r.ms}>
          {r.label}
        </option>
      ))}
    </select>
  )
}
