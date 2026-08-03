import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { PlaylistBrowser } from '@/features/playlists/playlist-browser'
import { ApiError, apiFetch } from '@/lib/api/client'
import { connectDsp } from '@/lib/api/commands'
import { renderWithProviders } from '@/test/render'

vi.mock('@/lib/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

// connectDsp leaves the app, which jsdom cannot do. Stubbing it keeps the
// assertion on "did we send the browser to the right place".
vi.mock('@/lib/api/commands', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/commands')>()
  return { ...actual, connectDsp: vi.fn() }
})

const mockFetch = apiFetch as unknown as Mock
const mockConnectDsp = connectDsp as unknown as Mock

const MORNING_COFFEE = {
  id: 'sp1',
  platform: 'spotify',
  name: 'Morning Coffee',
  description: 'Slow start',
  trackCount: 12,
}

beforeEach(() => {
  mockFetch.mockReset()
  mockConnectDsp.mockReset()
  // The last-used platform is remembered in real localStorage, which jsdom shares
  // across tests in a file. Left over, it silently changes which platform a test
  // starts on.
  localStorage.clear()
})

describe('PlaylistBrowser', () => {
  it('exposes the platforms as a radiogroup with Spotify selected by default', async () => {
    mockFetch.mockResolvedValue([])
    renderWithProviders(<PlaylistBrowser />)

    // findBy: the router resolves its first render a tick after mount.
    const group = await screen.findByRole('radiogroup', { name: 'Music platform' })
    expect(within(group).getAllByRole('radio')).toHaveLength(3)

    // Selection is announced, not just colored.
    expect(screen.getByRole('radio', { name: 'Spotify' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: 'Apple Music' })).toHaveAttribute('aria-checked', 'false')

    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('platform=spotify')))
  })

  it("lists the active platform's playlists", async () => {
    mockFetch.mockResolvedValue([MORNING_COFFEE])
    renderWithProviders(<PlaylistBrowser />)

    expect(await screen.findByText('Morning Coffee')).toBeInTheDocument()
    expect(screen.getByText('12 tracks')).toBeInTheDocument()
  })

  // Windowing: the point is that a long list costs a fixed amount of DOM. The
  // full height is still reserved so the scrollbar tells the truth, and
  // aria-setsize carries the real total, which a partial list cannot convey.
  it('renders only a slice of a long list while reporting its true size', async () => {
    const many = Array.from({ length: 200 }, (_, i) => ({
      id: `p${i}`,
      platform: 'spotify',
      name: `Playlist ${i}`,
      description: '',
      trackCount: i,
    }))
    mockFetch.mockResolvedValue(many)
    renderWithProviders(<PlaylistBrowser />)
    await screen.findByText('Playlist 0')

    const rendered = screen.getAllByRole('listitem')
    expect(rendered.length).toBeGreaterThanOrEqual(5)
    expect(rendered.length).toBeLessThan(60)
    // Far down the list, so it must not have been rendered.
    expect(screen.queryByText('Playlist 199')).not.toBeInTheDocument()

    // Every rendered row still announces its place in the whole set.
    expect(rendered[0]).toHaveAttribute('aria-setsize', '200')
    expect(rendered[0]).toHaveAttribute('aria-posinset', '1')

    // And the container reserves room for all 200, so scrolling is not truncated.
    const list = screen.getByRole('list', { name: 'Playlists' })
    expect(Number.parseInt(list.style.height, 10)).toBeGreaterThan(200 * 40)
  })

  it('switches platform and re-queries when another DSP is chosen', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.includes('youtube_music')
        ? Promise.resolve([{ id: 'yt1', platform: 'youtube_music', name: 'YT Mix', description: '', trackCount: 7 }])
        : Promise.resolve([MORNING_COFFEE]),
    )
    renderWithProviders(<PlaylistBrowser />)
    expect(await screen.findByText('Morning Coffee')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('radio', { name: 'YouTube Music' }))

    expect(await screen.findByText('YT Mix')).toBeInTheDocument()
    expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('platform=youtube_music'))
    expect(screen.getByRole('radio', { name: 'YouTube Music' })).toHaveAttribute('aria-checked', 'true')
  })

  // Results are held across a search or sort so typing doesn't flash skeletons,
  // but holding them across a platform change left one service's playlists on
  // screen under another's tab, labelled as something they are not.
  it("drops the previous platform's playlists as soon as the tab changes", async () => {
    let releaseYouTube: (playlists: unknown) => void = () => {}
    mockFetch.mockImplementation((path: string) => {
      if (path.startsWith('/auth/session')) return Promise.resolve({ userId: 'u1', connections: [] })
      if (path.includes('youtube_music')) {
        return new Promise((resolve) => {
          releaseYouTube = resolve
        })
      }
      return Promise.resolve([MORNING_COFFEE])
    })
    renderWithProviders(<PlaylistBrowser />)
    expect(await screen.findByText('Morning Coffee')).toBeInTheDocument()

    // YouTube Music's request is deliberately left in flight, which is the
    // window the stale results used to be visible in.
    await userEvent.click(screen.getByRole('radio', { name: 'YouTube Music' }))

    await waitFor(() => expect(screen.queryByText('Morning Coffee')).not.toBeInTheDocument())

    releaseYouTube([{ id: 'yt1', platform: 'youtube_music', name: 'YT Mix', description: '', trackCount: 7 }])
    expect(await screen.findByText('YT Mix')).toBeInTheDocument()
  })

  // The reason this state moved into the URL: useState died on unmount, so
  // returning from another page dropped the user back on the default.
  it('puts the selected platform in the URL so it survives navigation', async () => {
    mockFetch.mockResolvedValue([])
    const { router } = renderWithProviders(<PlaylistBrowser />)
    await screen.findByRole('radiogroup', { name: 'Music platform' })

    await userEvent.click(screen.getByRole('radio', { name: 'YouTube Music' }))

    await waitFor(() => expect(router.state.location.search).toMatchObject({ platform: 'youtube_music' }))
  })

  it('restores platform and sort from the URL on a cold load', async () => {
    // Non-empty, since the search and sort controls only render alongside a list.
    mockFetch.mockResolvedValue([MORNING_COFFEE])
    renderWithProviders(<PlaylistBrowser />, { path: '/playlists?platform=apple_music&sort=tracks' })
    // The controls only appear once the list has resolved.
    await screen.findByText('Morning Coffee')

    expect(screen.getByRole('radio', { name: 'Apple Music' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('combobox', { name: /sort/i })).toHaveValue('tracks')
    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('platform=apple_music')))
  })

  // A URL is typed by anyone, so an unknown platform must not reach the request.
  it('falls back to the default when the URL names a platform that does not exist', async () => {
    mockFetch.mockResolvedValue([MORNING_COFFEE])
    renderWithProviders(<PlaylistBrowser />, { path: '/playlists?platform=napster&sort=sideways' })
    await screen.findByText('Morning Coffee')

    expect(screen.getByRole('radio', { name: 'Spotify' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('combobox', { name: /sort/i })).toHaveValue('name')
  })

  // The header's Playlists link carries no search params, so without a remembered
  // value every trip through the nav bar landed back on the default.
  it('reopens on the last platform used when the URL says nothing', async () => {
    mockFetch.mockResolvedValue([])
    const first = renderWithProviders(<PlaylistBrowser />)
    await screen.findByRole('radiogroup', { name: 'Music platform' })
    await userEvent.click(screen.getByRole('radio', { name: 'YouTube Music' }))
    await waitFor(() => expect(first.router.state.location.search).toMatchObject({ platform: 'youtube_music' }))
    first.unmount()

    // A fresh mount at a bare /playlists, as the nav link produces.
    renderWithProviders(<PlaylistBrowser />)

    expect(await screen.findByRole('radio', { name: 'YouTube Music' })).toHaveAttribute('aria-checked', 'true')
  })

  // An explicit platform in the URL has to beat the remembered one, or a shared
  // link would show whatever the recipient looked at last.
  it('lets the URL override the remembered platform', async () => {
    mockFetch.mockResolvedValue([])
    localStorage.setItem('aurify.playlists.platform', 'youtube_music')

    renderWithProviders(<PlaylistBrowser />, { path: '/playlists?platform=apple_music' })

    expect(await screen.findByRole('radio', { name: 'Apple Music' })).toHaveAttribute('aria-checked', 'true')
  })

  it('moves selection with arrow keys, per the radiogroup pattern', async () => {
    mockFetch.mockResolvedValue([])
    renderWithProviders(<PlaylistBrowser />)

    await userEvent.tab() // skip link / first stop is the picker's selected radio
    screen.getByRole('radio', { name: 'Spotify' }).focus()
    await userEvent.keyboard('{ArrowRight}')

    await waitFor(() =>
      expect(screen.getByRole('radio', { name: 'Apple Music' })).toHaveAttribute('aria-checked', 'true'),
    )
    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('platform=apple_music')))
  })

  it('searches on the server: the request carries q, and the result replaces the list', async () => {
    // The mock keys off the q param, so search is exercised server-side rather
    // than by trimming a client-loaded list.
    mockFetch.mockImplementation((path: string) => {
      const q = new URL(`http://x${path}`).searchParams.get('q')
      if (q === 'gym') {
        return Promise.resolve([
          { id: 'sp2', platform: 'spotify', name: 'Gym Bangers', description: '', trackCount: 30 },
        ])
      }
      return Promise.resolve([MORNING_COFFEE])
    })
    renderWithProviders(<PlaylistBrowser />)
    await screen.findByText('Morning Coffee')

    await userEvent.type(screen.getByRole('searchbox', { name: /search playlists/i }), 'gym')

    expect(await screen.findByText('Gym Bangers')).toBeInTheDocument()
    expect(screen.queryByText('Morning Coffee')).not.toBeInTheDocument()
    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('q=gym')))
  })

  // The reported bug: typing a matching term straight after an unmatched one
  // briefly read "No playlists match <matching term>", because the message
  // interpolated the live input while the list still held the previous term's
  // empty results. A debounce already existed; the pair was mismatched.
  it('never names a term the results on screen do not belong to', async () => {
    mockFetch.mockImplementation((path: string) => {
      const q = new URL(`http://x${path}`).searchParams.get('q')
      if (q === 'zzz') return Promise.resolve([])
      if (q === 'gym') {
        return Promise.resolve([
          { id: 'sp2', platform: 'spotify', name: 'Gym Bangers', description: '', trackCount: 3 },
        ])
      }
      return Promise.resolve([MORNING_COFFEE])
    })
    renderWithProviders(<PlaylistBrowser />)
    const box = await screen.findByRole('searchbox', { name: /search playlists/i })

    await userEvent.type(box, 'zzz')
    expect(await screen.findByText(/no playlists match .zzz./i)).toBeInTheDocument()

    // Straight from an unmatched term to a matching one.
    await userEvent.clear(box)
    await userEvent.type(box, 'gym')

    // At no point may a verdict name "gym" while the empty "zzz" results are up.
    expect(screen.queryByText(/no playlists match .gym./i)).not.toBeInTheDocument()
    expect(await screen.findByText('Gym Bangers')).toBeInTheDocument()
  })

  it('shows a no-matches state when the server returns nothing for the search', async () => {
    mockFetch.mockImplementation((path: string) => {
      const q = new URL(`http://x${path}`).searchParams.get('q')
      return Promise.resolve(q ? [] : [MORNING_COFFEE])
    })
    renderWithProviders(<PlaylistBrowser />)
    await screen.findByText('Morning Coffee')

    await userEvent.type(screen.getByRole('searchbox', { name: /search playlists/i }), 'zzz')

    expect(await screen.findByText('No matches')).toBeInTheDocument()
  })

  it('sorts on the server: the request carries sort, and the server order is honored', async () => {
    const alpha = { id: 'a', platform: 'spotify', name: 'Alpha', description: '', trackCount: 5 }
    const zeta = { id: 'z', platform: 'spotify', name: 'Zeta', description: '', trackCount: 99 }
    mockFetch.mockImplementation((path: string) => {
      const sort = new URL(`http://x${path}`).searchParams.get('sort')
      return Promise.resolve(sort === 'tracks' ? [zeta, alpha] : [alpha, zeta])
    })
    renderWithProviders(<PlaylistBrowser />)
    await screen.findByText('Alpha')

    await userEvent.selectOptions(screen.getByRole('combobox', { name: /sort/i }), 'tracks')

    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('sort=tracks')))
    await waitFor(() => {
      const names = screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent)
      expect(names).toEqual(['Zeta', 'Alpha'])
    })
  })

  it('shows an empty state when the platform has no playlists', async () => {
    mockFetch.mockResolvedValue([])
    renderWithProviders(<PlaylistBrowser />)
    expect(await screen.findByText('No playlists found')).toBeInTheDocument()
  })

  it('prompts to connect only when the API says the account is unauthorized', async () => {
    mockFetch.mockRejectedValue(new ApiError(401, 'no token', 'Unauthorized', 'Link your Spotify account.'))
    renderWithProviders(<PlaylistBrowser />)

    expect(await screen.findByText('Connect your Spotify account')).toBeInTheDocument()
  })

  it('reports a generic failure with a retry instead of blaming the connection', async () => {
    mockFetch.mockRejectedValue(new Error('boom'))
    renderWithProviders(<PlaylistBrowser />)

    expect(await screen.findByText(/couldn.t load your playlists/i)).toBeInTheDocument()
    // The old UI showed "Connect your Spotify account" for every failure.
    expect(screen.queryByText('Connect your Spotify account')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument()
  })

  it("surfaces the API's problem detail when a request fails", async () => {
    mockFetch.mockRejectedValue(new ApiError(503, 'unavailable', 'Unavailable', 'Spotify is not responding.'))
    renderWithProviders(<PlaylistBrowser />)

    expect(await screen.findByText('Spotify is not responding.')).toBeInTheDocument()
  })

  it('requests a cover when "Aurify it" is clicked', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/playlists')
        ? Promise.resolve([MORNING_COFFEE])
        : Promise.resolve({ id: 'cover1', status: 'pending' }),
    )
    renderWithProviders(<PlaylistBrowser />)

    const row = (await screen.findByText('Morning Coffee')).closest('li')
    await userEvent.click(within(row as HTMLElement).getByRole('button', { name: 'Aurify it' }))

    await waitFor(() =>
      expect(mockFetch).toHaveBeenCalledWith('/covers', {
        method: 'POST',
        body: JSON.stringify({ platform: 'spotify', playlistId: 'sp1' }),
      }),
    )
  })

  it('confirms a started generation on the row that triggered it', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/playlists')
        ? Promise.resolve([MORNING_COFFEE, { ...MORNING_COFFEE, id: 'sp2', name: 'Late Night' }])
        : Promise.resolve({ id: 'cover1', status: 'pending' }),
    )
    renderWithProviders(<PlaylistBrowser />)

    const row = (await screen.findByText('Morning Coffee')).closest('li') as HTMLElement
    await userEvent.click(within(row).getByRole('button', { name: 'Aurify it' }))

    // Feedback lands on the acted-on row only — the other row stays untouched.
    expect(await within(row).findByText(/cover started/i)).toBeInTheDocument()
    const other = screen.getByText('Late Night').closest('li') as HTMLElement
    expect(within(other).queryByText(/cover started/i)).not.toBeInTheDocument()
  })

  // The reported bug: a second click blanked the first row's spinner, because
  // every row read one shared mutation result that only described the most recent
  // click. The first request was still running with nothing on screen to say so.
  it('runs several generations at once and reports each on its own row', async () => {
    const release: Record<string, (cover: unknown) => void> = {}
    mockFetch.mockImplementation((path: string, init?: { body?: string }) => {
      if (path.startsWith('/playlists')) {
        return Promise.resolve([MORNING_COFFEE, { ...MORNING_COFFEE, id: 'sp2', name: 'Late Night' }])
      }
      const { playlistId } = JSON.parse(init?.body ?? '{}') as { playlistId: string }
      return new Promise((resolve) => {
        release[playlistId] = resolve
      })
    })
    renderWithProviders(<PlaylistBrowser />)

    const first = (await screen.findByText('Morning Coffee')).closest('li') as HTMLElement
    const second = screen.getByText('Late Night').closest('li') as HTMLElement

    await userEvent.click(within(first).getByRole('button', { name: 'Aurify it' }))
    await userEvent.click(within(second).getByRole('button', { name: 'Aurify it' }))

    // Both are in flight: the second click must not have cancelled the first's
    // reporting.
    await waitFor(() => {
      expect(within(first).getByRole('button')).toHaveAttribute('aria-busy', 'true')
      expect(within(second).getByRole('button')).toHaveAttribute('aria-busy', 'true')
    })

    // Finish the first only. It reports success while the second keeps working.
    release.sp1?.({ id: 'cover1', status: 'pending' })

    expect(await within(first).findByText(/cover started/i)).toBeInTheDocument()
    expect(within(second).getByRole('button')).toHaveAttribute('aria-busy', 'true')
    expect(within(second).queryByText(/cover started/i)).not.toBeInTheDocument()
  })

  it('reports a failed generation on the row that triggered it', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/playlists')
        ? Promise.resolve([MORNING_COFFEE])
        : Promise.reject(new ApiError(502, 'upstream', 'Bad Gateway', 'The image service is down.')),
    )
    renderWithProviders(<PlaylistBrowser />)

    const row = (await screen.findByText('Morning Coffee')).closest('li') as HTMLElement
    await userEvent.click(within(row).getByRole('button', { name: 'Aurify it' }))

    expect(await within(row).findByRole('alert')).toHaveTextContent('The image service is down.')
  })

  it('starts the OAuth flow when "Connect" is clicked', async () => {
    mockFetch.mockResolvedValue([])
    renderWithProviders(<PlaylistBrowser />)

    await userEvent.click(await screen.findByRole('button', { name: 'Connect Spotify' }))

    // A top-level navigation, not a fetch: the API redirects to the provider and
    // the callback redirects back, so the SPA has no response to hold.
    expect(mockConnectDsp).toHaveBeenCalledWith('spotify')
  })

  // An empty playlist list is not the same answer as "not connected", so the
  // session's connection list says which it is rather than leaving the user to
  // infer it from whether anything appeared.
  it('reports a linked platform instead of offering to connect it', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/auth/session')
        ? Promise.resolve({ userId: 'u1', email: 'a@b.test', connections: ['youtube_music'] })
        : Promise.resolve([]),
    )
    renderWithProviders(<PlaylistBrowser />, { path: '/playlists?connected=youtube_music' })

    expect(await screen.findByText(/youtube music connected/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Connect YouTube Music' })).not.toBeInTheDocument()
    // Re-granting a revoked authorization stays reachable.
    expect(screen.getByRole('button', { name: 'Reconnect' })).toBeInTheDocument()
  })

  // The session already knows which platforms are linked, so landing on one the
  // user has never connected and being told to "Connect Spotify" reads as the
  // connection having failed. Bites on any device with no remembered preference.
  it('opens on a connected platform when nothing else picks one', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/auth/session')
        ? Promise.resolve({ userId: 'u1', email: 'a@b.test', connections: ['youtube_music'] })
        : Promise.resolve([]),
    )
    renderWithProviders(<PlaylistBrowser />)

    expect(await screen.findByText(/youtube music connected/i)).toBeInTheDocument()
    expect(await screen.findByRole('radio', { name: 'YouTube Music' })).toHaveAttribute('aria-checked', 'true')
  })

  // An explicit choice still wins over a connected one, so the picker keeps
  // working for a platform the user is about to link.
  it('still offers to connect a platform that is not linked', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/auth/session')
        ? Promise.resolve({ userId: 'u1', email: 'a@b.test', connections: ['youtube_music'] })
        : Promise.resolve([]),
    )
    renderWithProviders(<PlaylistBrowser />, { path: '/playlists?platform=spotify' })

    expect(await screen.findByRole('button', { name: 'Connect Spotify' })).toBeInTheDocument()
    expect(screen.queryByText(/spotify connected/i)).not.toBeInTheDocument()
  })

  it('opens on the platform the callback just connected', async () => {
    mockFetch.mockResolvedValue([])
    renderWithProviders(<PlaylistBrowser />, { path: '/playlists?connected=youtube_music' })

    expect(await screen.findByRole('radio', { name: 'YouTube Music' })).toHaveAttribute('aria-checked', 'true')
    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('platform=youtube_music')))
  })

  // The bug this scoping exists for: a failed YouTube Music connection used to
  // keep reporting itself after a switch to Spotify, blaming a platform the user
  // never attempted.
  it('reports a failed connection against the platform it belongs to, and only that one', async () => {
    mockFetch.mockResolvedValue([])
    renderWithProviders(<PlaylistBrowser />, {
      path: '/playlists?platform=youtube_music&connect_error=cancelled',
    })

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(/youtube music/i)

    await userEvent.click(screen.getByRole('radio', { name: 'Spotify' }))

    // Switching platform strips connect_error from the URL, so the outcome cannot
    // follow the switch and be reported against a platform never attempted.
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
  })
})
