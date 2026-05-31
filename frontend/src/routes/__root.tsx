import type { QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  Link,
  Outlet,
} from '@tanstack/react-router'

import { AurifyLogo } from '@/components/aurify-logo'
import { ThemeToggle } from '@/components/theme-toggle'

// Context made available to every route (loaders, components).
export interface RouterContext {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
})

const navLinkClass =
  'rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground [&.active]:text-foreground'

function RootLayout() {
  return (
    <div className="min-h-dvh">
      <header className="sticky top-0 z-20 border-b border-border/60 bg-background/70 backdrop-blur-md">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-4 py-3">
          <Link to="/" className="flex items-center gap-2.5">
            <AurifyLogo className="h-8 w-8" />
            <span className="bg-gradient-to-r from-primary to-accent bg-clip-text font-display text-lg font-bold tracking-tight text-transparent">
              Aurify
            </span>
          </Link>
          <nav className="flex items-center gap-1">
            <Link to="/playlists" className={navLinkClass}>
              Playlists
            </Link>
            <Link to="/covers" className={navLinkClass}>
              Covers
            </Link>
            <ThemeToggle />
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-10">
        <Outlet />
      </main>
    </div>
  )
}
