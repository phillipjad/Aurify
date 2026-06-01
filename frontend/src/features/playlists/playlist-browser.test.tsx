import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import type { ReactElement } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import { PlaylistBrowser } from '@/features/playlists/playlist-browser'
import { apiFetch } from '@/lib/api/client'

vi.mock('@/lib/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockFetch = apiFetch as unknown as Mock

function renderWithClient(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

beforeEach(() => {
  mockFetch.mockReset()
})

describe('PlaylistBrowser', () => {
  it('renders the three DSP platforms with Spotify selected by default', async () => {
    mockFetch.mockResolvedValue([])
    renderWithClient(<PlaylistBrowser />)

    expect(screen.getByRole('button', { name: 'Spotify' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Apple Music' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'YouTube Music' })).toBeInTheDocument()

    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith('/playlists?platform=spotify'))
    expect(screen.getByRole('button', { name: 'Spotify' })).toHaveClass('bg-primary')
  })

  it("lists the active platform's playlists", async () => {
    mockFetch.mockResolvedValue([
      {
        id: 'sp1',
        platform: 'spotify',
        name: 'Morning Coffee',
        trackCount: 12,
      },
    ])
    renderWithClient(<PlaylistBrowser />)

    expect(await screen.findByText('Morning Coffee')).toBeInTheDocument()
    expect(screen.getByText('12 tracks')).toBeInTheDocument()
  })

  it('switches platform and re-queries when another DSP is chosen', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.includes('youtube_music')
        ? Promise.resolve([
            {
              id: 'yt1',
              platform: 'youtube_music',
              name: 'YT Mix',
              trackCount: 7,
            },
          ])
        : Promise.resolve([
            {
              id: 'sp1',
              platform: 'spotify',
              name: 'Morning Coffee',
              trackCount: 12,
            },
          ]),
    )
    renderWithClient(<PlaylistBrowser />)
    expect(await screen.findByText('Morning Coffee')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'YouTube Music' }))

    expect(await screen.findByText('YT Mix')).toBeInTheDocument()
    expect(mockFetch).toHaveBeenCalledWith('/playlists?platform=youtube_music')
    expect(screen.getByRole('button', { name: 'YouTube Music' })).toHaveClass('bg-primary')
  })

  it('shows an empty state when the platform has no playlists', async () => {
    mockFetch.mockResolvedValue([])
    renderWithClient(<PlaylistBrowser />)
    expect(await screen.findByText('No playlists found')).toBeInTheDocument()
  })

  it('shows a connect prompt when loading playlists fails', async () => {
    mockFetch.mockRejectedValue(new Error('boom'))
    renderWithClient(<PlaylistBrowser />)
    expect(await screen.findByText('Connect your Spotify account')).toBeInTheDocument()
  })

  it('requests a cover when "Aurify it" is clicked', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/playlists')
        ? Promise.resolve([
            {
              id: 'sp1',
              platform: 'spotify',
              name: 'Morning Coffee',
              trackCount: 12,
            },
          ])
        : Promise.resolve({ id: 'cover1', status: 'pending' }),
    )
    renderWithClient(<PlaylistBrowser />)

    const card = (await screen.findByText('Morning Coffee')).closest('li')
    await userEvent.click(within(card as HTMLElement).getByRole('button', { name: 'Aurify it' }))

    await waitFor(() =>
      expect(mockFetch).toHaveBeenCalledWith('/covers', {
        method: 'POST',
        body: JSON.stringify({ platform: 'spotify', playlistId: 'sp1' }),
      }),
    )
  })

  it('starts the OAuth flow when "Connect" is clicked', async () => {
    // Resolve an empty authUrl so the success handler skips jsdom navigation.
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/auth/') ? Promise.resolve({ authUrl: '' }) : Promise.resolve([]),
    )
    renderWithClient(<PlaylistBrowser />)

    await userEvent.click(screen.getByRole('button', { name: 'Connect Spotify' }))

    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith('/auth/spotify/login'))
  })
})
