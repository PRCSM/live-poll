import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { configureAuth, request } from './api'
import { read, write } from './storage'
import type { Session, User } from './types'

const KEY = 'tally.session'

function loadSession(): Session | null {
  const raw = read(KEY)
  if (!raw) return null
  try {
    const s = JSON.parse(raw) as Session
    if (Date.parse(s.expiresAt) <= Date.now()) return null
    return s
  } catch {
    return null
  }
}

// The token lives in a module variable as well as state so the API client can read
// it synchronously without re-rendering.
let current: Session | null = loadSession()
let handleUnauthorized = () => {
  current = null
  write(KEY, null)
}

// Configured at import time so requests fired by child components on first render
// already carry the token.
configureAuth(
  () => current?.token ?? null,
  () => handleUnauthorized(),
)

type AuthState = {
  user: User | null
  login: (email: string, password: string) => Promise<void>
  signup: (name: string, email: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(current)
  const qc = useQueryClient()

  const store = useCallback(
    (s: Session | null) => {
      current = s
      write(KEY, s ? JSON.stringify(s) : null)
      setSession(s)
      // Poll views differ for owners and visitors; drop anything fetched as the other.
      qc.removeQueries()
    },
    [qc],
  )

  useEffect(() => {
    handleUnauthorized = () => store(null)
  }, [store])

  // Confirms a stored token is still valid (it may have been revoked by a logout elsewhere).
  useEffect(() => {
    if (!current) return
    request<{ user: User }>('/api/auth/me').catch(() => {})
  }, [])

  const value = useMemo<AuthState>(
    () => ({
      user: session?.user ?? null,
      login: async (email, password) => {
        store(await request<Session>('/api/auth/login', { method: 'POST', body: { email, password } }))
      },
      signup: async (name, email, password) => {
        store(await request<Session>('/api/auth/signup', { method: 'POST', body: { name, email, password } }))
      },
      logout: async () => {
        try {
          await request('/api/auth/logout', { method: 'POST' })
        } finally {
          store(null)
        }
      },
    }),
    [session, store],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}
