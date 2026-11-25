import { useState, type FormEvent, type ReactNode } from 'react'
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { ApiError } from '../lib/api'
import { useAuth } from '../lib/auth'
import { Card } from '../components/ui'

// Only same-site paths are allowed as a post-login redirect.
function safeNext(raw: string | null): string {
  return raw && raw.startsWith('/') && !raw.startsWith('//') ? raw : '/dashboard'
}

export function AuthPage({ mode }: { mode: 'login' | 'signup' }) {
  const { user, login, signup } = useAuth()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const next = safeNext(params.get('next'))
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  if (user) return <Navigate to={next} replace />

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    setFields({})
    try {
      if (mode === 'signup') await signup(name, email, password)
      else await login(email, password)
      navigate(next, { replace: true })
    } catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length) setFields(err.fields)
      else setError(err instanceof Error ? err.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }

  const isSignup = mode === 'signup'
  return (
    <div className="bento">
      <Card i={0} className="light" style={{ gridColumn: 'span 2', maxWidth: 560 }}>
        <div className="card-head">
          <span className="label-mono">{isSignup ? 'New account' : 'Sign in'}</span>
        </div>
        <h1 style={{ fontSize: 36, fontWeight: 300, letterSpacing: '-0.045em' }}>{isSignup ? 'Create an account' : 'Welcome back'}</h1>
        <p className="lede">{isSignup ? 'You need an account to create and manage polls. Voting never needs one.' : 'Sign in to create and manage your polls.'}</p>
        <form className="stack" onSubmit={submit} noValidate>
          {error && (
            <div className="banner" role="alert">
              {error}
            </div>
          )}
          {isSignup && (
            <Field id="name" label="Name" error={fields.name}>
              <input id="name" type="text" autoComplete="name" required maxLength={60} value={name} onChange={(e) => setName(e.target.value)} aria-invalid={!!fields.name} />
            </Field>
          )}
          <Field id="email" label="Email" error={fields.email}>
            <input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} aria-invalid={!!fields.email} />
          </Field>
          <Field id="password" label="Password" error={fields.password} hint={isSignup ? 'At least 8 characters.' : undefined}>
            <input
              id="password"
              type="password"
              autoComplete={isSignup ? 'new-password' : 'current-password'}
              required
              minLength={isSignup ? 8 : undefined}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              aria-invalid={!!fields.password}
            />
          </Field>
          <div className="spread">
            <button className="btn" disabled={busy}>
              {busy ? 'Please wait…' : isSignup ? 'Create account' : 'Sign in'}
            </button>
            <span className="small muted">
              {isSignup ? 'Have an account? ' : 'New here? '}
              <Link to={`/${isSignup ? 'login' : 'signup'}?next=${encodeURIComponent(next)}`}>{isSignup ? 'Sign in' : 'Create one'}</Link>
            </span>
          </div>
        </form>
      </Card>
      <Card i={1} className="sunrise" style={{ justifyContent: 'space-between' }}>
        <span className="label-mono">Why sign in</span>
        <div className="stack" style={{ gap: 6 }}>
          <span className="small ink-2">Polls you create stay private to manage: close, reopen, export or delete them any time.</span>
        </div>
      </Card>
    </div>
  )
}

export function Field({ id, label, error, hint, children }: { id: string; label: string; error?: string; hint?: string; children: ReactNode }) {
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {children}
      {hint && !error && <span className="hint">{hint}</span>}
      {error && (
        <span className="error" role="alert">
          {error}
        </span>
      )}
    </div>
  )
}

export function RequireAuth({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  const loc = useLocation()
  if (!user) return <Navigate to={`/login?next=${encodeURIComponent(loc.pathname + loc.search)}`} replace />
  return <>{children}</>
}
