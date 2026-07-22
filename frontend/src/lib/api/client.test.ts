import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'

import { ApiError, apiFetch } from '@/lib/api/client'

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

describe('apiFetch', () => {
  let fetchMock: Mock

  beforeEach(() => {
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('prefixes the API base URL and sends JSON + dev-user headers', async () => {
    fetchMock.mockResolvedValue(jsonResponse([{ id: 'p1' }]))

    const data = await apiFetch<{ id: string }[]>('/playlists?platform=spotify')

    expect(data).toEqual([{ id: 'p1' }])
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/playlists?platform=spotify')
    const headers = init.headers as Record<string, string>
    expect(headers['X-User-ID']).toBe('demo-user')
    expect(headers['Content-Type']).toBe('application/json')
  })

  it('merges caller init: method, body, and extra headers', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ id: 'c1' }))

    await apiFetch('/covers', {
      method: 'POST',
      body: JSON.stringify({ playlistId: 'p1' }),
      headers: { 'X-Trace': 'abc' },
    })

    const [, init] = fetchMock.mock.calls[0]
    expect(init.method).toBe('POST')
    expect(init.body).toBe('{"playlistId":"p1"}')
    const headers = init.headers as Record<string, string>
    expect(headers['X-Trace']).toBe('abc')
    // The defaults are still applied alongside caller headers.
    expect(headers['X-User-ID']).toBe('demo-user')
  })

  it('returns undefined for a 204 No Content response', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }))
    const data = await apiFetch('/covers/x', { method: 'DELETE' })
    expect(data).toBeUndefined()
  })

  it('throws ApiError carrying the status and response body on failure', async () => {
    // Fresh Response per call: its body stream can only be read once.
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
    // The live API returns exactly this on a missing DSP connection. An empty
    // string is not nullish, so leaving it intact rendered a blank message.
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

  it('flags 401 and 403 as unauthorized, and other statuses as not', async () => {
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
