import type { QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  Link,
  Outlet,
} from '@tanstack/react-router'

// Context made available to every route (loaders, components).
export interface RouterContext {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
})

function RootLayout() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b border-border">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-4 py-3">
          <Link to="/" className="text-lg font-semibold tracking-tight">
            Aurify
          </Link>
          <nav className="flex gap-4 text-sm">
            <Link
              to="/playlists"
              className="text-muted-foreground hover:text-foreground [&.active]:text-foreground"
            >
              Playlists
            </Link>
            <Link
              to="/covers"
              className="text-muted-foreground hover:text-foreground [&.active]:text-foreground"
            >
              Covers
            </Link>
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-8">
        <Outlet />
      </main>
    </div>
  )
}
