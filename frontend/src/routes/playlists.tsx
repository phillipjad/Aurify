import { createFileRoute } from '@tanstack/react-router'

import { PlaylistBrowser } from '@/features/playlists/playlist-browser'

export const Route = createFileRoute('/playlists')({
  component: PlaylistsPage,
})

function PlaylistsPage() {
  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-semibold tracking-tight">Your playlists</h1>
      <PlaylistBrowser />
    </section>
  )
}
