import { useCovers } from '@/lib/api/queries'
import type { ColorWeight } from '@/lib/api/types'

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
        <li
          key={cover.id}
          className="overflow-hidden rounded-lg border border-border"
        >
          {cover.imageUrl ? (
            <img
              src={cover.imageUrl}
              alt={`Generated cover for playlist ${cover.playlistId}`}
              className="aspect-square w-full object-cover"
            />
          ) : (
            <div className="aspect-square w-full bg-muted" />
          )}
          <div className="flex items-center justify-between p-3">
            <span className="text-sm capitalize text-muted-foreground">
              {cover.status}
            </span>
            <PalettePreview palette={cover.palette} />
          </div>
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
          className="h-4 w-4 rounded-full"
          style={{ backgroundColor: c.hexColor }}
        />
      ))}
    </div>
  )
}
