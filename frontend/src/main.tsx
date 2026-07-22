import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createRouter } from '@tanstack/react-router'

import { ThemeProvider } from './components/theme-provider'
import { NotFound, RouteError } from './components/route-fallbacks'
import { routeTree } from './routeTree.gen'
import { queryClient } from './lib/query-client'
import './styles.css'

// The router carries the QueryClient in context so route loaders can prefetch.
const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: 'intent',
  // Branded fallbacks for unknown URLs and render-time errors, rendered inside
  // the root layout so the header and navigation stay put.
  defaultNotFoundComponent: NotFound,
  defaultErrorComponent: RouteError,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

const rootEl = document.getElementById('root')
if (!rootEl) throw new Error('root element not found')

createRoot(rootEl).render(
  <StrictMode>
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
)
