import { createFileRoute } from '@tanstack/react-router'

import { PageHeader } from '@/components/page-header'
import { PlaylistBrowser } from '@/features/playlists/playlist-browser'
import { toPlatform, toPlaylistSort } from '@/features/playlists/platforms'
import type { PlaylistSort } from '@/lib/api/queries'
import type { Platform } from '@/lib/api/types'
import { requireSession } from '@/lib/auth-guard'

/**
 * The browser's view state, held in the URL rather than in the component.
 *
 * Which platform, search term and ordering a user is looking at is exactly the
 * state they expect to survive a reload, a back button and a pasted link. Held in
 * useState it died on unmount, so returning from another page dropped them back
 * on Spotify with an empty search box.
 *
 * `connected` and `connectError` are different in kind: transient outcomes the
 * OAuth callback redirects back with, read once and not navigated to again.
 *
 * Every field is optional, which is what lets a plain `<Link to="/playlists">`
 * stay a plain link.
 */
export interface PlaylistsSearch {
  platform?: Platform
  q?: string
  sort?: PlaylistSort
  connected?: Platform
  // Named for the URL key rather than camelCased, so the route and the component
  // reading it loosely agree on one spelling.
  connect_error?: string
}

export const Route = createFileRoute('/playlists')({
  beforeLoad: ({ context, location }) => requireSession(context.queryClient, location.href),
  // Narrowed rather than trusted: a URL is typed by anyone, and the callback
  // parameters arrive from a redirect rather than from anything the app controls.
  validateSearch: (search: Record<string, unknown>): PlaylistsSearch => ({
    platform: toPlatform(search.platform),
    q: typeof search.q === 'string' && search.q !== '' ? search.q : undefined,
    sort: toPlaylistSort(search.sort),
    connected: toPlatform(search.connected),
    connect_error: typeof search.connect_error === 'string' ? search.connect_error : undefined,
  }),
  component: PlaylistsPage,
})

function PlaylistsPage() {
  return (
    <section className="space-y-8">
      <PageHeader
        title="Your playlists"
        description="Pick a playlist and Aurify turns its sound and lyrics into a cover."
      />
      <PlaylistBrowser />
    </section>
  )
}
