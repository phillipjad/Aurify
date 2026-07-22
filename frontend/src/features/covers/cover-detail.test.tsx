import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CoverDetail } from '@/features/covers/cover-detail'
import { ApiError, apiFetch } from '@/lib/api/client'
import { renderWithProviders } from '@/test/render'

vi.mock('@/lib/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockFetch = apiFetch as unknown as Mock

const COVER = {
  id: 'c1',
  status: 'ready',
  platform: 'spotify',
  playlistId: 'pl1',
  playlistName: 'Morning Coffee',
  imageUrl: 'https://cdn.test/c1.png',
  prompt: 'A warm abstract sunrise in oranges and violets.',
  createdAt: '2026-01-02T00:00:00Z',
  palette: [
    { dimension: 'organic', hexColor: '#7fb069', weight: 0.42 },
    { dimension: 'melancholic', hexColor: '#5c4d7d', weight: 0.31 },
    { dimension: 'introspective', hexColor: '#4f86c6', weight: 0.27 },
  ],
}

// Route GET /covers/:id to the cover; POST and DELETE resolve like the API.
function routeApi(cover: unknown = COVER) {
  mockFetch.mockImplementation((_path: string, init?: RequestInit) => {
    if (init?.method === 'DELETE') return Promise.resolve(undefined)
    if (init?.method === 'POST') return Promise.resolve({ ...COVER, id: 'c2', status: 'pending' })
    return Promise.resolve(cover)
  })
}

beforeEach(() => {
  mockFetch.mockReset()
})

describe('CoverDetail', () => {
  it('renders the full palette breakdown and the prompt', async () => {
    routeApi()
    renderWithProviders(<CoverDetail coverId="c1" />)

    expect(await screen.findByRole('heading', { level: 1, name: 'Morning Coffee' })).toBeInTheDocument()
    // Every dimension is shown, not just the top few.
    for (const dim of ['organic', 'melancholic', 'introspective']) {
      expect(screen.getByText(dim)).toBeInTheDocument()
    }
    expect(screen.getByText('42%')).toBeInTheDocument()
    expect(screen.getByText('A warm abstract sunrise in oranges and violets.')).toBeInTheDocument()
  })

  it('confirms before deleting, then calls DELETE', async () => {
    routeApi()
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    // First click only reveals the confirmation — nothing is deleted yet.
    await userEvent.click(screen.getByRole('button', { name: /delete/i }))
    expect(screen.getByText(/delete this cover\?/i)).toBeInTheDocument()
    expect(mockFetch).not.toHaveBeenCalledWith('/covers/c1', { method: 'DELETE' })

    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(mockFetch).toHaveBeenCalledWith('/covers/c1', { method: 'DELETE' })
  })

  it('can back out of a delete', async () => {
    routeApi()
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    await userEvent.click(screen.getByRole('button', { name: /delete/i }))
    await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(screen.queryByText(/delete this cover\?/i)).not.toBeInTheDocument()
    expect(mockFetch).not.toHaveBeenCalledWith('/covers/c1', { method: 'DELETE' })
  })

  it('regenerates by generating a fresh cover for the same playlist', async () => {
    routeApi()
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    await userEvent.click(screen.getByRole('button', { name: /regenerate/i }))
    expect(mockFetch).toHaveBeenCalledWith('/covers', {
      method: 'POST',
      body: JSON.stringify({ platform: 'spotify', playlistId: 'pl1' }),
    })
  })

  it('explains a deleted / missing cover instead of erroring blank', async () => {
    mockFetch.mockRejectedValue(new ApiError(404, 'not found', 'Not Found'))
    renderWithProviders(<CoverDetail coverId="gone" />)

    expect(await screen.findByText(/doesn.t exist/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /all covers/i })).toHaveAttribute('href', '/covers')
  })

  it('offers a download link for a ready cover', async () => {
    routeApi()
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    const download = screen.getByRole('link', { name: /download/i })
    expect(download).toHaveAttribute('href', 'https://cdn.test/c1.png')
    expect(download).toHaveAttribute('download')
  })
})
