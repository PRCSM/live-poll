import { useState, type FormEvent } from 'react'
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { ApiError } from '../lib/api'
import { useAuth } from '../lib/auth'

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

  const signup_ = mode === 'signup'
  return (
    <section className="narrow">
      <h1>{signup_ ? 'Create an account' : 'Sign in'}</h1>
      <p className="lede">{signup_ ? 'You need an account to create and manage polls. Voting never needs one.' : 'Welcome back.'}</p>
      <form className="stack" onSubmit={submit} noValidate>
        {error && (
          <div className="banner" role="alert">
            {error}
          </div>
        )}
        {signup_ && (
          <Field id="name" label="Name" error={fields.name}>
            <input id="name" type="text" autoComplete="name" required maxLength={60} value={name} onChange={(e) => setName(e.target.value)} aria-invalid={!!fields.name} />
          </Field>
        )}
        <Field id="email" label="Email" error={fields.email}>
          <input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} aria-invalid={!!fields.email} />
        </Field>
        <Field id="password" label="Password" error={fields.password} hint={signup_ ? 'At least 8 characters.' : undefined}>
          <input
            id="password"
            type="password"
            autoComplete={signup_ ? 'new-password' : 'current-password'}
            required
            minLength={signup_ ? 8 : undefined}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            aria-invalid={!!fields.password}
          />
        </Field>
        <div className="row">
          <button className="btn" disabled={busy}>
            {busy ? 'Please wait…' : signup_ ? 'Create account' : 'Sign in'}
          </button>
          <span className="small muted">
            {signup_ ? (
              <>
                Have an account? <Link to={`/login?next=${encodeURIComponent(next)}`}>Sign in</Link>
              </>
            ) : (
              <>
                New here? <Link to={`/signup?next=${encodeURIComponent(next)}`}>Create an account</Link>
              </>
            )}
          </span>
        </div>
      </form>
    </section>
  )
}

export function Field({ id, label, error, hint, children }: { id: string; label: string; error?: string; hint?: string; children: React.ReactNode }) {
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

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const { user } = useAuth()
  const loc = useLocation()
  if (!user) return <Navigate to={`/login?next=${encodeURIComponent(loc.pathname + loc.search)}`} replace />
  return <>{children}</>
}
