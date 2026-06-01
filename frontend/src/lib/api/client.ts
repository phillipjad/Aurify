// Thin fetch wrapper around the Aurify API. The base URL is proxied to the Go
// backend in dev (see vite.config.ts) and configurable via env in production.

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

// SCAFFOLD: real auth is not wired yet, so we send a dev user id header that the
// backend reads (see backend handlers/common.go). Replace with session/JWT.
const DEV_USER_ID = import.meta.env.VITE_DEV_USER_ID ?? 'demo-user'

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

// Like RequestInit, but headers are a plain record — which is all any caller
// passes. Narrowing it lets us spread `init.headers` safely (RequestInit's
// headers union allows Headers/string[][], which Oxlint's no-misused-spread flags).
type FetchInit = Omit<RequestInit, 'headers'> & {
  headers?: Record<string, string>
}

export async function apiFetch<T>(path: string, init?: FetchInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      'X-User-ID': DEV_USER_ID,
      ...init?.headers,
    },
  })

  if (!res.ok) {
    const detail = await res.text().catch(() => res.statusText)
    throw new ApiError(res.status, detail || res.statusText)
  }

  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}
