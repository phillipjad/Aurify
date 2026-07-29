import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CoverGallery } from '@/features/covers/cover-gallery'
import { ApiError, apiFetch } from '@/lib/api/client'
import { renderWithProviders } from '@/test/render'

vi.mock('@/lib/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockFetch = apiFetch as unknown as Mock

const READY_COVER = {
  id: 'c1',
  status: 'ready',
  platform: 'spotify',
  playlistId: 'pl1',
  playlistName: 'Morning Coffee',
  imageUrl: 'https://cdn.test/c1.png',
  createdAt: '2026-01-01T00:00:00Z',
  palette: [
    { dimension: 'energetic', hexColor: '#ff5a36', weight: 0.8 },
    { dimension: 'introspective', hexColor: '#4f86c6', weight: 0.4 },
  ],
}

beforeEach(() => {
  mockFetch.mockReset()
})

describe('CoverGallery', () => {
  // Windowing: a long list costs a fixed amount of DOM, while the reported size
  // and the reserved height still describe the whole set.
  it('renders only a slice of a long covers list', async () => {
    const many = Array.from({ length: 90 }, (_, i) => ({ ...READY_COVER, id: `c${i}`, playlistName: `Cover ${i}` }))
    mockFetch.mockResolvedValue(many)
    renderWithProviders(<CoverGallery />)
    await screen.findByRole('heading', { name: 'Cover 0' })

    const tiles = screen.getAllByRole('listitem')
    expect(tiles.length).toBeLessThan(30)
    expect(screen.queryByRole('heading', { name: 'Cover 89' })).not.toBeInTheDocument()
    expect(tiles[0]).toHaveAttribute('aria-setsize', '90')

    const list = screen.getByRole('list', { name: 'Covers' })
    expect(Number.parseInt(list.style.height, 10)).toBeGreaterThan(90 * 40)
  })

  it('shows six skeleton placeholders while loading', async () => {
    mockFetch.mockReturnValue(new Promise(() => {})) // never resolves
    renderWithProviders(<CoverGallery />)
    // findAll: the router resolves its first render a tick after mount.
    expect(await screen.findAllByRole('listitem')).toHaveLength(6)
  })

  it('identifies each cover by its playlist name', async () => {
    mockFetch.mockResolvedValue([READY_COVER])
    renderWithProviders(<CoverGallery />)

    const img = await screen.findByRole('img', { name: /cover generated for morning coffee/i })
    expect(img).toHaveAttribute('src', 'https://cdn.test/c1.png')
    expect(screen.getByRole('heading', { name: 'Morning Coffee' })).toBeInTheDocument()
  })

  it('renders the status in plain language', async () => {
    mockFetch.mockResolvedValue([READY_COVER])
    renderWithProviders(<CoverGallery />)
    // Scope to the tile so we read the status badge, not the "Ready" filter chip.
    const tile = (await screen.findByRole('heading', { name: 'Morning Coffee' })).closest(
      '[role="listitem"]',
    ) as HTMLElement
    expect(within(tile).getByText('Ready')).toBeInTheDocument()
  })

  it('filters by status on the server (the request carries the status param)', async () => {
    // The mock keys off the status query param, so the filter is exercised
    // server-side, not by trimming a client-loaded list.
    mockFetch.mockImplementation((path: string) => {
      const status = new URL(`http://x${path}`).searchParams.get('status')
      if (status === 'failed') {
        return Promise.resolve([
          { ...READY_COVER, id: 'f1', playlistName: 'Failed One', status: 'failed', imageUrl: undefined, error: 'x' },
        ])
      }
      return Promise.resolve([{ ...READY_COVER, id: 'r1', playlistName: 'Ready One', status: 'ready' }])
    })
    renderWithProviders(<CoverGallery />)
    await screen.findByRole('heading', { name: 'Ready One' })

    await userEvent.click(screen.getByRole('button', { name: 'Failed', pressed: false }))

    expect(await screen.findByRole('heading', { name: 'Failed One' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Ready One' })).not.toBeInTheDocument()
    expect(mockFetch).toHaveBeenCalledWith('/covers?limit=20&offset=0&status=failed')
  })

  it('pages through covers: a full first page reveals Load more, which fetches the next offset', async () => {
    const page = (start: number, count: number) =>
      Array.from({ length: count }, (_, i) => ({
        ...READY_COVER,
        id: `c${start + i}`,
        playlistName: `Playlist ${start + i}`,
      }))
    mockFetch.mockImplementation((path: string) => {
      const offset = Number(new URL(`http://x${path}`).searchParams.get('offset') ?? '0')
      // First page is full (20) so more exist; the second page is short (ends it).
      return Promise.resolve(offset === 0 ? page(0, 20) : page(20, 3))
    })
    renderWithProviders(<CoverGallery />)

    await screen.findByRole('heading', { name: 'Playlist 0' })
    expect(mockFetch).toHaveBeenCalledWith('/covers?limit=20&offset=0')

    await userEvent.click(await screen.findByRole('button', { name: /load more/i }))

    // The 23rd cover is deliberately outside the rendered window, so paging is
    // asserted on the request and on the size the grid reports, not on a tile that
    // windowing is supposed to leave out.
    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith('/covers?limit=20&offset=20'))
    await waitFor(() => expect(screen.getAllByRole('listitem')[0]).toHaveAttribute('aria-setsize', '23'))
    // Short second page → no further Load more.
    expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument()
  })

  it('renders the palette as readable text, not a hover-only tooltip', async () => {
    mockFetch.mockResolvedValue([READY_COVER])
    renderWithProviders(<CoverGallery />)

    // Dimension and weight are visible content now; the old UI hid them in title=.
    expect(await screen.findByText('energetic')).toBeInTheDocument()
    expect(screen.getByText('80%')).toBeInTheDocument()
    expect(document.querySelector('[title]')).toBeNull()
  })

  it('shows a failed cover’s error, and links the tile to its detail (retry lives there)', async () => {
    mockFetch.mockResolvedValue([
      { ...READY_COVER, status: 'failed', imageUrl: undefined, error: 'The image service timed out.' },
    ])
    renderWithProviders(<CoverGallery />)

    const tile = (await screen.findByRole('heading', { name: 'Morning Coffee' })).closest(
      '[role="listitem"]',
    ) as HTMLElement
    expect(within(tile).getByText('The image service timed out.')).toBeInTheDocument()
    // The gallery no longer carries an inline retry; the whole tile is a link to
    // the detail view, where regenerate/delete/download live.
    expect(within(tile).queryByRole('button')).not.toBeInTheDocument()
    expect(within(tile).getByRole('link')).toHaveAttribute('href', '/covers/c1')
  })

  it("surfaces the API's problem detail when covers fail to load", async () => {
    mockFetch.mockRejectedValue(new ApiError(503, 'unavailable', 'Unavailable', 'Storage is unreachable.'))
    renderWithProviders(<CoverGallery />)

    expect(await screen.findByText(/couldn.t load your covers/i)).toBeInTheDocument()
    expect(screen.getByText('Storage is unreachable.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument()
  })
})
