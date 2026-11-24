import { Link } from 'react-router-dom'
import { useAuth } from '../lib/auth'

export function Home() {
  const { user } = useAuth()
  return (
    <>
      <section className="hero">
        <h1>Ask the room. Watch it answer.</h1>
        <p className="lede">
          Write a question, share one link, and see every vote land the moment it is cast. Nobody refreshes anything.
        </p>
        <div className="row">
          <Link className="btn" to={user ? '/new' : '/signup?next=/new'}>
            Create a poll
          </Link>
          {user && (
            <Link className="btn ghost" to="/dashboard">
              My polls
            </Link>
          )}
        </div>
      </section>
      <ol className="steps">
        <li>
          <strong>Create</strong>
          <span className="small muted">Up to 10 options, a closing time, and who sees results when.</span>
        </li>
        <li>
          <strong>Share</strong>
          <span className="small muted">Send the link or put the QR code on screen.</span>
        </li>
        <li>
          <strong>Vote</strong>
          <span className="small muted">No account needed to vote. One vote per device.</span>
        </li>
        <li>
          <strong>Watch</strong>
          <span className="small muted">Bars move as votes arrive, on every screen at once.</span>
        </li>
      </ol>
    </>
  )
}

export function NotFound() {
  return (
    <section className="center-msg">
      <h1>Nothing here.</h1>
      <p className="lede">That page or poll does not exist. It may have been deleted by its owner.</p>
      <Link className="btn ghost" to="/">
        Go home
      </Link>
    </section>
  )
}
