import { useEffect, useRef, useState, type RefObject } from 'react'
import type { QueryClient } from '@tanstack/react-query'
import { createRootRouteWithContext, Link, Outlet, useRouterState } from '@tanstack/react-router'

import { AurifyLogo } from '@/components/aurify-logo'
import { ThemeToggle } from '@/components/theme-toggle'

// Context made available to every route (loaders, components).
export interface RouterContext {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
})

// The active route is marked by weight and an underline as well as color, so
// current location survives both color-blindness and a forced-colors mode.
const navLinkClass = [
  'rounded-md px-3 py-2 text-sm font-medium text-muted-foreground',
  'transition-colors hover:text-foreground',
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
  '[&.active]:font-semibold [&.active]:text-foreground [&.active]:underline [&.active]:decoration-primary [&.active]:decoration-2 [&.active]:underline-offset-8',
].join(' ')

function RootLayout() {
  const mainRef = useRef<HTMLElement>(null)
  const announcement = useRouteAnnouncement(mainRef)

  return (
    <div className="min-h-dvh">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-[var(--z-toast)] focus:rounded-md focus:bg-card focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:shadow-md focus:ring-2 focus:ring-ring"
      >
        Skip to content
      </a>

      <header className="sticky top-0 z-[var(--z-sticky)] border-b border-border/60 bg-background/80 backdrop-blur-md">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-4 py-3">
          <Link
            to="/"
            className="flex items-center gap-2.5 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          >
            <AurifyLogo className="h-8 w-8" />
            <span className="font-display text-lg font-bold tracking-tight text-foreground">Aurify</span>
          </Link>
          <nav aria-label="Main" className="flex items-center gap-1">
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
      <main id="main" ref={mainRef} tabIndex={-1} className="mx-auto max-w-5xl px-4 py-10 focus:outline-none">
        <Outlet />
      </main>

      {/* On navigation the SPA swaps content silently; announce the new page to
          assistive tech (focus moves to <main> in the hook above). */}
      <div role="status" aria-live="polite" className="sr-only">
        {announcement}
      </div>
    </div>
  )
}

/**
 * After a client-side navigation, move focus to the main region and announce the
 * new page. Skipped on first paint so we neither steal focus on load nor
 * announce a page the user just opened directly.
 */
function useRouteAnnouncement(mainRef: RefObject<HTMLElement | null>): string {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const [message, setMessage] = useState('')
  const firstRender = useRef(true)

  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    mainRef.current?.focus()
    setMessage(`${pageName(pathname)}, Aurify`)
  }, [pathname, mainRef])

  return message
}

function pageName(pathname: string): string {
  if (pathname === '/') return 'Home'
  if (pathname.startsWith('/playlists')) return 'Playlists'
  if (/^\/covers\/.+/.test(pathname)) return 'Cover details'
  if (pathname.startsWith('/covers')) return 'Your covers'
  return 'Page'
}
