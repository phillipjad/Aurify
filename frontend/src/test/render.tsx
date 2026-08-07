import type { ReactElement } from 'react'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory, createRootRoute, createRoute, createRouter } from '@tanstack/react-router'

import { APP_SCROLL_ID } from '@/lib/app-scroll'

/**
 * Render a feature component with the providers it needs in the real app:
 * a QueryClient and a router.
 *
 * `options.path` sets the starting URL, which matters for components whose state
 * is the query string.
 *
 * Feature components render router <Link>s (empty-state CTAs, the "view it"
 * affordance after a generation starts), and those throw outside a router
 * context. Stub routes exist only so link targets resolve — the root renders
 * the component under test and never an <Outlet>, so navigation targets stay
 * inert.
 */
export function renderWithProviders(ui: ReactElement, options?: { path?: string }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  // Feature components are rendered without the root layout, so the app's scroll
  // container has to be stood up here: the virtualized list and grid measure
  // against it by id (see lib/app-scroll.ts) and render nothing without it.
  const rootRoute = createRootRoute({
    component: () => (
      <main id={APP_SCROLL_ID} data-scroll-container className="overflow-y-auto">
        {ui}
      </main>
    ),
  })
  const stubRoutes = ['/', '/playlists', '/covers', '/covers/$coverId', '/sign-in', '/sign-up'].map((path) =>
    createRoute({ getParentRoute: () => rootRoute, path, component: () => null }),
  )
  const router = createRouter({
    routeTree: rootRoute.addChildren(stubRoutes),
    history: createMemoryHistory({ initialEntries: [options?.path ?? '/'] }),
  })

  // The router comes back so a test can assert on the URL, which is where view
  // state now lives, and the client so a test can drive the cache the way the
  // SSE stream does.
  return {
    ...render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    ),
    router,
    queryClient,
  }
}
