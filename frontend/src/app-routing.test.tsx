import { beforeEach, describe, expect, it, vi, type Mock } from 'vite-plus/test'
import { cleanup, render, screen, within } from '@testing-library/react'
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

  it('announces the new page and moves focus to main on navigation', async () => {
    renderApp(['/'])
    await userEvent.click(await screen.findByRole('link', { name: 'Playlists' }))
    await screen.findByRole('heading', { name: 'Your playlists' })

    // Screen-reader users get told the page changed, and focus lands in the
    // new content instead of staying on the (now-stale) nav link.
    expect(screen.getByRole('status')).toHaveTextContent(/playlists/i)
    expect(document.getElementById('main')).toHaveFocus()
  })

  it('remounts the route wrapper on navigation so the entrance replays', async () => {
    renderApp(['/'])
    await screen.findByRole('heading', { name: /cover art that captures the vibe/i })
    const before = document.querySelector('.route-enter')

    await userEvent.click(await screen.findByRole('link', { name: 'Playlists' }))
    await screen.findByRole('heading', { name: 'Your playlists' })

    // A CSS animation only replays on a fresh element, which is what the
    // pathname key buys. Drop the key and this node would be reused.
    const after = document.querySelector('.route-enter')
    expect(after).toBeInTheDocument()
    expect(after).not.toBe(before)
  })

  it('keeps the footer to one row of links, with security among them', async () => {
    renderApp(['/'])

    const footer = await screen.findByRole('navigation', { name: 'Footer' })
    expect(
      within(footer)
        .getAllByRole('link')
        .map((link) => link.textContent),
    ).toEqual(['About', 'Privacy', 'Security', 'Contact'])
    // The pitch that used to sit above these links now lives on /security only.
    expect(screen.queryByText(/security is a focus here/i)).not.toBeInTheDocument()
  })

  it('returns the scroll container to the top on navigation', async () => {
    renderApp(['/'])
    await screen.findByRole('link', { name: 'Playlists' })

    // <main> is the only scroller now, and an element keeps its scrollTop across
    // a route change, so the layout has to put it back itself.
    const main = document.getElementById('main') as HTMLElement
    const scrollTo = vi.spyOn(main, 'scrollTo')

    await userEvent.click(screen.getByRole('link', { name: 'Playlists' }))
    await screen.findByRole('heading', { name: 'Your playlists' })

    expect(scrollTo).toHaveBeenCalledWith({ top: 0 })
    scrollTo.mockRestore()
  })

  it('renders the covers page with its empty state and CTA link', async () => {
    renderApp(['/covers'])

    expect(await screen.findByText('No covers yet')).toBeInTheDocument()
    // The empty-state action is a real router <Link> to /playlists.
    expect(screen.getByRole('link', { name: 'Browse playlists' })).toBeInTheDocument()
    // Claims exactly one screen so the grid scrolls, not the page (__root.tsx
    // keys its grid row off this attribute).
    expect(document.querySelector('[data-fills-shell]')).toBeInTheDocument()
  })

  it('sends a signed-in user away from sign-in and sign-up', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/auth/session')
        ? Promise.resolve({ userId: 'u1', email: 'ada@example.com' })
        : Promise.resolve([]),
    )

    renderApp(['/sign-in'])
    // Home, not the form: signing in again is not something to offer.
    expect(await screen.findByRole('heading', { name: /cover art that captures the vibe/i })).toBeInTheDocument()
    expect(screen.queryByLabelText('Email')).not.toBeInTheDocument()

    cleanup()
    renderApp(['/sign-up'])
    expect(await screen.findByRole('heading', { name: /cover art that captures the vibe/i })).toBeInTheDocument()
  })

  it('honours the guard’s destination when a signed-in user hits sign-in', async () => {
    mockFetch.mockImplementation((path: string) =>
      path.startsWith('/auth/session')
        ? Promise.resolve({ userId: 'u1', email: 'ada@example.com' })
        : Promise.resolve([]),
    )

    // The guard stashes where you were headed; already being signed in should
    // complete that trip rather than dump you on the home page.
    renderApp(['/sign-in?redirect=%2Fplaylists'])
    expect(await screen.findByRole('heading', { name: 'Your playlists' })).toBeInTheDocument()
  })

  it('still shows the sign-in form when signed out', async () => {
    mockFetch.mockResolvedValue(null)
    renderApp(['/sign-in'])

    expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument()
  })

  it('shows the not-found page for an unknown URL', async () => {
    renderApp(['/no-such-page'])

    expect(await screen.findByText('Page not found')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /back home/i })).toBeInTheDocument()
  })
})
