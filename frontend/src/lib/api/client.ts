// Thin fetch wrapper around the Aurify API. The base URL is proxied to the Go
// backend in dev (see vite.config.ts) and configurable via env in production.
import type { ProblemDetails } from './types'

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

// SCAFFOLD: real auth is not wired yet, so we send a dev user id header that the
// backend reads (see backend handlers/common.go). Replace with session/JWT.
const DEV_USER_ID = import.meta.env.VITE_DEV_USER_ID ?? 'demo-user'

/**
 * ApiError carries the parsed RFC 7807 problem details the API returns on
 * failure, so callers can branch on `status` and show `detail` verbatim instead
 * of inventing their own copy.
 */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    /** The problem's short human-readable summary, when the body carried one. */
    public readonly title?: string,
    /** The problem's specific explanation, when the body carried one. */
    public readonly detail?: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }

  /** True when the failure means "this DSP account isn't linked yet". */
  get isUnauthorized(): boolean {
    return this.status === 401 || this.status === 403
  }
}

/** Narrow an unknown thrown value to an ApiError. */
export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError
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
    throw await toApiError(res)
  }

  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

/**
 * Build an ApiError from a failed response. The API reports errors as RFC 7807
 * problem details (see the ProblemDetails schema), so prefer `detail`/`title`
 * over the raw body — but stay tolerant of proxies and gateways that return
 * plain text or HTML.
 */
async function toApiError(res: Response): Promise<ApiError> {
  const body = await res.text().catch(() => '')

  if (res.headers.get('Content-Type')?.includes('json')) {
    try {
      const problem = JSON.parse(body) as ProblemDetails
      // Normalize blank fields to undefined. The API sends `"detail": ""` on
      // some errors, and an empty string is not nullish — it would survive a
      // `??` chain in the UI and render an empty message.
      const title = problem.title?.trim() || undefined
      const detail = problem.detail?.trim() || undefined
      const message = detail ?? title
      if (message) {
        return new ApiError(res.status, message, title, detail)
      }
    } catch {
      // Malformed JSON: fall through to the plain-text path below.
    }
  }

  return new ApiError(res.status, body || res.statusText)
}
