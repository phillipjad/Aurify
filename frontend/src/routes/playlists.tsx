import { createFileRoute } from '@tanstack/react-router'

import { PageHeader } from '@/components/page-header'
import { PlaylistBrowser } from '@/features/playlists/playlist-browser'
import { requireSession } from '@/lib/auth-guard'

export const Route = createFileRoute('/playlists')({
  beforeLoad: ({ context, location }) => requireSession(context.queryClient, location.href),
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
