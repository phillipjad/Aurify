import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory, createRouter } from '@tanstack/react-router'

import { ThemeProvider } from '@/components/theme-provider'
import { NotFound, RouteError } from '@/components/route-fallbacks'
import { routeTree } from '@/routeTree.gen'
import { apiFetch } from '@/lib/api/client'

vi.mock('@/lib/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockFetch = apiFetch as unknown as Mock

// Builds the real app router (from the generated route tree) on a memory
// history so navigation is exercised end-to-end without a browser.
function renderApp(initialEntries: string[]) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries }),
    defaultNotFoundComponent: NotFound,
    defaultErrorComponent: RouteError,
  })
  render(
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </ThemeProvider>,
  )
}

beforeEach(() => {
  mockFetch.mockReset()
  mockFetch.mockResolvedValue([]) // playlists + covers default to empty
})

describe('app routing', () => {
  it('renders the home hero at /', async () => {
    renderApp(['/'])
    expect(
      await screen.findByRole('heading', {
        name: /cover art that captures the vibe/i,
      }),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Browse playlists' })).toBeInTheDocument()
  })

  it('navigates to the playlists page via the header nav', async () => {
    renderApp(['/'])

    // Wait for the router's initial render before interacting with the nav.
    const playlistsNav = await screen.findByRole('link', { name: 'Playlists' })
    await userEvent.click(playlistsNav)

    expect(await screen.findByRole('heading', { name: 'Your playlists' })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Spotify' })).toBeInTheDocument()
  })

  it('renders the covers page with its empty state and CTA link', async () => {
    renderApp(['/covers'])

    expect(await screen.findByText('No covers yet')).toBeInTheDocument()
    // The empty-state action is a real router <Link> to /playlists.
    expect(screen.getByRole('link', { name: 'Browse playlists' })).toBeInTheDocument()
  })

  it('shows the not-found page for an unknown URL', async () => {
    renderApp(['/no-such-page'])

    expect(await screen.findByText('Page not found')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /back home/i })).toBeInTheDocument()
  })
})
