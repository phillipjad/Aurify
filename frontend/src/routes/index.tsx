import { createFileRoute, Link } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { ExampleCovers } from '@/features/covers/example-covers'

export const Route = createFileRoute('/')({
  component: Home,
})

function Home() {
  return (
    <div className="space-y-14">
      <section className="max-w-2xl space-y-6">
        <h1 className="font-display text-4xl leading-display font-bold tracking-tight text-balance sm:text-5xl">
          Cover art that captures the vibe.
        </h1>

        <p className="max-w-prose text-lg leading-relaxed text-pretty text-foreground/85">
          Connect a playlist. Aurify reads the music and lyrics, then paints a cover to match the mood.
        </p>

        <div>
          <Button asChild size="lg">
            <Link to="/playlists">Browse playlists</Link>
          </Button>
        </div>
      </section>

      <section aria-labelledby="examples-heading" className="space-y-4">
        <h2 id="examples-heading" className="font-display text-xl font-bold tracking-tight text-balance">
          Every playlist gets its own palette
        </h2>
        <ExampleCovers />
      </section>
    </div>
  )
}
