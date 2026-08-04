// Route guards. Kept in one place so "which routes need a session" is a single
// list rather than a decision repeated per route file, where the next route
// added is the one that quietly forgets.
import { redirect } from '@tanstack/react-router'
import type { QueryClient } from '@tanstack/react-query'

import { sessionQuery } from './api/auth'

/**
 * Redirect to sign-in unless a session exists.
 *
 * This is a UX guard, not a security boundary. Nothing here protects data: the
 * API authenticates every request on its own, and a user who edits their way
 * past this still gets 401s. Its job is to send someone to the sign-in page
 * instead of an empty screen full of failed requests.
 */
export async function requireSession(queryClient: QueryClient, href: string) {
  const session = await queryClient.ensureQueryData(sessionQuery())
  if (!session) {
    throw redirect({
      to: '/sign-in',
      // Carry the destination so sign-in can return the user where they were
      // headed rather than dumping them on the home page.
      search: { redirect: href },
    })
  }
  return session
}

/**
 * The inverse guard: keep a signed-in user off the sign-in and sign-up pages.
 *
 * Landing on a sign-in form while already signed in is a dead end. The form
 * would work, but the honest answer to "sign in" when you already are is to put
 * you where you were going, so this redirects rather than explains.
 *
 * Deliberately not applied to the routes that arrive with a token in the URL
 * (verify-email, reset-password): those are legitimate to open while signed in,
 * and redirecting would strand a working link.
 */
export async function redirectIfSignedIn(queryClient: QueryClient, href?: string) {
  const session = await queryClient.ensureQueryData(sessionQuery())
  if (!session) return

  // href is the destination the sign-in guard stashed, already validated as an
  // in-app path by the route's validateSearch. Without one, home.
  if (href) throw redirect({ href })
  throw redirect({ to: '/' })
}
