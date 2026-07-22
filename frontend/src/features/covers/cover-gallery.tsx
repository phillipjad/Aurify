import { AlertTriangle, ImageOff, RefreshCw, Sparkles } from 'lucide-react'
import { Link } from '@tanstack/react-router'

import { EmptyState } from '@/components/empty-state'
import { Badge, type BadgeProps } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { isApiError } from '@/lib/api/client'
import { useGenerateCover } from '@/lib/api/commands'
import { useCovers } from '@/lib/api/queries'
import { cn } from '@/lib/utils'
import type { ColorWeight, Cover, CoverStatus } from '@/lib/api/types'

const GRID = 'grid gap-4 sm:grid-cols-2 lg:grid-cols-3'

// Map a cover's lifecycle status to a Badge variant.
const STATUS_VARIANT: Record<CoverStatus, BadgeProps['variant']> = {
  pending: 'secondary',
  analyzing: 'secondary',
  generating: 'default',
  ready: 'success',
  failed: 'destructive',
}

// Plain-language status copy. "analyzing"/"generating" are internal pipeline
// stages; users care what is happening to *their* playlist.
const STATUS_LABEL: Record<CoverStatus, string> = {
  pending: 'Queued',
  analyzing: 'Listening',
  generating: 'Painting',
  ready: 'Ready',
  failed: 'Failed',
}

const IN_PROGRESS: ReadonlySet<CoverStatus> = new Set<CoverStatus>(['pending', 'analyzing', 'generating'])

export function CoverGallery() {
  const covers = useCovers()
  const regenerate = useGenerateCover()

  if (covers.isPending) {
    return <CoverSkeletons />
  }

  if (covers.isError) {
    return (
      <EmptyState
        icon={ImageOff}
        title="Couldn’t load your covers"
        description={errorMessage(covers.error, 'The request failed before it reached your library.')}
        action={
          <Button variant="outline" size="sm" loading={covers.isFetching} onClick={() => void covers.refetch()}>
            <RefreshCw aria-hidden="true" className="size-4" />
            Try again
          </Button>
        }
      />
    )
  }

  if (!covers.data || covers.data.length === 0) {
    return (
      <EmptyState
        icon={Sparkles}
        title="No covers yet"
        description="Pick a playlist and Aurify will read its audio and lyrics, then paint a cover from the result."
        action={
          <Link to="/playlists" className={buttonVariants({ size: 'sm' })}>
            Browse playlists
          </Link>
        }
      />
    )
  }

  return (
    <ul className={GRID}>
      {covers.data.map((cover) => (
        <li key={cover.id}>
          <CoverTile
            cover={cover}
            regenerating={regenerate.isPending && regenerate.variables?.playlistId === cover.playlistId}
            onRegenerate={() => regenerate.mutate({ platform: cover.platform, playlistId: cover.playlistId })}
          />
        </li>
      ))}
    </ul>
  )
}

interface CoverTileProps {
  cover: Cover
  regenerating: boolean
  onRegenerate: () => void
}

function CoverTile({ cover, regenerating, onRegenerate }: CoverTileProps) {
  const working = IN_PROGRESS.has(cover.status)

  return (
    // h-full + column flex so every tile in a row shares the tallest one's
    // height; without it the grid item stretches but the card doesn't fill it,
    // leaving ragged bottoms whenever one cover has more metadata than another.
    <Card className="flex h-full flex-col overflow-hidden">
      <div className="relative">
        {cover.imageUrl ? (
          <img
            src={cover.imageUrl}
            alt={`Cover generated for ${cover.playlistName}`}
            loading="lazy"
            className="aspect-square w-full object-cover"
          />
        ) : (
          <div aria-hidden="true" className="flex aspect-square w-full items-center justify-center bg-muted">
            {cover.status === 'failed' ? (
              <AlertTriangle className="size-8 text-muted-foreground" />
            ) : (
              <Sparkles className={cn('size-8 text-muted-foreground', working && 'animate-pulse')} />
            )}
          </div>
        )}
      </div>

      <div className="flex flex-1 flex-col gap-2 p-3">
        <div className="flex items-start justify-between gap-2">
          <h3 className="min-w-0 flex-1 truncate font-display text-sm font-semibold tracking-tight">
            {cover.playlistName}
          </h3>
          {/* aria-live so a status flipping under polling is announced, not silent. */}
          <Badge variant={STATUS_VARIANT[cover.status]} aria-live="polite">
            {STATUS_LABEL[cover.status]}
          </Badge>
        </div>

        {cover.status === 'failed' ? (
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">
              {cover.error || 'Generation didn’t finish. Nothing was saved.'}
            </p>
            <Button variant="outline" size="sm" loading={regenerating} onClick={onRegenerate}>
              <RefreshCw aria-hidden="true" className="size-4" />
              Try again
            </Button>
          </div>
        ) : (
          <PalettePreview palette={cover.palette} />
        )}
      </div>
    </Card>
  )
}

function CoverSkeletons() {
  return (
    <ul className={GRID}>
      {Array.from({ length: 6 }, (_, i) => (
        <li key={i}>
          <Card className="overflow-hidden">
            <Skeleton className="aspect-square w-full rounded-none" />
            <div className="space-y-2 p-3">
              <div className="flex items-center justify-between gap-2">
                <Skeleton className="h-4 w-1/2" />
                <Skeleton className="h-5 w-16 rounded-full" />
              </div>
              <Skeleton className="h-4 w-24" />
            </div>
          </Card>
        </li>
      ))}
    </ul>
  )
}

/**
 * Read a user-facing message off an unknown error, falling back to plain copy.
 *
 * Only `detail` is used: RFC 7807 defines it as the explanation specific to this
 * occurrence, while `title` summarizes the problem *type* and in practice is the
 * bare HTTP status phrase ("Not Found"). Our own sentence beats that every time.
 */
function errorMessage(error: unknown, fallback: string): string {
  return isApiError(error) ? (error.detail ?? fallback) : fallback
}

/**
 * The derived palette, with the dimension each color came from.
 *
 * The meaning used to live in a `title` attribute, which is invisible on touch
 * and unreliable in screen readers — so the one output that explains *why* a
 * cover looks the way it does was effectively hidden. It is text now.
 */
function PalettePreview({ palette }: { palette?: ColorWeight[] }) {
  if (!palette || palette.length === 0) return null

  const top = [...palette].sort((a, b) => b.weight - a.weight).slice(0, 3)

  return (
    <ul className="flex flex-wrap gap-x-3 gap-y-1">
      {top.map((color) => (
        <li key={color.dimension} className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span
            aria-hidden="true"
            className="size-3 shrink-0 rounded-full ring-1 ring-border"
            style={{ backgroundColor: color.hexColor }}
          />
          <span className="capitalize">{color.dimension}</span>
          <span className="tabular-nums">{Math.round(color.weight * 100)}%</span>
        </li>
      ))}
    </ul>
  )
}
