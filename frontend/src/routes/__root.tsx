import { useEffect, useRef, useState, useSyncExternalStore, type RefObject } from 'react'
import type { QueryClient } from '@tanstack/react-query'
import { createRootRouteWithContext, Link, Outlet, useRouterState } from '@tanstack/react-router'

import { AurifyLogo } from '@/components/aurify-logo'
import { CoverGenerationWatcher } from '@/components/cover-generation-watcher'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { SkipLink } from '@/components/ui/skip-link'
import { Toaster } from '@/components/ui/sonner'
import { UserMenu } from '@/features/auth/user-menu'
import { getUnreadCoverCount, subscribeUnreadCovers } from '@/lib/cover-unread'

// Context made available to every route (loaders, components).
export interface RouterContext {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
})

function RootLayout() {
  const mainRef = useRef<HTMLElement>(null)
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const announcement = useRouteAnnouncement(pathname, mainRef)

  return (
    // The shell is bounded to the viewport, not merely at least as tall as it:
    // grid rows of auto / minmax(0, 1fr) / auto, where the 0 minimum is what
    // lets the middle row shrink and scroll instead of pushing the footer down
    // the document. See docs/adr/0017-bounded-app-shell.md.
    <div className="grid h-dvh grid-rows-[auto_minmax(0,1fr)_auto]">
      <SkipLink href="#main">Skip to content</SkipLink>

      {/* No longer sticky, and no longer blurred: it is a grid row that never
          scrolls, so there is nothing to stick to and nothing passing behind
          it. The translucency stays so the aurora still tints it. */}
      <header className="border-b border-border/60 bg-background/80">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-y-2 px-4 py-3">
          <Link to="/" className="flex h-control-sm items-center gap-2.5 rounded-md">
            <AurifyLogo className="h-8 w-8" />
            <span className="font-display text-lg font-bold tracking-tight text-foreground">Aurify</span>
          </Link>
          <nav aria-label="Main" className="flex flex-wrap items-center justify-end gap-1">
            <Button asChild variant="nav" size="sm">
              <Link to="/playlists">Playlists</Link>
            </Button>
            <CoversNavLink />
            <Separator orientation="vertical" className="mx-1 h-5" />
            <UserMenu />
          </nav>
        </div>
      </header>
      {/* The shell's default scroll container (see lib/app-scroll.ts). The width
          cap moves inside it so the scrollbar rides the viewport edge rather
          than the content column. data-scroll-container carries the shared
          scrollbar styling, including the reserved gutter (see styles.css). */}
      <main
        id="main"
        ref={mainRef}
        tabIndex={-1}
        data-scroll-container
        className="grid grid-rows-[auto] overflow-y-auto [&:has([data-fills-shell])]:grid-rows-[minmax(0,1fr)]"
      >
        {/* Keyed on the path so every navigation replays the entrance. Search
            params are deliberately excluded: the playlists view keeps its state
            in the query string, and filtering should not flash the whole page.

            The row is content-sized by default, so an ordinary page is as tall
            as it needs and main scrolls. A route that scrolls a region of
            itself marks its root with data-fills-shell, and the :has() above
            switches the row to exactly one screen: that definite height is what
            gives the route's flex-1 something real to divide. Declared by the
            route rather than listed here, so this file needs no route table. */}
        <div key={pathname} className="route-enter mx-auto flex w-full max-w-5xl flex-col px-4 py-2">
          <Outlet />
        </div>
      </main>

      <SiteFooter />

      {/* Watched from the root so the streams survive route changes; finished
          covers announce themselves here (see lib/cover-toasts.tsx). */}
      <CoverGenerationWatcher />
      <Toaster />

      {/* On navigation the SPA swaps content silently; announce the new page to
          assistive tech (focus moves to <main> in the hook above). */}
      <div role="status" aria-live="polite" className="sr-only">
        {announcement}
      </div>
    </div>
  )
}

/**
 * The Covers link, counting generations that finished while the user was
 * elsewhere. This is what lets the toast be brief: it expires, the badge does
 * not, until they reach the gallery.
 */
function CoversNavLink() {
  const unread = useSyncExternalStore(subscribeUnreadCovers, getUnreadCoverCount)

  return (
    <Button asChild variant="nav" size="sm" className="gap-1.5">
      <Link to="/covers">
        Covers
        {unread > 0 && (
          <>
            {/* Decoration: the sentence below is what gets read out. */}
            <Badge aria-hidden="true" className="px-1.5 py-0 tabular-nums">
              {unread}
            </Badge>
            <span className="sr-only">
              ({unread} new {unread === 1 ? 'cover' : 'covers'})
            </span>
          </>
        )}
      </Link>
    </Button>
  )
}

/**
 * A single status bar, because in a bounded shell the footer is on screen on
 * every route and every pixel it takes is one the content never gets back. The
 * security pitch that used to live here moved to the page it linked to, and
 * that page is now reachable straight from these links.
 */
const FOOTER_LINKS = [
  { to: '/about', label: 'About' },
  { to: '/privacy', label: 'Privacy' },
  { to: '/security', label: 'Security' },
  { to: '/contact', label: 'Contact' },
] as const

function SiteFooter() {
  return (
    <footer className="border-t border-border/60">
      <nav
        aria-label="Footer"
        // px-2 rather than px-4: each link carries the other half of the
        // padding, so the labels still line up with the content column.
        className="mx-auto flex w-full max-w-5xl flex-wrap items-center px-2 py-1"
      >
        {FOOTER_LINKS.map(({ to, label }) => (
          <Button key={to} asChild variant="nav" size="sm" className="px-2 font-normal">
            <Link to={to}>{label}</Link>
          </Button>
        ))}
        <span className="ml-auto px-2 text-sm text-muted-foreground">&copy; {new Date().getFullYear()} Aurify</span>
      </nav>
    </footer>
  )
}

/**
 * After a client-side navigation, move focus to the main region and announce the
 * new page. Skipped on first paint so we neither steal focus on load nor
 * announce a page the user just opened directly.
 */
function useRouteAnnouncement(pathname: string, mainRef: RefObject<HTMLElement | null>): string {
  const [message, setMessage] = useState('')
  const firstRender = useRef(true)

  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    // The scroll position belongs to <main> now, and an element keeps its
    // scrollTop across a route change. Without this, leaving a scrolled list
    // drops you into the middle of the next page.
    mainRef.current?.scrollTo({ top: 0 })
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
  if (pathname === '/sign-in') return 'Sign in'
  if (pathname === '/sign-up') return 'Sign up'
  if (pathname === '/verify-email') return 'Verify your email'
  if (pathname === '/forgot-password') return 'Reset your password'
  if (pathname === '/reset-password') return 'Choose a new password'
  if (pathname === '/about') return 'About'
  if (pathname === '/privacy') return 'Privacy Policy'
  if (pathname === '/contact') return 'Contact'
  if (pathname === '/security') return 'Security'
  return 'Page'
}
