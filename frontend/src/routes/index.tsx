import { createFileRoute, Link } from '@tanstack/react-router'

import { buttonVariants } from '@/components/ui/button'

export const Route = createFileRoute('/')({
  component: Home,
})

function Home() {
  return (
    <section className="space-y-6">
      <div className="space-y-3">
        <h1 className="text-3xl font-semibold tracking-tight">
          Cover art that captures the vibe.
        </h1>
        <p className="max-w-prose text-muted-foreground">
          Connect your favorite music platform, pick a playlist, and Aurify
          analyzes every track — its audio features and lyric sentiment — to
          generate an abstract cover from a palette tuned to how the music feels.
        </p>
      </div>
      <div className="flex gap-3">
        <Link to="/playlists" className={buttonVariants()}>
          Browse playlists
        </Link>
      </div>
    </section>
  )
}
