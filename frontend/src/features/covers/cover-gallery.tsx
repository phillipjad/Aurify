import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { AlertTriangle, ImageOff, RefreshCw, Sparkles } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { useWindowVirtualizer } from '@tanstack/react-virtual'

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

/**
 * Starting height for an unmeasured grid row: a square thumbnail at a third of
 * the container, plus the title, status and palette beneath it. Real heights
 * replace it once rows are measured.
 */
const ROW_ESTIMATE = 360

/**
 * The breakpoints GRID switches on, read through matchMedia so the column count
 * comes from the same numbers the CSS uses rather than from measuring a width and
 * hoping the two agree.
 */
const COLUMN_QUERIES: readonly [string, number][] = [
  ['(min-width: 1024px)', 3],
  ['(min-width: 640px)', 2],
]

function currentColumns(): number {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return 1
  for (const [query, columns] of COLUMN_QUERIES) {
    if (window.matchMedia(query).matches) return columns
  }
  return 1
}

/**
 * How many tiles sit on a row right now.
 *
 * A windowed grid has to know this: it virtualizes rows, not tiles, so the row
 * count and each row's contents depend on the breakpoint.
 */
function useGridColumns(): number {
  const [columns, setColumns] = useState(currentColumns)

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
    const lists = COLUMN_QUERIES.map(([query]) => window.matchMedia(query))
    const update = () => setColumns(currentColumns())
    for (const list of lists) list.addEventListener('change', update)
    update()
    return () => {
      for (const list of lists) list.removeEventListener('change', update)
    }
  }, [])

  return columns
}

// Filter options map 1:1 to a single backend status (or "all"), so the server
// does the filtering across every page, not just the ones already loaded.
const FILTERS: { id: CoverFilter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'ready', label: 'Ready' },
  { id: 'failed', label: 'Failed' },
]

export function CoverGallery() {
  const [filter, setFilter] = useState<CoverFilter>('all')
  const columns = useGridColumns()
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
        <WindowedCoverGrid items={items} columns={columns} />
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
/**
 * The covers grid, rendering only the rows near the viewport.
 *
 * Rows are virtualized rather than tiles, because the layout is a grid: how many
 * covers share a row is a function of the breakpoint, so the unit that can be
 * positioned is the row.
 *
 * Semantics come from roles rather than <ul>/<li>. A wrapper element per row is
 * unavoidable — it is what gets positioned — and one is not allowed between a
 * list and its items, so role="list" and role="listitem" carry the meaning
 * instead. aria-setsize and aria-posinset supply the position that a partially
 * rendered list cannot.
 */
function WindowedCoverGrid({ items, columns }: { items: Cover[]; columns: number }) {
  const gridRef = useRef<HTMLDivElement>(null)
  const [gridTop, setGridTop] = useState(0)
  const rowCount = Math.ceil(items.length / columns)

  const virtualizer = useWindowVirtualizer({
    count: rowCount,
    estimateSize: () => ROW_ESTIMATE,
    // Rows are tall, so a couple either side is plenty of slack.
    overscan: 2,
    scrollMargin: gridTop,
    // Falling back to the estimate keeps this sane where there is no layout to
    // measure, which is every test environment.
    measureElement: (el) => el.getBoundingClientRect().height || ROW_ESTIMATE,
  })

  // The grid sits below a filter bar whose height changes, so its offset is
  // re-read after each render and written only when it actually moved.
  useLayoutEffect(() => {
    const next = gridRef.current?.offsetTop ?? 0
    if (next !== gridTop) setGridTop(next)
  })

  // A breakpoint change repacks every row, so previous measurements describe a
  // layout that no longer exists.
  useEffect(() => {
    virtualizer.measure()
  }, [columns, virtualizer])

  return (
    <div ref={gridRef}>
      {/* Named, because a tile contains its own palette list and an unnamed one
          is indistinguishable from it to a screen reader. */}
      <div role="list" aria-label="Covers" className="relative" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((row) => {
          const first = row.index * columns
          return (
            <div
              key={row.index}
              data-index={row.index}
              ref={virtualizer.measureElement}
              className={cn(GRID, 'absolute inset-x-0 top-0 pb-4')}
              style={{ transform: `translateY(${row.start - gridTop}px)` }}
            >
              {items.slice(first, first + columns).map((cover, offset) => (
                <div key={cover.id} role="listitem" aria-setsize={items.length} aria-posinset={first + offset + 1}>
                  <CoverTile cover={cover} />
                </div>
              ))}
            </div>
          )
        })}
      </div>
    </div>
  )
}

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
