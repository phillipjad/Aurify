import { createFileRoute, Link } from '@tanstack/react-router'

import { buttonVariants } from '@/components/ui/button-variants'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'

export const Route = createFileRoute('/')({
  component: Home,
})

// Brand palette dots — a small nod to the generated covers.
const SWATCHES = [
  '#FF5A36',
  '#FFB23E',
  '#FFE15D',
  '#7FB069',
  '#4F86C6',
  '#5C4D7D',
]

function Home() {
  return (
    <section className="space-y-8">
      <Card className="overflow-hidden">
        <CardContent className="space-y-6 p-8 pt-8 sm:p-10 sm:pt-10">
          <Badge variant="secondary" className="gap-2">
            <span className="size-1.5 rounded-full bg-primary" />
            Playlist cover generator
          </Badge>

          <h1 className="font-display text-4xl leading-[1.1] font-bold tracking-tight sm:text-5xl">
            Cover art that captures{' '}
            <span className="bg-gradient-to-r from-primary via-accent to-[var(--brand-cool)] bg-clip-text text-transparent">
              the vibe
            </span>
            .
          </h1>

          <p className="max-w-prose text-base leading-relaxed text-foreground/80">
            Connect your favorite music platform, pick a playlist, and Aurify
            analyzes every track — its audio features and the sentiment of its
            lyrics — to generate an abstract cover from a palette tuned to how
            the music feels.
          </p>

          <div className="flex flex-wrap items-center gap-4">
            <Link to="/playlists" className={buttonVariants({ size: 'lg' })}>
              Browse playlists
            </Link>
            <div className="flex items-center gap-1.5">
              {SWATCHES.map((hex) => (
                <span
                  key={hex}
                  className="size-4 rounded-full ring-1 ring-border"
                  style={{ backgroundColor: hex }}
                />
              ))}
            </div>
          </div>
        </CardContent>
      </Card>
    </section>
  )
}
