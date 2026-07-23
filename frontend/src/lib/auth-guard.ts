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
