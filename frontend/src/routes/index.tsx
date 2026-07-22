import { createFileRoute, Link } from '@tanstack/react-router'

import { buttonVariants } from '@/components/ui/button-variants'

export const Route = createFileRoute('/')({
  component: Home,
})

// The dimensions Aurify weighs, with the base color each contributes to a
// cover's palette. Mirrors `palette` in backend/internal/analysis/weights.go —
// keep the names and hexes in sync when dimensions are added or retuned.
const DIMENSIONS: { name: string; hex: string; signal: string }[] = [
  { name: 'energetic', hex: '#FF5A36', signal: 'energy + liveness' },
  { name: 'danceable', hex: '#FFB23E', signal: 'danceability' },
  { name: 'euphoric', hex: '#FFE15D', signal: 'valence + lyric polarity' },
  { name: 'organic', hex: '#7FB069', signal: 'acousticness' },
  { name: 'introspective', hex: '#4F86C6', signal: 'instrumentalness' },
  { name: 'melancholic', hex: '#5C4D7D', signal: 'low valence + negative lyrics' },
  { name: 'intimate', hex: '#C46BAE', signal: 'speechiness' },
]

function Home() {
  return (
    <div className="space-y-16">
      <section className="max-w-2xl space-y-6">
        <h1 className="font-display text-4xl leading-[1.1] font-bold tracking-tight text-balance sm:text-5xl">
          Cover art that captures the vibe.
        </h1>

        <p className="max-w-prose text-lg leading-relaxed text-pretty text-foreground/85">
          Connect your music platform and pick a playlist. Aurify reads every track — its audio features and the
          sentiment of its lyrics — then paints a cover from a palette weighted to how the music actually feels.
        </p>

        <div>
          <Link to="/playlists" className={buttonVariants({ size: 'lg' })}>
            Browse playlists
          </Link>
        </div>
      </section>

      <section aria-labelledby="palette-heading" className="space-y-5">
        <div className="max-w-prose space-y-2">
          <h2 id="palette-heading" className="font-display text-2xl font-bold tracking-tight text-balance">
            Seven dimensions, weighted per playlist
          </h2>
          <p className="text-pretty text-muted-foreground">
            Each one contributes a color in proportion to how strongly your tracks express it. A live, high-energy
            playlist leans red; an instrumental one drifts blue.
          </p>
        </div>

        <ul className="flex flex-wrap gap-x-6 gap-y-3">
          {DIMENSIONS.map((dimension) => (
            <li key={dimension.name} className="flex items-baseline gap-2">
              <span
                aria-hidden="true"
                className="size-3 shrink-0 translate-y-0.5 rounded-full ring-1 ring-border"
                style={{ backgroundColor: dimension.hex }}
              />
              <span>
                <span className="font-medium capitalize">{dimension.name}</span>{' '}
                <span className="text-sm text-muted-foreground">{dimension.signal}</span>
              </span>
            </li>
          ))}
        </ul>
      </section>
    </div>
  )
}
