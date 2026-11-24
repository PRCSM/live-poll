import { API_URL } from './config'
import { voterToken } from './storage'
import { setServerTime } from './time'

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly fields: Record<string, string>

  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message)
    this.status = status
    this.code = code
    this.fields = fields
  }
}

let getToken: () => string | null = () => null
let onUnauthorized: () => void = () => {}

export function configureAuth(tokenGetter: () => string | null, unauthorized: () => void) {
  getToken = tokenGetter
  onUnauthorized = unauthorized
}

type Options = { method?: string; body?: unknown; signal?: AbortSignal }

export async function request<T>(path: string, opts: Options = {}): Promise<T> {
  const res = await send(path, opts)
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

// send performs the request and turns every non-2xx response into an ApiError.
export async function send(path: string, { method = 'GET', body, signal }: Options = {}): Promise<Response> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`
  const voter = voterToken.get()
  if (voter) headers['X-Voter-Token'] = voter
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  let res: Response
  try {
    res = await fetch(API_URL + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal,
    })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(0, 'network', 'Could not reach the server. Check your connection.')
  }

  const serverTime = res.headers.get('X-Server-Time')
  if (serverTime) setServerTime(serverTime)

  if (res.ok) return res

  let payload: { error?: { code: string; message: string; fields?: Record<string, string> } } = {}
  try {
    payload = await res.json()
  } catch {
    // Non-JSON error (proxy page, cold start): fall through to a generic message.
  }
  const e = payload.error
  if (res.status === 401 && token) onUnauthorized()
  if (res.status === 429) {
    const retry = res.headers.get('Retry-After')
    throw new ApiError(429, 'rate_limited', retry ? `Too many requests. Try again in ${retry}s.` : 'Too many requests.')
  }
  throw new ApiError(res.status, e?.code ?? 'http_error', e?.message ?? `Request failed (${res.status}).`, e?.fields)
}

export function rememberVoterToken(t?: string) {
  if (t) voterToken.set(t)
}
