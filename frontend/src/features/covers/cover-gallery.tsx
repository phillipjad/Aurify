import { AlertTriangle, ImageOff, RefreshCw, Sparkles } from 'lucide-react'
import { Link } from '@tanstack/react-router'

import { EmptyState } from '@/components/empty-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { isApiError } from '@/lib/api/client'
import { useCovers } from '@/lib/api/queries'
import { cn } from '@/lib/utils'
import type { ColorWeight, Cover } from '@/lib/api/types'
import { STATUS_LABEL, STATUS_VARIANT, isInProgress } from './cover-status'

const GRID = 'grid gap-4 sm:grid-cols-2 lg:grid-cols-3'

export function CoverGallery() {
  const covers = useCovers()

  if (covers.isPending) {
    return <CoverSkeletons />
  }

  if (covers.isError) {
    return (
      <EmptyState
        icon={ImageOff}
        title="Couldn’t load your covers"
        description={errorText(covers.error, 'The request failed before it reached your library.')}
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
          <CoverTile cover={cover} />
        </li>
      ))}
    </ul>
  )
}

// A gallery tile is a pure navigation target: the whole card links to the cover
// detail, where the actions (download, regenerate, delete) live. Keeping the
// tile link-only avoids nested interactive elements and keeps the grid scannable.
function CoverTile({ cover }: { cover: Cover }) {
  const working = isInProgress(cover.status)

  return (
    <Card className="group h-full overflow-hidden">
      <Link
        to="/covers/$coverId"
        params={{ coverId: cover.id }}
        className="flex h-full flex-col rounded-xl focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      >
        <div className="relative overflow-hidden">
          {cover.imageUrl ? (
            <img
              src={cover.imageUrl}
              alt={`Cover generated for ${cover.playlistName}`}
              loading="lazy"
              className="aspect-square w-full object-cover transition-transform duration-300 ease-out group-hover:scale-[1.03]"
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
            <h3 className="min-w-0 flex-1 truncate font-display text-sm font-semibold tracking-tight group-hover:underline">
              {cover.playlistName}
            </h3>
            {/* aria-live so a status flipping under polling is announced, not silent. */}
            <Badge variant={STATUS_VARIANT[cover.status]} aria-live="polite">
              {STATUS_LABEL[cover.status]}
            </Badge>
          </div>

          {cover.status === 'failed' ? (
            <p className="line-clamp-2 text-xs text-muted-foreground">
              {cover.error || 'Generation didn’t finish. Open to try again.'}
            </p>
          ) : (
            <PalettePreview palette={cover.palette} />
          )}
        </div>
      </Link>
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
 * The derived palette as a compact swatch+label row (top 3 dimensions). The full
 * breakdown lives on the detail view; this is the at-a-glance version.
 */
function PalettePreview({ palette }: { palette?: ColorWeight[] }) {
  if (!palette || palette.length === 0) return null

  const top = [...palette].sort((a, b) => b.weight - a.weight).slice(0, 3)

  return (
    <ul className="mt-auto flex flex-wrap gap-x-3 gap-y-1">
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

function errorText(error: unknown, fallback: string): string {
  return isApiError(error) ? (error.detail ?? fallback) : fallback
}
