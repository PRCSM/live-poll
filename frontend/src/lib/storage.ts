// localStorage can throw (Safari private mode, blocked storage). Treat that as "nothing stored".
export function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

export function write(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key)
    else localStorage.setItem(key, value)
  } catch {
    // Storage unavailable: the app still works for this tab, it just won't remember.
  }
}

const VOTER_KEY = 'tally.voter'

export const voterToken = {
  get: () => read(VOTER_KEY),
  set: (t: string) => write(VOTER_KEY, t),
}
