import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { PlaylistBrowser } from '@/features/playlists/playlist-browser'
import { ApiError, apiFetch } from '@/lib/api/client'
import { renderWithProviders } from '@/test/render'

vi.mock('@/lib/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockFetch = apiFetch as unknown as Mock

const MORNING_COFFEE = {
  id: 'sp1',
  platform: 'spotify',
  name: 'Morning Coffee',
  description: 'Slow start',
  trackCount: 12,
}

beforeEach(() => {
  mockFetch.mockReset()
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

    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith('/playlists?platform=spotify'))
  })

  it("lists the active platform's playlists", async () => {
    mockFetch.mockResolvedValue([MORNING_COFFEE])
    renderWithProviders(<PlaylistBrowser />)

    expect(await screen.findByText('Morning Coffee')).toBeInTheDocument()
    expect(screen.getByText('12 tracks')).toBeInTheDocument()
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
    expect(mockFetch).toHaveBeenCalledWith('/playlists?platform=youtube_music')
    expect(screen.getByRole('radio', { name: 'YouTube Music' })).toHaveAttribute('aria-checked', 'true')
  })

  it('moves selection with arrow keys, per the radiogroup pattern', async () => {
    mockFetch.mockResolvedValue([])
    renderWithProviders(<PlaylistBrowser />)

    await userEvent.tab() // skip link / first stop is the picker's selected radio
    screen.getByRole('radio', { name: 'Spotify' }).focus()
    await userEvent.keyboard('{ArrowRight}')

    expect(screen.getByRole('radio', { name: 'Apple Music' })).toHaveAttribute('aria-checked', 'true')
    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith('/playlists?platform=apple_music'))
  })

  it('filters playlists by a search query', async () => {
    mockFetch.mockResolvedValue([
      MORNING_COFFEE,
      { id: 'sp2', platform: 'spotify', name: 'Gym Bangers', description: 'high energy', trackCount: 30 },
    ])
    renderWithProviders(<PlaylistBrowser />)
    await screen.findByText('Morning Coffee')

    await userEvent.type(screen.getByRole('searchbox', { name: /search playlists/i }), 'gym')

    expect(screen.getByText('Gym Bangers')).toBeInTheDocument()
    expect(screen.queryByText('Morning Coffee')).not.toBeInTheDocument()
  })

  it('shows a no-matches state when the search excludes everything', async () => {
    mockFetch.mockResolvedValue([MORNING_COFFEE])
    renderWithProviders(<PlaylistBrowser />)
    await screen.findByText('Morning Coffee')

    await userEvent.type(screen.getByRole('searchbox', { name: /search playlists/i }), 'zzz')

    expect(await screen.findByText('No matches')).toBeInTheDocument()
  })

  it('sorts playlists by track count when chosen', async () => {
    mockFetch.mockResolvedValue([
      { id: 'a', platform: 'spotify', name: 'Alpha', description: '', trackCount: 5 },
      { id: 'z', platform: 'spotify', name: 'Zeta', description: '', trackCount: 99 },
    ])
    renderWithProviders(<PlaylistBrowser />)
    await screen.findByText('Alpha')

    // Default sort is by name (Alpha first); switching to Tracks puts Zeta first.
    await userEvent.selectOptions(screen.getByRole('combobox', { name: /sort/i }), 'tracks')

    const names = screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent)
    expect(names).toEqual(['Zeta', 'Alpha'])
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
    // Resolve an empty authUrl so the success handler skips jsdom navigation.
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/auth/') ? Promise.resolve({ authUrl: '' }) : Promise.resolve([]),
    )
    renderWithProviders(<PlaylistBrowser />)

    await userEvent.click(await screen.findByRole('button', { name: 'Connect Spotify' }))

    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith('/auth/spotify/login'))
  })
})
