import { Link, NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { Wordmark } from './Status'

export function Layout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  return (
    <div className="shell">
      <header className="topbar">
        <Link to="/" className="wordmark">
          <Wordmark />
        </Link>
        <nav aria-label="Main">
          {user ? (
            <>
              <NavLink to="/dashboard" end>
                My polls
              </NavLink>
              <NavLink to="/new">New poll</NavLink>
              <span className="who">{user.name}</span>
              <button
                type="button"
                className="linkish"
                onClick={async () => {
                  await logout().catch(() => {})
                  navigate('/')
                }}
              >
                Sign out
              </button>
            </>
          ) : (
            <>
              <NavLink to="/login">Sign in</NavLink>
              <Link to="/signup" className="btn small">
                Create account
              </Link>
            </>
          )}
        </nav>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
