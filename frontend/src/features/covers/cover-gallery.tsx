import { ImageOff, Sparkles } from 'lucide-react'
import { Link } from '@tanstack/react-router'

import { EmptyState } from '@/components/empty-state'
import { Badge, type BadgeProps } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button-variants'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useCovers } from '@/lib/api/queries'
import type { ColorWeight, CoverStatus } from '@/lib/api/types'

const GRID = 'grid gap-4 sm:grid-cols-2 lg:grid-cols-3'

// Map a cover's lifecycle status to a Badge variant.
const STATUS_VARIANT: Record<CoverStatus, BadgeProps['variant']> = {
  pending: 'secondary',
  analyzing: 'secondary',
  generating: 'default',
  ready: 'success',
  failed: 'destructive',
}

export function CoverGallery() {
  const covers = useCovers()

  if (covers.isPending) {
    return <CoverSkeletons />
  }
  if (covers.isError) {
    return (
      <EmptyState
        icon={ImageOff}
        title="Couldn’t load covers"
        description="Something went wrong fetching your covers. Try again in a moment."
      />
    )
  }
  if (!covers.data || covers.data.length === 0) {
    return (
      <EmptyState
        icon={Sparkles}
        title="No covers yet"
        description="Generate one from a playlist and it’ll show up here."
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
          <Card className="overflow-hidden">
            {cover.imageUrl ? (
              <img
                src={cover.imageUrl}
                alt={`Generated cover for playlist ${cover.playlistId}`}
                className="aspect-square w-full object-cover"
              />
            ) : (
              <div className="aspect-square w-full bg-gradient-to-br from-muted to-secondary" />
            )}
            <div className="flex items-center justify-between gap-2 p-3">
              <Badge variant={STATUS_VARIANT[cover.status]} className="capitalize">
                {cover.status}
              </Badge>
              <PalettePreview palette={cover.palette} />
            </div>
          </Card>
        </li>
      ))}
    </ul>
  )
}

function CoverSkeletons() {
  return (
    <ul className={GRID}>
      {Array.from({ length: 6 }).map((_, i) => (
        <li key={i}>
          <Card className="overflow-hidden">
            <Skeleton className="aspect-square w-full rounded-none" />
            <div className="flex items-center justify-between gap-2 p-3">
              <Skeleton className="h-5 w-16 rounded-full" />
              <Skeleton className="h-4 w-20" />
            </div>
          </Card>
        </li>
      ))}
    </ul>
  )
}

function PalettePreview({ palette }: { palette?: ColorWeight[] }) {
  if (!palette || palette.length === 0) return null
  return (
    <div className="flex gap-1">
      {palette.slice(0, 5).map((c) => (
        <span
          key={c.dimension}
          title={`${c.dimension} ${(c.weight * 100).toFixed(0)}%`}
          className="size-4 rounded-full ring-1 ring-border"
          style={{ backgroundColor: c.hexColor }}
        />
      ))}
    </div>
  )
}
