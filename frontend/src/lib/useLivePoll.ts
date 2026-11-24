import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import { API_URL } from './config'
import type { PollStatus, Results } from './types'

export type LiveState = {
  ver: number
  counts?: Record<string, number>
  total: number
  status: PollStatus
  closesAt: string
  hidden: boolean
  viewers: number
  deleted: boolean
}

export type Connection = 'connecting' | 'live' | 'reconnecting' | 'offline'

type Action =
  | { type: 'reset'; state: LiveState }
  | { type: 'snapshot'; data: Omit<LiveState, 'deleted' | 'hidden'> & { resultsHidden: boolean } }
  | { type: 'counts'; data: { ver: number; counts?: Record<string, number>; total: number } }
  | { type: 'status'; data: { status: PollStatus; closesAt: string } }
  | { type: 'viewers'; data: { viewers?: number } }
  | { type: 'deleted' }

export function liveReducer(s: LiveState, a: Action): LiveState {
  switch (a.type) {
    case 'reset':
      // A refetched view can be older than what the stream already delivered; keep the newer numbers.
      if (a.state.ver < s.ver && a.state.hidden === s.hidden) {
        return { ...a.state, ver: s.ver, counts: s.counts, total: s.total }
      }
      return a.state
    case 'snapshot': {
      // A snapshot is authoritative, even if its version is lower (state rebuilt server-side).
      const { resultsHidden, ...rest } = a.data
      return { ...s, ...rest, counts: rest.counts, hidden: resultsHidden }
    }
    case 'counts':
      if (a.data.ver <= s.ver) return s // stale or duplicate
      return { ...s, ver: a.data.ver, total: a.data.total, counts: a.data.counts ?? s.counts }
    case 'status':
      return { ...s, status: a.data.status, closesAt: a.data.closesAt }
    case 'viewers':
      return { ...s, viewers: a.data.viewers ?? 0 }
    case 'deleted':
      return { ...s, deleted: true }
  }
}

const MAX_BACKOFF = 30_000

/**
 * Subscribes to a poll's live stream. The server sends a snapshot first, then
 * versioned updates; anything older than what we already have is ignored, so
 * reconnects and duplicate deliveries can't move the numbers backwards.
 */
export function useLivePoll(
  slug: string,
  streamToken: string | undefined,
  initial: LiveState,
  onStatusChange: () => void,
) {
  const [state, dispatch] = useReducer(liveReducer, initial)
  const [connection, setConnection] = useState<Connection>('connecting')
  const statusCb = useRef(onStatusChange)
  useEffect(() => {
    statusCb.current = onStatusChange
  }, [onStatusChange])

  useEffect(() => {
    let es: EventSource | null = null
    let timer: ReturnType<typeof setTimeout> | undefined
    let attempt = 0
    let stopped = false

    const url = `${API_URL}/api/polls/${encodeURIComponent(slug)}/stream${streamToken ? `?st=${encodeURIComponent(streamToken)}` : ''}`

    const parse = <T,>(e: Event) => JSON.parse((e as MessageEvent<string>).data) as T

    const open = () => {
      if (stopped) return
      setConnection(attempt === 0 ? 'connecting' : 'reconnecting')
      es = new EventSource(url)
      es.addEventListener('open', () => {
        attempt = 0
        setConnection('live')
      })
      es.addEventListener('snapshot', (e) => dispatch({ type: 'snapshot', data: parse(e) }))
      es.addEventListener('counts', (e) => dispatch({ type: 'counts', data: parse(e) }))
      es.addEventListener('viewers', (e) => dispatch({ type: 'viewers', data: parse(e) }))
      es.addEventListener('status', (e) => {
        dispatch({ type: 'status', data: parse(e) })
        statusCb.current()
      })
      es.addEventListener('deleted', () => {
        dispatch({ type: 'deleted' })
        stop()
      })
      es.onerror = () => {
        if (!es || stopped) return
        if (es.readyState === EventSource.CLOSED) {
          // The browser gave up (HTTP error, e.g. rate limit or the server was down).
          // Retry ourselves with backoff and jitter so clients don't reconnect in lockstep.
          es = null
          const delay = Math.min(MAX_BACKOFF, 1000 * 2 ** attempt) + Math.random() * 1000
          attempt++
          setConnection(navigator.onLine ? 'reconnecting' : 'offline')
          statusCb.current()
          timer = setTimeout(open, delay)
        } else {
          setConnection(navigator.onLine ? 'reconnecting' : 'offline')
        }
      }
    }

    const stop = () => {
      stopped = true
      es?.close()
      es = null
      clearTimeout(timer)
    }

    const goOffline = () => setConnection('offline')
    const goOnline = () => {
      if (!es && !stopped) {
        clearTimeout(timer)
        attempt = 0
        open()
      }
    }

    open()
    window.addEventListener('offline', goOffline)
    window.addEventListener('online', goOnline)
    return () => {
      stop()
      window.removeEventListener('offline', goOffline)
      window.removeEventListener('online', goOnline)
    }
  }, [slug, streamToken])

  // Applies counts returned directly by our own vote, which may arrive before the stream event.
  const apply = useCallback((r: Results) => {
    dispatch({ type: 'counts', data: r })
  }, [])

  const reset = useCallback((s: LiveState) => dispatch({ type: 'reset', state: s }), [])

  return { state, connection, apply, reset }
}
