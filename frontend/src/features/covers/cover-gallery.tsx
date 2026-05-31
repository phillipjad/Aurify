import { Badge, type BadgeProps } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { useCovers } from '@/lib/api/queries'
import type { ColorWeight, CoverStatus } from '@/lib/api/types'

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
    return <p className="text-muted-foreground">Loading covers…</p>
  }
  if (covers.isError) {
    return <p className="text-muted-foreground">Couldn’t load covers.</p>
  }
  if (!covers.data || covers.data.length === 0) {
    return (
      <p className="text-muted-foreground">
        No covers yet. Generate one from a playlist.
      </p>
    )
  }

  return (
    <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
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
              <Badge
                variant={STATUS_VARIANT[cover.status]}
                className="capitalize"
              >
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
