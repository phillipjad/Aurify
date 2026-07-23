import { useEffect, useRef, useState, type RefObject } from 'react'
import { ArrowRight, Lock } from 'lucide-react'
import type { QueryClient } from '@tanstack/react-query'
import { createRootRouteWithContext, Link, Outlet, useRouterState } from '@tanstack/react-router'

import { AurifyLogo } from '@/components/aurify-logo'
import { ThemeToggle } from '@/components/theme-toggle'
import { UserMenu } from '@/features/auth/user-menu'

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
  const announcement = useRouteAnnouncement(mainRef)

  return (
    <div className="flex min-h-dvh flex-col">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-[var(--z-toast)] focus:rounded-md focus:bg-card focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:shadow-md focus:ring-2 focus:ring-ring"
      >
        Skip to content
      </a>

      <header className="sticky top-0 z-[var(--z-sticky)] border-b border-border/60 bg-background/80 backdrop-blur-md">
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
            <Link to="/covers" className={navLinkClass}>
              Covers
            </Link>
            <span className="mx-1 h-5 w-px bg-border" aria-hidden="true" />
            <UserMenu />
            <ThemeToggle />
          </nav>
        </div>
      </header>
      <main
        id="main"
        ref={mainRef}
        tabIndex={-1}
        className="mx-auto w-full max-w-5xl flex-1 px-4 py-10 focus:outline-none"
      >
        <Outlet />
      </main>

      <SiteFooter />

      {/* On navigation the SPA swaps content silently; announce the new page to
          assistive tech (focus moves to <main> in the hook above). */}
      <div role="status" aria-live="polite" className="sr-only">
        {announcement}
      </div>
    </div>
  )
}

function SiteFooter() {
  const onSecurityPage = useRouterState({ select: (state) => state.location.pathname === '/security' })

  return (
    <footer className="border-t border-border/60">
      <div className="mx-auto w-full max-w-5xl px-4 py-10">
        {/* Redundant on the security page itself, so collapse it there. Kept
            mounted (not conditionally rendered) so it can animate out as well as
            in — the grid 0fr↔1fr trick animates height:auto, and `inert` keeps
            the collapsed link out of tab order and the a11y tree. */}
        <div
          className="grid transition-[grid-template-rows,opacity] duration-300 ease-out motion-reduce:transition-none"
          style={{ gridTemplateRows: onSecurityPage ? '0fr' : '1fr', opacity: onSecurityPage ? 0 : 1 }}
          inert={onSecurityPage || undefined}
        >
          <div className="overflow-hidden">
            <div className="mb-6 max-w-2xl space-y-2 border-b border-border/60 pb-6">
              <h2 className="text-lg">
                <Link
                  to="/security"
                  className="group inline-flex items-center gap-2 rounded font-display font-semibold tracking-tight text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
                >
                  <Lock aria-hidden="true" className="size-4 text-primary" />
                  <span className="underline decoration-transparent decoration-2 underline-offset-4 transition-colors group-hover:decoration-primary">
                    Security is a focus here, not a vibe
                  </span>
                  <ArrowRight
                    aria-hidden="true"
                    className="size-4 text-primary transition-transform group-hover:translate-x-0.5"
                  />
                </Link>
              </h2>
              <p className="text-pretty text-sm text-muted-foreground">
                Spotify, Apple Music, and YouTube Music sign-ins happen through their own platforms. Aurify receives a
                safe, ephemeral, minimally scoped access token, never your password.
              </p>
            </div>
          </div>
        </div>

        <nav aria-label="Footer" className="flex flex-wrap items-center gap-x-6 gap-y-2">
          <Link to="/about" className={footerLinkClass}>
            About
          </Link>
          <Link to="/privacy" className={footerLinkClass}>
            Privacy Policy
          </Link>
          <Link to="/contact" className={footerLinkClass}>
            Contact
          </Link>
          <span className="ml-auto text-sm text-muted-foreground">© {new Date().getFullYear()} Aurify</span>
        </nav>
      </div>
    </footer>
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
