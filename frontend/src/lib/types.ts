export type User = { id: string; email: string; name: string; createdAt: string }

export type Session = { token: string; expiresAt: string; user: User }

export type Option = { id: string; text: string }

export type Visibility = 'always' | 'after_vote' | 'after_close'

export type PollStatus = 'open' | 'closed'

export type Results = { ver: number; counts?: Record<string, number>; total: number }

export type PublicPoll = {
  id?: string
  slug: string
  question: string
  options: Option[]
  multiChoice: boolean
  resultsVisibility: Visibility
  requireLogin: boolean
  status: PollStatus
  closesAt: string
  createdAt: string
  isOwner: boolean
}

export type PollView = {
  poll: PublicPoll
  results: Results
  resultsHidden: boolean
  hasVoted: boolean
  viewers: number
  voterToken?: string
  streamToken?: string
}

export type VoteResponse = {
  results: Results
  resultsHidden: boolean
  hasVoted: true
  voterToken?: string
  streamToken?: string
}

export type OwnerPoll = {
  id: string
  slug: string
  question: string
  options: Option[]
  multiChoice: boolean
  resultsVisibility: Visibility
  requireLogin: boolean
  status: PollStatus
  closesAt: string
  createdAt: string
  totalVotes: number | null
}
