import { useEffect } from 'react'
import { useRouterState } from '@tanstack/react-router'

import { useWatchCoverGenerations } from '@/lib/api/commands'
import { clearUnreadCovers } from '@/lib/cover-unread'
import { dismissCoverToasts } from '@/lib/cover-toasts'

/**
 * Invisible root-layout resident owning the plumbing behind cover
 * notifications: it holds the streams open across route changes, which the
 * playlists and covers surfaces cannot, and retires outstanding toasts and the
 * unread badge on arrival in the covers section.
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
