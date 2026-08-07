import { useState } from 'react'
import { AlertTriangle, ArrowLeft, Download, ImageOff, RefreshCw, Sparkles, Trash2 } from 'lucide-react'
import { Link, useNavigate } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Card } from '@/components/ui/card'
import { EmptyState } from '@/components/empty-state'
import { ImageWithFallback } from '@/components/image-with-fallback'
import { Skeleton } from '@/components/ui/skeleton'
import { isApiError } from '@/lib/api/client'
import { useDeleteCover, useGenerateCover } from '@/lib/api/commands'
import { useCover } from '@/lib/api/queries'
import { cn } from '@/lib/utils'
import type { ColorWeight, Cover } from '@/lib/api/types'
import { platformLabel } from '@/features/playlists/platforms'
import { StatusBadge, isInProgress } from './cover-status'

export function CoverDetail({ coverId }: { coverId: string }) {
  const cover = useCover(coverId)

  if (cover.isPending) return <CoverDetailSkeleton />

  if (cover.isError) {
    const notFound = isApiError(cover.error) && cover.error.status === 404
    return (
      <EmptyState
        icon={ImageOff}
        title={notFound ? 'This cover doesn’t exist' : 'Couldn’t load this cover'}
        description={
          notFound
            ? 'It may have been deleted. It won’t appear in your gallery anymore.'
            : 'The request failed before it reached your library.'
        }
        action={
          <Link to="/covers" className={buttonVariants({ size: 'sm' })}>
            <ArrowLeft aria-hidden="true" className="size-4" />
            All covers
          </Link>
        }
      />
    )
  }

  return <CoverDetailView cover={cover.data} />
}

function CoverDetailView({ cover }: { cover: Cover }) {
  const navigate = useNavigate()
  const regenerate = useGenerateCover()
  const del = useDeleteCover()
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  const working = isInProgress(cover.status)

  return (
    <div className="space-y-6">
      <Link
        to="/covers"
        className="inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
      >
        <ArrowLeft aria-hidden="true" className="size-4" />
        All covers
      </Link>

      <div className="grid gap-6 md:grid-cols-[minmax(0,22rem)_1fr] md:items-start">
        <CoverImage cover={cover} working={working} />

        <div className="space-y-6">
          <header className="space-y-2">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
              <span>{platformLabel(cover.platform)}</span>
              <span aria-hidden="true">·</span>
              <time dateTime={cover.createdAt}>{formatDate(cover.createdAt)}</time>
              <StatusBadge status={cover.status} />
            </div>
            <h1 className="font-display text-2xl font-bold tracking-tight text-balance sm:text-3xl">
              {cover.playlistName}
            </h1>
          </header>

          {cover.status === 'failed' && (
            <p role="alert" className="text-sm text-destructive-text">
              {cover.error || 'Generation didn’t finish. Nothing was saved to the cover.'}
            </p>
          )}

          <PaletteBreakdown palette={cover.palette} />

          {cover.prompt && (
            <section className="space-y-1.5">
              <h2 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">Prompt</h2>
              <p className="text-pretty text-sm text-foreground/80">{cover.prompt}</p>
            </section>
          )}

          <div className="flex flex-wrap items-center gap-2 border-t border-border pt-5">
            {cover.imageUrl && (
              <a
                href={cover.imageUrl}
                download={`aurify-${cover.playlistName}.png`}
                target="_blank"
                rel="noreferrer"
                className={buttonVariants({ variant: 'outline', size: 'sm' })}
              >
                <Download aria-hidden="true" className="size-4" />
                Download
              </a>
            )}

            <Button
              variant="outline"
              size="sm"
              loading={regenerate.isPending}
              onClick={() =>
                regenerate.mutate(
                  { platform: cover.platform, playlistId: cover.playlistId },
                  { onSuccess: () => void navigate({ to: '/covers' }) },
                )
              }
            >
              <RefreshCw aria-hidden="true" className="size-4" />
              Regenerate
            </Button>

            {confirmingDelete ? (
              <span className="inline-flex items-center gap-2 text-sm">
                <span className="text-muted-foreground">Delete this cover?</span>
                <Button
                  variant="outline"
                  size="sm"
                  className="border-destructive/40 text-destructive hover:bg-destructive/10"
                  loading={del.isPending}
                  onClick={() => del.mutate(cover.id, { onSuccess: () => void navigate({ to: '/covers' }) })}
                >
                  Delete
                </Button>
                <Button variant="ghost" size="sm" disabled={del.isPending} onClick={() => setConfirmingDelete(false)}>
                  Cancel
                </Button>
              </span>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                className="text-muted-foreground hover:text-destructive"
                onClick={() => setConfirmingDelete(true)}
              >
                <Trash2 aria-hidden="true" className="size-4" />
                Delete
              </Button>
            )}
          </div>

          {(regenerate.isError || del.isError) && (
            <p role="alert" className="text-sm text-destructive-text">
              {errorText(regenerate.error ?? del.error, 'That didn’t work. Try again in a moment.')}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

function CoverImage({ cover, working }: { cover: Cover; working: boolean }) {
  return (
    <Card className="overflow-hidden">
      <ImageWithFallback
        src={cover.imageUrl}
        alt={`Cover generated for ${cover.playlistName}`}
        className="aspect-square w-full object-cover"
        fallback={
          <div aria-hidden="true" className="flex aspect-square w-full items-center justify-center bg-muted">
            {cover.status === 'failed' ? (
              <AlertTriangle className="size-10 text-muted-foreground" />
            ) : (
              <Sparkles
                data-motion="breathe"
                className={cn('size-10 text-muted-foreground', working && 'animate-breathe')}
              />
            )}
          </div>
        }
      />
    </Card>
  )
}

/** Every dimension in the derived palette, as a labeled weight bar. */
function PaletteBreakdown({ palette }: { palette?: ColorWeight[] }) {
  if (!palette || palette.length === 0) return null
  const sorted = [...palette].sort((a, b) => b.weight - a.weight)

  return (
    <section className="space-y-2">
      <h2 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">Palette</h2>
      <ul className="space-y-2">
        {sorted.map((color) => (
          <li key={color.dimension} className="flex items-center gap-3">
            <span
              aria-hidden="true"
              className="size-4 shrink-0 rounded-full ring-1 ring-border"
              style={{ backgroundColor: color.hexColor }}
            />
            <span className="w-28 shrink-0 text-sm font-medium capitalize">{color.dimension}</span>
            <span className="h-2 flex-1 overflow-hidden rounded-full bg-muted">
              <span
                className="block h-full rounded-full"
                style={{ width: `${Math.round(color.weight * 100)}%`, backgroundColor: color.hexColor }}
              />
            </span>
            <span className="w-10 shrink-0 text-right text-sm tabular-nums text-muted-foreground">
              {Math.round(color.weight * 100)}%
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}

function CoverDetailSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-5 w-24" />
      <div className="grid gap-6 md:grid-cols-[minmax(0,22rem)_1fr]">
        <Skeleton className="aspect-square w-full rounded-xl" />
        <div className="space-y-4">
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-8 w-3/4" />
          <div className="space-y-2 pt-4">
            {Array.from({ length: 4 }, (_, i) => (
              <Skeleton key={i} className="h-4 w-full" />
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

function errorText(error: unknown, fallback: string): string {
  return isApiError(error) ? (error.detail ?? fallback) : fallback
}
