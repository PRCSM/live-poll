const raw = import.meta.env.VITE_API_URL as string | undefined

if (!raw) {
  throw new Error('VITE_API_URL is not set. Copy .env.example to .env.local.')
}

export const API_URL = raw.replace(/\/+$/, '')
