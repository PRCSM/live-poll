import { useQuery } from '@tanstack/react-query'
import { ApiError, rememberVoterToken, request } from './api'
import type { LiveState } from './useLivePoll'
import type { PollView } from './types'

export function usePollView(slug: string) {
  return useQuery({
    queryKey: ['poll', slug],
    queryFn: async ({ signal }) => {
      const v = await request<PollView>(`/api/polls/${encodeURIComponent(slug)}`, { signal })
      rememberVoterToken(v.voterToken)
      return v
    },
    retry: (n, err) => !(err instanceof ApiError && err.status === 404) && n < 2,
  })
}

export function toLive(v: PollView): LiveState {
  return {
    ver: v.results.ver,
    counts: v.results.counts,
    total: v.results.total,
    status: v.poll.status,
    closesAt: v.poll.closesAt,
    hidden: v.resultsHidden,
    viewers: v.viewers,
    deleted: false,
  }
}
