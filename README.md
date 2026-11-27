# Tally: live polls

Create a poll, share the link, and watch votes arrive on every screen as they are cast.

- **Live app:** https://tally-polls-live.vercel.app
- **API:** https://tally-live-poll-api-jbhg.onrender.com (`/healthz`)
- **Demo video:** https://youtu.be/M4UJQKsmya0

| Layer    | Tech                         | What it does here |
|----------|------------------------------|-------------------|
| Frontend | React 19, Vite, TypeScript   | Auth, poll builder, voting, live results, presenter view |
| Backend  | Go, Gin                      | REST API, validation, auth, Server-Sent Events, workers |
| Database | MongoDB                      | Source of truth: users, polls, every vote |
| Realtime | Redis                        | Live counts, vote dedupe, pub/sub fan-out, vote queue, rate limits, presence, auto-close schedule |

```
/frontend   React app (deployed to Vercel)
/backend    Go service (deployed to Render, Docker)
render.yaml Render blueprint for the API and Redis
```

## How a vote travels

```
browser ── POST /api/polls/:slug/votes ──▶ Go API
                                            │ validate body, poll open, options belong to poll
                                            ▼
                                   Redis Lua script (one atomic step)
                                   ├─ status + closing time check
                                   ├─ SADD voter key        → duplicate? reject
                                   ├─ HINCRBY option counts
                                   ├─ INCR version
                                   ├─ XADD votes:stream     → durable queue
                                   └─ PUBLISH poll:<id>:events
                                            │
              ┌─────────────────────────────┴───────────────────────────┐
              ▼                                                         ▼
  every API instance: one PSUBSCRIBE                       stream worker (consumer group)
  → hub coalesces bursts (≤1 push / 100 ms / poll)         → insert into MongoDB
  → SSE to each viewer                                       (unique index makes replays no-ops)
                                                           → XACK only after Mongo accepted it
```

## Key decisions

**Server-Sent Events, not WebSockets.** Updates only flow server → browser; votes are ordinary POSTs. SSE reconnects by itself, goes through proxies as plain HTTP, and needs no extra protocol. Each stream starts with a snapshot and every update carries a version, so the browser drops anything older than what it has. Reconnects and duplicate deliveries cannot move numbers backwards.

**One Lua script per vote.** The dedupe check, count update, version bump, queue entry and broadcast happen atomically. 50 concurrent requests from the same voter produce exactly one counted vote (see `internal/live/state_test.go`).

**Redis is fast state, MongoDB is truth.** Votes reach Mongo through a Redis Stream with a consumer group. Entries are acknowledged only after Mongo stores them, and pending entries are reclaimed after a crash. If Redis loses its data, the first request for a poll rebuilds counts and voters from Mongo under a lock, then sets a fresh millisecond-based version so clients accept the new numbers.

**One subscription per server, not per viewer.** Each API instance holds a single `PSUBSCRIBE poll:*:events` and fans out in memory, so Redis connections stay constant as viewers grow and any instance can serve any viewer. Slow clients get a fresh snapshot instead of blocking the hub.

**No cookies.** The frontend (`*.vercel.app`) and API (`*.onrender.com`) are different sites, where browsers block third-party cookies. Auth is a bearer JWT (HS256 only, 24 h, revocable through a per-user token version). Anonymous voters carry an HMAC-signed device token in a header. Without cookies, CSRF does not apply; the trade-off is that injected script could read tokens, so the app never renders HTML from user input and tokens are short-lived.

**Hidden results are enforced on the server.** For "after voting" and "after close" polls the API and the stream omit counts until the viewer may see them. A short-lived signed stream token unlocks counts on the stream, because `EventSource` cannot send headers.

## Security

- Strict JSON decoding (unknown fields, trailing data and bodies over 16 KB are rejected), typed structs only, so no operator injection into Mongo.
- Server-side validation of every field: lengths after Unicode normalisation, zero-width and bidi-override characters stripped, duplicate options (case-insensitive), closing time 1 min – 30 days ahead, option ids must belong to the poll, single vs multi choice.
- bcrypt (cost 12); login responds identically and takes the same time for unknown emails and wrong passwords.
- Redis sliding-window rate limits shared by all instances: signup/login per IP and per email, poll creation per user, votes per IP and per IP+poll. Limits fail closed if Redis is down.
- Client IP comes from `True-Client-IP` (set by Render's Cloudflare edge); `X-Forwarded-For` is not trusted because clients can prepend to it.
- CORS allows exactly the frontend origin. Security headers on API and frontend. Owner-only routes return 404 for other users' polls. CSV export escapes formula cells.
- Secrets only in environment variables; gitleaks runs in CI.

## Known limits

- One vote per device is bypassable with a private window. Polls can require sign-in to vote, which makes it one vote per account.
- Votes accepted by Redis but not yet written by the worker would be lost if Redis lost its data in that window (normally milliseconds).
- Render's free tier sleeps after 15 idle minutes. The API pings its own public URL every 10 minutes (only when `RENDER_EXTERNAL_URL` is set) so reviewers never hit a cold start; a scheduled GitHub Action pings it as a backup.

## Run locally

Requirements: Go 1.24+, Node 20+, Docker.

```bash
docker compose up -d                 # MongoDB + Redis

cd backend
cp .env.example .env                 # fill JWT_SECRET and VOTER_SECRET: openssl rand -base64 48
set -a; . ./.env; set +a
go run ./cmd/server                  # http://localhost:8080

cd ../frontend
cp .env.example .env.local
npm install && npm run dev           # http://localhost:5173
```

Tests: `cd backend && go test ./...` (uses an in-memory Redis) and `cd frontend && npm test`.

## API

| Method | Path | Auth |
|---|---|---|
| POST | `/api/auth/signup`, `/api/auth/login` | – |
| POST | `/api/auth/logout` · GET `/api/auth/me` | bearer |
| POST, GET | `/api/my/polls` | bearer |
| PATCH, DELETE | `/api/my/polls/:id` | owner |
| GET | `/api/my/polls/:id/export.csv` | owner |
| GET | `/api/polls/:slug` | optional |
| POST | `/api/polls/:slug/votes` | voter token or bearer |
| GET | `/api/polls/:slug/stream` | – (SSE) |
