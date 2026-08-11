import { useEffect, useRef, useState, useSyncExternalStore, type RefObject } from 'react'
import type { QueryClient } from '@tanstack/react-query'
import { createRootRouteWithContext, Link, Outlet, useRouterState } from '@tanstack/react-router'

import { AurifyLogo } from '@/components/aurify-logo'
import { CoverGenerationWatcher } from '@/components/cover-generation-watcher'
import { Badge } from '@/components/ui/badge'
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

// The active route is marked by weight and an underline as well as color, so
// current location survives both color-blindness and a forced-colors mode.
const navLinkClass = [
  'rounded-md px-3 py-2 text-sm font-medium text-muted-foreground',
  'transition-colors hover:text-foreground',
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
  '[&.active]:font-semibold [&.active]:text-foreground [&.active]:underline [&.active]:decoration-primary [&.active]:decoration-2 [&.active]:underline-offset-8',
].join(' ')

const footerLinkClass = [
  'rounded text-sm text-muted-foreground transition-colors hover:text-foreground',
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
  '[&.active]:font-medium [&.active]:text-foreground',
].join(' ')

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
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-[var(--z-toast)] focus:rounded-md focus:bg-card focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:shadow-md focus:ring-2 focus:ring-ring"
      >
        Skip to content
      </a>

      {/* No longer sticky, and no longer blurred: it is a grid row that never
          scrolls, so there is nothing to stick to and nothing passing behind
          it. The translucency stays so the aurora still tints it. */}
      <header className="border-b border-border/60 bg-background/80">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-y-2 px-4 py-3">
          <Link
            to="/"
            className="flex items-center gap-2.5 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          >
            <AurifyLogo className="h-8 w-8" />
            <span className="font-display text-lg font-bold tracking-tight text-foreground">Aurify</span>
          </Link>
          <nav aria-label="Main" className="flex flex-wrap items-center justify-end gap-1">
            <Link to="/playlists" className={navLinkClass}>
              Playlists
            </Link>
            <CoversNavLink />
            <span className="mx-1 h-5 w-px bg-border" aria-hidden="true" />
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
        className="grid grid-rows-[auto] overflow-y-auto [&:has([data-fills-shell])]:grid-rows-[minmax(0,1fr)] focus:outline-none"
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
    <Link to="/covers" className={`${navLinkClass} inline-flex items-center gap-1.5`}>
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
  )
}

/**
 * A single status bar, because in a bounded shell the footer is on screen on
 * every route and every pixel it takes is one the content never gets back. The
 * security pitch that used to live here moved to the page it linked to, and
 * that page is now reachable straight from these links.
 */
function SiteFooter() {
  return (
    <footer className="border-t border-border/60">
      <nav
        aria-label="Footer"
        className="mx-auto flex w-full max-w-5xl flex-wrap items-center gap-x-5 gap-y-1 px-4 py-2.5"
      >
        <Link to="/about" className={footerLinkClass}>
          About
        </Link>
        <Link to="/privacy" className={footerLinkClass}>
          Privacy
        </Link>
        <Link to="/security" className={footerLinkClass}>
          Security
        </Link>
        <Link to="/contact" className={footerLinkClass}>
          Contact
        </Link>
        <span className="ml-auto text-sm text-muted-foreground">&copy; {new Date().getFullYear()} Aurify</span>
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
