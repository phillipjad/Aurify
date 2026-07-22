import { useState } from 'react'
import { AlertTriangle, ImageOff, RefreshCw, Sparkles } from 'lucide-react'
import { Link } from '@tanstack/react-router'

import { EmptyState } from '@/components/empty-state'
import { LoadMore } from '@/components/load-more'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Card } from '@/components/ui/card'
import { ImageWithFallback } from '@/components/image-with-fallback'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { isApiError } from '@/lib/api/client'
import { useCovers, type CoverFilter } from '@/lib/api/queries'
import type { ColorWeight, Cover } from '@/lib/api/types'
import { STATUS_LABEL, STATUS_VARIANT, isInProgress } from './cover-status'

const GRID = 'grid gap-4 sm:grid-cols-2 lg:grid-cols-3'

// Filter options map 1:1 to a single backend status (or "all"), so the server
// does the filtering across every page, not just the ones already loaded.
const FILTERS: { id: CoverFilter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'ready', label: 'Ready' },
  { id: 'failed', label: 'Failed' },
]

export function CoverGallery() {
  const [filter, setFilter] = useState<CoverFilter>('all')
  const covers = useCovers(filter)

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

  // First load for this filter (keepPreviousData means switching filters won't
  // land here — the previous grid stays up while the new one loads).
  if (covers.isPending) {
    return (
      <div className="space-y-4">
        {filter !== 'all' && <FilterBar value={filter} onChange={setFilter} />}
        <CoverSkeletons />
      </div>
    )
  }

  // Pages partition by offset, but dedupe by id anyway so a cover created or
  // deleted between page refetches can never surface a duplicate React key.
  const items = dedupeById(covers.data?.pages.flat() ?? [])

  // Genuinely empty (unfiltered) → the onboarding empty state.
  if (filter === 'all' && items.length === 0) {
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
    <div className="space-y-4" aria-busy={covers.isFetching || undefined}>
      <FilterBar value={filter} onChange={setFilter} />

      {items.length > 0 ? (
        <ul className={GRID}>
          {items.map((cover) => (
            <li key={cover.id}>
              <CoverTile cover={cover} />
            </li>
          ))}
        </ul>
      ) : (
        <p className="py-6 text-center text-sm text-muted-foreground">
          No {FILTERS.find((f) => f.id === filter)?.label.toLowerCase()} covers.
        </p>
      )}

      <LoadMore
        label="Load more covers"
        hasNextPage={covers.hasNextPage}
        isFetchingNextPage={covers.isFetchingNextPage}
        fetchNextPage={() => void covers.fetchNextPage()}
      />
    </div>
  )
}

/** Mutually-exclusive, server-driven status filter. */
function FilterBar({ value, onChange }: { value: CoverFilter; onChange: (filter: CoverFilter) => void }) {
  return (
    <div role="group" aria-label="Filter covers by status" className="flex flex-wrap gap-2">
      {FILTERS.map((option) => {
        const active = option.id === value
        return (
          <button
            key={option.id}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(option.id)}
            className={cn(
              'rounded-full border px-3 py-1 text-sm transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none',
              active
                ? 'border-transparent bg-primary text-primary-foreground'
                : 'border-border text-muted-foreground hover:text-foreground',
            )}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}

function dedupeById(covers: Cover[]): Cover[] {
  const seen = new Set<string>()
  const out: Cover[] = []
  for (const cover of covers) {
    if (!seen.has(cover.id)) {
      seen.add(cover.id)
      out.push(cover)
    }
  }
  return out
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
          <ImageWithFallback
            src={cover.imageUrl}
            alt={`Cover generated for ${cover.playlistName}`}
            loading="lazy"
            className="aspect-square w-full object-cover transition-transform duration-300 ease-out group-hover:scale-[1.03]"
            fallback={
              <div aria-hidden="true" className="flex aspect-square w-full items-center justify-center bg-muted">
                {cover.status === 'failed' ? (
                  <AlertTriangle className="size-8 text-muted-foreground" />
                ) : (
                  <Sparkles className={cn('size-8 text-muted-foreground', working && 'animate-pulse')} />
                )}
              </div>
            }
          />
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
