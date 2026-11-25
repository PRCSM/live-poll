import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { ResultBars } from '../components/Results'
import { Card, Counter, Pill, Ruler } from '../components/ui'

const DEMO_OPTIONS = [
  { id: 'a', text: 'Goa' },
  { id: 'b', text: 'Shillong' },
  { id: 'c', text: 'Pondicherry' },
  { id: 'd', text: 'Coorg' },
]

// A local, clearly labelled demo of how results move. No server involved.
function useDemoVotes() {
  const [counts, setCounts] = useState<Record<string, number>>({ a: 6, b: 9, c: 4, d: 3 })
  useEffect(() => {
    if (matchMedia('(prefers-reduced-motion: reduce)').matches) return
    const weights = [0.3, 0.38, 0.2, 0.12]
    const t = setInterval(() => {
      const r = Math.random()
      let acc = 0
      const pick = DEMO_OPTIONS.find((_, i) => (acc += weights[i]) >= r) ?? DEMO_OPTIONS[0]
      setCounts((c) => {
        const total = Object.values(c).reduce((a, b) => a + b, 0)
        return total > 80 ? { a: 6, b: 9, c: 4, d: 3 } : { ...c, [pick.id]: c[pick.id] + 1 }
      })
    }, 1400)
    return () => clearInterval(t)
  }, [])
  return counts
}

export function Home() {
  const { user } = useAuth()
  const counts = useDemoVotes()
  const total = Object.values(counts).reduce((a, b) => a + b, 0)

  return (
    <div className="bento">
      <Card i={0} className="span-2 light" style={{ justifyContent: 'space-between' }}>
        <div className="spread">
          <span className="label-mono">Tally · live polls</span>
          <Pill tone="green" live>
            <span className="dot" />
            Realtime
          </Pill>
        </div>
        <div>
          <h1 className="display">
            Ask the room.
            <br />
            Watch it answer.
          </h1>
          <p className="lede">Write a question, share one link, and see every vote land the moment it is cast. No refreshing, on any screen.</p>
          <div className="row">
            <Link className="btn" to={user ? '/new' : '/signup?next=/new'}>
              Create a poll
            </Link>
            {user ? (
              <Link className="btn ghost" to="/dashboard">
                My polls
              </Link>
            ) : (
              <Link className="btn ghost" to="/login">
                Sign in
              </Link>
            )}
          </div>
        </div>
      </Card>

      <Card i={1} className="sunrise" style={{ justifyContent: 'space-between' }}>
        <div className="card-head">
          <h2 className="card-title">Refreshes needed</h2>
        </div>
        <div className="big-num">
          0<span className="unit">ever</span>
        </div>
        <p className="small ink-2" style={{ margin: 0 }}>
          Every vote is pushed to every open screen over a live stream, the moment it is counted.
        </p>
      </Card>

      <Card i={2} className="span-2">
        <div className="card-head">
          <h2 className="card-title">
            Where should the offsite be? <span className="count">DEMO</span>
          </h2>
          <span className="label-mono">
            <Counter value={total} /> votes
          </span>
        </div>
        <ResultBars options={DEMO_OPTIONS} counts={counts} total={total} />
      </Card>

      <Card i={3}>
        <div className="card-head">
          <h2 className="card-title">How it works</h2>
        </div>
        <dl className="kv" style={{ gap: '10px 14px' }}>
          <dt>01</dt>
          <dd>Create</dd>
          <dt>02</dt>
          <dd>Share link or QR</dd>
          <dt>03</dt>
          <dd>Audience votes</dd>
          <dt>04</dt>
          <dd>Results move live</dd>
        </dl>
        <div style={{ marginTop: 'auto' }}>
          <Ruler fraction={0.62} ticks={30} tag="closes on time" />
        </div>
      </Card>
    </div>
  )
}

export function NotFound() {
  return (
    <section className="center-msg">
      <h1 className="display">Nothing here.</h1>
      <p className="lede">That page or poll does not exist. Its owner may have deleted it.</p>
      <div>
        <Link className="btn ghost" to="/">
          Go home
        </Link>
      </div>
    </section>
  )
}
