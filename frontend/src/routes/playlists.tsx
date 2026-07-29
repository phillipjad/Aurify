import { createFileRoute } from '@tanstack/react-router'

import { PageHeader } from '@/components/page-header'
import { PlaylistBrowser } from '@/features/playlists/playlist-browser'
import { toPlatform } from '@/features/playlists/platforms'
import type { Platform } from '@/lib/api/types'
import { requireSession } from '@/lib/auth-guard'

/**
 * What the DSP OAuth callback redirects back with: `connected` on success, or
 * `connect_error` plus the `platform` it belongs to. Every field is optional —
 * the page is reachable as a plain link — which is also what keeps existing
 * `<Link to="/playlists">` call sites from having to pass a search object.
 */
interface PlaylistsSearch {
  connected?: Platform
  connectError?: string
  platform?: Platform
}

export const Route = createFileRoute('/playlists')({
  beforeLoad: ({ context, location }) => requireSession(context.queryClient, location.href),
  // Narrowed rather than trusted: these arrive from a redirect, not from
  // anything the app controls.
  validateSearch: (search: Record<string, unknown>): PlaylistsSearch => ({
    connected: toPlatform(search.connected),
    connectError: typeof search.connect_error === 'string' ? search.connect_error : undefined,
    platform: toPlatform(search.platform),
  }),
  component: PlaylistsPage,
})

function PlaylistsPage() {
  const { connected, connectError, platform } = Route.useSearch()

  return (
    <section className="space-y-8">
      <PageHeader
        title="Your playlists"
        description="Pick a playlist and Aurify turns its sound and lyrics into a cover."
      />
      <PlaylistBrowser connected={connected} connectError={connectError} connectErrorPlatform={platform} />
    </section>
  )
}
