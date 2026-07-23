import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'

import { ApiError, apiFetch } from '@/lib/api/client'

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

function unauthorized(): Response {
  return jsonResponse({ title: 'Unauthorized', detail: 'Sign in again.', status: 401 }, { status: 401 })
}

/** Set the CSRF cookie the API issues alongside the session. */
function setCsrfCookie(value: string) {
  document.cookie = `aurify_csrf=${value}; path=/`
}

function clearCookies() {
  for (const cookie of document.cookie.split(';')) {
    const name = cookie.split('=')[0]?.trim()
    if (name) document.cookie = `${name}=; path=/; max-age=0`
  }
}

/** Headers of the nth fetch call, as a plain record. */
function headersOf(fetchMock: Mock, call = 0): Record<string, string> {
  const init = fetchMock.mock.calls[call]?.[1] as { headers?: Record<string, string> } | undefined
  return init?.headers ?? {}
}

describe('apiFetch', () => {
  let fetchMock: Mock

  beforeEach(() => {
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    clearCookies()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    clearCookies()
  })

  it('prefixes the API base URL and sends the session cookies', async () => {
    fetchMock.mockResolvedValue(jsonResponse([{ id: 'p1' }]))

    const data = await apiFetch<{ id: string }[]>('/playlists?platform=spotify')

    expect(data).toEqual([{ id: 'p1' }])
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/playlists?platform=spotify')
    // Without this the browser withholds the auth cookies entirely and every
    // request is anonymous.
    expect(init.credentials).toBe('include')
    expect(headersOf(fetchMock)['Content-Type']).toBe('application/json')
  })

  // The header was an authentication bypass: any client could name any user and
  // be believed. It must never come back.
  it('never sends the X-User-ID identity header', async () => {
    fetchMock.mockResolvedValue(jsonResponse([]))
    await apiFetch('/playlists')
    expect(headersOf(fetchMock)).not.toHaveProperty('X-User-ID')
  })

  it('echoes the CSRF cookie in a header on state-changing requests', async () => {
    setCsrfCookie('csrf-token-value')
    fetchMock.mockResolvedValue(jsonResponse({ id: 'c1' }, { status: 201 }))

    await apiFetch('/covers', { method: 'POST', body: '{}' })

    expect(headersOf(fetchMock)['X-CSRF-Token']).toBe('csrf-token-value')
  })

  it('omits the CSRF header on safe requests', async () => {
    setCsrfCookie('csrf-token-value')
    fetchMock.mockResolvedValue(jsonResponse([]))

    await apiFetch('/playlists')

    expect(headersOf(fetchMock)).not.toHaveProperty('X-CSRF-Token')
  })

  it('merges caller init: method, body, and extra headers', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ id: 'c1' }))

    await apiFetch('/covers', {
      method: 'POST',
      body: JSON.stringify({ playlistId: 'p1' }),
      headers: { 'X-Trace': 'abc' },
    })

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init.method).toBe('POST')
    expect(init.body).toBe('{"playlistId":"p1"}')
    expect(headersOf(fetchMock)['X-Trace']).toBe('abc')
    expect(headersOf(fetchMock)['Content-Type']).toBe('application/json')
  })

  it('returns undefined for a 204 No Content response', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }))
    const data = await apiFetch('/covers/x', { method: 'DELETE' })
    expect(data).toBeUndefined()
  })

  it('throws ApiError carrying the status and response body on failure', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(new Response('playlist not found', { status: 404 })))

    await expect(apiFetch('/covers/missing')).rejects.toBeInstanceOf(ApiError)
    await expect(apiFetch('/covers/missing')).rejects.toMatchObject({
      name: 'ApiError',
      status: 404,
      message: 'playlist not found',
    })
  })

  it('parses RFC 7807 problem details into title and detail', async () => {
    fetchMock.mockImplementation(() =>
      Promise.resolve(
        jsonResponse(
          { type: 'about:blank', title: 'Unavailable', detail: 'Spotify is not responding.', status: 503 },
          { status: 503 },
        ),
      ),
    )

    await expect(apiFetch('/playlists')).rejects.toMatchObject({
      status: 503,
      title: 'Unavailable',
      detail: 'Spotify is not responding.',
      message: 'Spotify is not responding.',
    })
  })

  it('treats a blank problem detail as absent', async () => {
    fetchMock.mockImplementation(() =>
      Promise.resolve(
        jsonResponse({ type: 'about:blank', title: 'Not Found', detail: '', status: 404 }, { status: 404 }),
      ),
    )

    const err = (await apiFetch('/playlists').catch((e: unknown) => e)) as ApiError
    expect(err.detail).toBeUndefined()
    expect(err.title).toBe('Not Found')
    expect(err.message).toBe('Not Found')
  })

  it('flags 401 and 403 as unauthorized, and other statuses as not', () => {
    for (const [status, expected] of [
      [401, true],
      [403, true],
      [404, false],
      [500, false],
    ] as const) {
      expect(new ApiError(status, 'x').isUnauthorized).toBe(expected)
    }
  })
})

// Access tokens last 15 minutes, so an idle tab hits 401 constantly. Recovering
// transparently is what stops that reading as a random logout.
describe('apiFetch silent refresh', () => {
  let fetchMock: Mock

  beforeEach(() => {
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    clearCookies()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    clearCookies()
  })

  it('refreshes once on a 401 and replays the original request', async () => {
    fetchMock
      .mockResolvedValueOnce(unauthorized())
      .mockResolvedValueOnce(jsonResponse({ userId: 'u1' }))
      .mockResolvedValueOnce(jsonResponse([{ id: 'p1' }]))

    const data = await apiFetch<{ id: string }[]>('/playlists')

    expect(data).toEqual([{ id: 'p1' }])
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/v1/auth/refresh')
    expect(fetchMock.mock.calls[2]?.[0]).toBe('/api/v1/playlists')
  })

  it('gives up and throws the original 401 when the refresh fails', async () => {
    fetchMock.mockResolvedValueOnce(unauthorized()).mockResolvedValueOnce(unauthorized())

    await expect(apiFetch('/playlists')).rejects.toMatchObject({ status: 401 })
    // The original request must not be replayed against a dead session.
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('does not retry more than once', async () => {
    fetchMock
      .mockResolvedValueOnce(unauthorized())
      .mockResolvedValueOnce(jsonResponse({ userId: 'u1' }))
      .mockResolvedValueOnce(unauthorized())

    await expect(apiFetch('/playlists')).rejects.toMatchObject({ status: 401 })
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })

  // A 401 from sign-in means "wrong password", not "expired session". Refreshing
  // there would swallow the real error and confuse the form.
  it('does not refresh on a 401 from an auth endpoint', async () => {
    fetchMock.mockResolvedValue(unauthorized())

    await expect(apiFetch('/auth/signin', { method: 'POST', body: '{}' })).rejects.toMatchObject({ status: 401 })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('shares one refresh between requests that 401 together', async () => {
    let refreshCalls = 0
    fetchMock.mockImplementation((url: string) => {
      if (url.endsWith('/auth/refresh')) {
        refreshCalls += 1
        return Promise.resolve(jsonResponse({ userId: 'u1' }))
      }
      // Fail until the refresh has happened, then succeed.
      return Promise.resolve(refreshCalls === 0 ? unauthorized() : jsonResponse([]))
    })

    await Promise.all([apiFetch('/playlists'), apiFetch('/covers'), apiFetch('/covers?status=ready')])

    // A refresh per in-flight request would rotate the token three times and
    // trip the reuse detector, killing the session it was trying to save.
    expect(refreshCalls).toBe(1)
  })
})
