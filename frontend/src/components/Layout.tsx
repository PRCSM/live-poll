import { useEffect, type CSSProperties, type KeyboardEvent } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { request } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useNow } from '../lib/time'
import type { OwnerPoll } from '../lib/types'
import { Icon } from './ui'

type NavItem = { to: string; label: string; sub: string; live?: boolean }

function greeting(hour: number) {
  if (hour < 5) return 'Up late'
  if (hour < 12) return 'Good morning'
  if (hour < 17) return 'Good afternoon'
  return 'Good evening'
}

function Clock({ open }: { open?: number }) {
  const { user } = useAuth()
  const now = new Date(useNow(15_000))
  const time = now.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' }).toLowerCase()
  const [hm, ampm] = time.split(' ')
  return (
    <div className="clock">
      <div className="time">
        <span>
          {hm}
          {ampm && <small>{ampm}</small>}
        </span>
        {!!open && (
          <span className="badge" title={`${open} open poll${open === 1 ? '' : 's'}`}>
            {open}
          </span>
        )}
      </div>
      <div className="greeting">
        {greeting(now.getHours())},<strong>{user ? user.name.split(' ')[0] : 'guest'}</strong>
      </div>
    </div>
  )
}

/** Section names on a vertical wheel: the current one is in focus, the rest recede. */
function Wheel({ items, active }: { items: NavItem[]; active: number }) {
  const navigate = useNavigate()
  const onKey = (e: KeyboardEvent) => {
    const step = e.key === 'ArrowDown' ? 1 : e.key === 'ArrowUp' ? -1 : 0
    if (!step) return
    e.preventDefault()
    const next = items[Math.min(items.length - 1, Math.max(0, active + step))]
    navigate(next.to)
  }
  return (
    <nav className="wheel" aria-label="Sections" tabIndex={0} onKeyDown={onKey}>
      {items.map((it, i) => {
        const d = i - active
        return (
          <Link
            key={it.label}
            to={it.to}
            aria-current={d === 0 ? 'page' : undefined}
            tabIndex={-1}
            style={{ '--d': d, '--ad': Math.min(Math.abs(d), 3) } as CSSProperties}
          >
            <span className="label">{it.label}</span>
            <span className="sub">
              {it.live && <span className="dot" />}
              {it.sub}
            </span>
          </Link>
        )
      })}
    </nav>
  )
}

export function Layout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const { pathname } = useLocation()

  const mine = useQuery({
    queryKey: ['my-polls-summary'],
    queryFn: () => request<{ polls: OwnerPoll[] }>('/api/my/polls'),
    enabled: !!user,
    refetchInterval: 30_000,
  })
  const open = mine.data?.polls.filter((p) => p.status === 'open').length ?? 0

  // Braces matter: newer browsers return a Promise from scrollTo, and React would
  // treat any returned value as the effect's cleanup function.
  useEffect(() => {
    window.scrollTo(0, 0)
  }, [pathname])

  const onPoll = pathname.startsWith('/p/')
  const items: NavItem[] = user
    ? [
        { to: '/', label: 'Overview', sub: 'Home' },
        { to: '/dashboard', label: 'Polls', sub: open ? `${open} live now` : 'Your polls', live: open > 0 },
        { to: '/new', label: 'Create', sub: 'Ask a question' },
      ]
    : [
        { to: '/', label: 'Overview', sub: 'Home' },
        { to: '/login', label: 'Sign in', sub: 'Welcome back' },
        { to: '/signup', label: 'Join', sub: 'Run your own polls' },
      ]
  if (onPoll) items.splice(1, 0, { to: pathname, label: 'Live', sub: 'Poll in progress', live: true })
  const active = Math.max(
    0,
    items.findIndex((it) => it.to === pathname),
  )

  return (
    <div className="stage">
      <div className="frame">
        <aside className="rail">
          <Clock open={open} />
          <Wheel items={items} active={active} />
          <nav className="capsule" aria-label="Quick links">
            <NavLink to="/" end aria-label="Overview" title="Overview">
              <Icon name="home" />
            </NavLink>
            {user ? (
              <>
                <NavLink to="/dashboard" aria-label="My polls" title="My polls">
                  <Icon name="polls" />
                </NavLink>
                <NavLink to="/new" aria-label="New poll" title="New poll">
                  <Icon name="plus" />
                </NavLink>
                <button
                  type="button"
                  aria-label="Sign out"
                  title="Sign out"
                  onClick={async () => {
                    await logout().catch(() => {})
                    navigate('/')
                  }}
                >
                  <Icon name="out" />
                </button>
              </>
            ) : (
              <NavLink to="/login" aria-label="Sign in" title="Sign in">
                <Icon name="user" />
              </NavLink>
            )}
          </nav>
        </aside>
        <main className="content">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
