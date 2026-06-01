import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import type { ReactElement } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import { CoverGallery } from '@/features/covers/cover-gallery'
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

describe('CoverGallery', () => {
  it('shows six skeleton placeholders while loading', () => {
    mockFetch.mockReturnValue(new Promise(() => {})) // never resolves
    renderWithClient(<CoverGallery />)
    expect(screen.getAllByRole('listitem')).toHaveLength(6)
  })

  it('renders covers with status and palette once loaded', async () => {
    mockFetch.mockResolvedValue([
      {
        id: 'c1',
        status: 'ready',
        playlistId: 'pl1',
        imageUrl: 'https://cdn.test/c1.png',
        palette: [
          { dimension: 'energy', hexColor: '#ff5a36', weight: 0.8 },
          { dimension: 'valence', hexColor: '#4f86c6', weight: 0.4 },
        ],
      },
    ])
    renderWithClient(<CoverGallery />)

    const img = await screen.findByRole('img', {
      name: /generated cover for playlist pl1/i,
    })
    expect(img).toHaveAttribute('src', 'https://cdn.test/c1.png')
    expect(screen.getByText('ready')).toBeInTheDocument()
    // Each palette swatch carries a "<dimension> <pct>%" tooltip.
    expect(screen.getByTitle('energy 80%')).toBeInTheDocument()
  })

  it('shows an error state when covers fail to load', async () => {
    mockFetch.mockRejectedValue(new Error('boom'))
    renderWithClient(<CoverGallery />)
    expect(await screen.findByText(/couldn.t load covers/i)).toBeInTheDocument()
  })
})
