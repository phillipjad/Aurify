import { useWatchCoverGenerations } from '@/lib/api/commands'

/**
 * Invisible root-layout resident that keeps an SSE stream open for every
 * accepted, still-running cover generation, whatever route is on screen.
 *
 * The streams cannot belong to the playlists or covers surfaces: navigating
 * away unmounts them, and a closed stream means the rest of the generation
 * goes unobserved, which is exactly when the user has wandered off and most
 * needs it followed.
 */
export function CoverGenerationWatcher() {
  useWatchCoverGenerations()
  return null
}
