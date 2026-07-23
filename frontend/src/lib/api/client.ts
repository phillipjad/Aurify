// Thin fetch wrapper around the Aurify API. The base URL is proxied to the Go
// backend in dev (see vite.config.ts) and configurable via env in production.
//
// Authentication is entirely by cookie: the API sets an access, a refresh and a
// CSRF cookie on sign-in, and this module's job is to send them, echo the CSRF
// value in a header, and recover from an expired access token without the user
// noticing. Nothing here reads or stores a token — the two that matter are
// HttpOnly and deliberately invisible to script.
import type { ProblemDetails } from './types'

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

/**
 * Name of the CSRF cookie the API issues. It is the one auth cookie readable by
 * script, because the double-submit check requires us to hand its value back in
 * a header — an attacker on another origin can make the browser *send* our
 * cookies but cannot read them, so they cannot populate the header.
 */
const CSRF_COOKIE = 'aurify_csrf'
const CSRF_HEADER = 'X-CSRF-Token'

const REFRESH_PATH = '/auth/refresh'

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

  /** True when the failure means "not signed in" or "not allowed". */
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

/**
 * Listeners notified when the session is definitively gone — a 401 that a
 * refresh could not rescue. The session store subscribes so the UI can drop to
 * a signed-out state from anywhere, including a background query.
 */
type SessionExpiredListener = () => void
const sessionExpiredListeners = new Set<SessionExpiredListener>()

/** Subscribe to "the session ended"; returns an unsubscribe function. */
export function onSessionExpired(listener: SessionExpiredListener): () => void {
  sessionExpiredListeners.add(listener)
  return () => {
    sessionExpiredListeners.delete(listener)
  }
}

function notifySessionExpired() {
  for (const listener of sessionExpiredListeners) listener()
}

export async function apiFetch<T>(path: string, init?: FetchInit): Promise<T> {
  const res = await send(path, init)

  if (res.status === 401 && canRetry(path) && hasSessionHint()) {
    // The access token lasts 15 minutes, so this is the ordinary state of any
    // tab left open. Rotate the pair and replay once before surfacing anything.
    const refreshed = await refreshSession()
    if (!refreshed) {
      notifySessionExpired()
      throw await toApiError(res)
    }
    const retried = await send(path, init)
    if (!retried.ok) {
      // Deliberately not reporting expiry here. The refresh just succeeded, so
      // the session is known good; a still-401 reply is about this endpoint, not
      // the session. /playlists answers 401 when the user has not linked that
      // DSP yet — treating that as expiry signed a perfectly valid user out.
      throw await toApiError(retried)
    }
    return readBody<T>(retried)
  }

  if (!res.ok) {
    throw await toApiError(res)
  }
  return readBody<T>(res)
}

function send(path: string, init?: FetchInit): Promise<Response> {
  const method = init?.method ?? 'GET'
  return fetch(`${BASE_URL}${path}`, {
    ...init,
    // Cookies are the credential. Without this the browser sends none of them
    // and every request is anonymous.
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...csrfHeader(method),
      ...init?.headers,
    },
  })
}

/**
 * The CSRF header, on state-changing requests only.
 *
 * Safe methods are exempt because the API only checks the header where it can
 * change something, and sending it on every GET would spread the value further
 * than it needs to go.
 */
function csrfHeader(method: string): Record<string, string> {
  if (!isStateChanging(method)) return {}
  const token = readCookie(CSRF_COOKIE)
  return token ? { [CSRF_HEADER]: token } : {}
}

function isStateChanging(method: string): boolean {
  return ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase())
}

/**
 * Endpoints where a 401 is the final answer, so a refresh must not be attempted.
 *
 * These either verify a credential themselves — a 401 from sign-in means the
 * password was wrong, and refreshing would swallow the real error — or would
 * recurse, in the case of refresh itself.
 *
 * This is an explicit list rather than a blanket `/auth/` prefix on purpose.
 * `/auth/session` lives under the same prefix and is the single endpoint that
 * most needs to refresh: a 401 there means the access token expired, which is
 * the ordinary state of any tab left open longer than the token's 15 minutes.
 * Excluding it sent users with a perfectly good 30-day refresh token back to
 * the sign-in page.
 */
const TERMINAL_401_PATHS: readonly string[] = [
  '/auth/refresh',
  '/auth/signin',
  '/auth/signup',
  '/auth/signout',
  '/auth/verify-email',
  '/auth/password/forgot',
  '/auth/password/reset',
]

/** Whether a 401 from this path should trigger a refresh and one replay. */
function canRetry(path: string): boolean {
  const [pathname] = path.split('?')
  return !TERMINAL_401_PATHS.includes(pathname ?? path)
}

/**
 * Whether this browser looks like it holds a session worth refreshing.
 *
 * The access and refresh cookies are HttpOnly, so script cannot see them. The
 * CSRF cookie is the readable proxy: the API issues it with the session and
 * clears it with the session, so its presence means "we were signed in".
 *
 * Without this check, a signed-out page load turns its 401 into a refresh that
 * also 401s. That is not merely wasteful — paired with the cache reset that a
 * failed refresh triggers, it re-runs the session query and the two endpoints
 * loop against each other as fast as the network allows.
 */
function hasSessionHint(): boolean {
  return readCookie(CSRF_COOKIE) !== undefined
}

/**
 * In-flight refresh, shared by every caller that 401s at once.
 *
 * This has to be single-flight. Refresh tokens rotate on every use and the API
 * treats a replayed one as theft and revokes the whole session — so three
 * parallel refreshes would kill the very session they were trying to save.
 */
let inFlightRefresh: Promise<boolean> | null = null

function refreshSession(): Promise<boolean> {
  inFlightRefresh ??= send(REFRESH_PATH, { method: 'POST' })
    .then((res) => res.ok)
    .catch(() => false)
    .finally(() => {
      inFlightRefresh = null
    })
  return inFlightRefresh
}

async function readBody<T>(res: Response): Promise<T> {
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

/** Read a cookie by name, or undefined when it is absent. */
function readCookie(name: string): string | undefined {
  if (typeof document === 'undefined') return undefined
  for (const part of document.cookie.split(';')) {
    const [key, ...rest] = part.split('=')
    if (key?.trim() === name) return decodeURIComponent(rest.join('='))
  }
  return undefined
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
