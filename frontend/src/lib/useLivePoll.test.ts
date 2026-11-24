import { describe, expect, it } from 'vitest'
import { liveReducer, type LiveState } from './useLivePoll'

const base: LiveState = {
  ver: 10,
  counts: { o1: 1, o2: 0 },
  total: 1,
  status: 'open',
  closesAt: '2030-01-01T00:00:00Z',
  hidden: false,
  viewers: 3,
  deleted: false,
}

describe('liveReducer', () => {
  it('applies newer counts', () => {
    const s = liveReducer(base, { type: 'counts', data: { ver: 11, counts: { o1: 1, o2: 1 }, total: 2 } })
    expect(s.total).toBe(2)
    expect(s.counts).toEqual({ o1: 1, o2: 1 })
  })

  it('ignores stale and duplicate counts', () => {
    expect(liveReducer(base, { type: 'counts', data: { ver: 10, counts: { o1: 9 }, total: 9 } })).toBe(base)
    expect(liveReducer(base, { type: 'counts', data: { ver: 3, counts: { o1: 9 }, total: 9 } })).toBe(base)
  })

  it('keeps counts when an event omits them (results hidden)', () => {
    const s = liveReducer(base, { type: 'counts', data: { ver: 12, total: 5 } })
    expect(s.total).toBe(5)
    expect(s.counts).toEqual(base.counts)
  })

  it('treats a snapshot as authoritative even with a lower version', () => {
    const s = liveReducer(base, {
      type: 'snapshot',
      data: { ver: 2, counts: { o1: 4, o2: 4 }, total: 8, status: 'open', closesAt: base.closesAt, resultsHidden: false, viewers: 1 },
    })
    expect(s.ver).toBe(2)
    expect(s.total).toBe(8)
  })

  it('keeps fresher stream numbers when a refetch is older', () => {
    const s = liveReducer(base, { type: 'reset', state: { ...base, ver: 5, total: 0, status: 'closed' } })
    expect(s.total).toBe(1)
    expect(s.status).toBe('closed')
  })
})
