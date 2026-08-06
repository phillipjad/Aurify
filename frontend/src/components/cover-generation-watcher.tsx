import { useEffect } from 'react'
import { useRouterState } from '@tanstack/react-router'

import { useWatchCoverGenerations } from '@/lib/api/commands'
import { clearUnreadCovers } from '@/lib/cover-unread'
import { dismissCoverToasts } from '@/lib/cover-toasts'

/**
 * Invisible root-layout resident that owns the plumbing behind cover
 * notifications.
 *
 * It keeps an SSE stream open for every accepted, still-running generation,
 * whatever route is on screen. The streams cannot belong to the playlists or
 * covers surfaces: navigating away unmounts them, and a closed stream means
 * the ready event, and the toast it fires, never arrives, precisely when the
 * user is elsewhere and the toast is the only signal.
 *
 * It also retires notifications on arrival in the covers section: outstanding
 * toasts, because a generation can finish while the user is on their way there
 * and a toast pointing at a cover already on screen is noise, and the unread
 * badge, because they are now looking at the thing it was counting.
 */
export function CoverGenerationWatcher() {
  useWatchCoverGenerations()

  const pathname = useRouterState({ select: (state) => state.location.pathname })
  useEffect(() => {
    if (!pathname.startsWith('/covers')) return
    dismissCoverToasts()
    clearUnreadCovers()
  }, [pathname])

  return null
}
