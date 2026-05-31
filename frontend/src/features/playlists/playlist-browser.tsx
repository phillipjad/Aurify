import { useState } from 'react'
import { ListMusic, Unplug } from 'lucide-react'

import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useConnectDsp, useGenerateCover } from '@/lib/api/commands'
import { usePlaylists } from '@/lib/api/queries'
import type { Platform } from '@/lib/api/types'

const PLATFORMS: { id: Platform; label: string }[] = [
  { id: 'spotify', label: 'Spotify' },
  { id: 'apple_music', label: 'Apple Music' },
  { id: 'youtube_music', label: 'YouTube Music' },
]

const GRID = 'grid gap-4 sm:grid-cols-2 lg:grid-cols-3'

export function PlaylistBrowser() {
  const [platform, setPlatform] = useState<Platform>('spotify')
  const playlists = usePlaylists(platform)
  const connect = useConnectDsp()
  const generate = useGenerateCover()

  const activeLabel = PLATFORMS.find((p) => p.id === platform)?.label

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-2">
        {PLATFORMS.map((p) => (
          <Button
            key={p.id}
            variant={p.id === platform ? 'default' : 'outline'}
            size="sm"
            onClick={() => setPlatform(p.id)}
          >
            {p.label}
          </Button>
        ))}
        <Button
          variant="ghost"
          size="sm"
          disabled={connect.isPending}
          onClick={() => connect.mutate(platform)}
        >
          Connect {activeLabel}
        </Button>
      </div>

      {playlists.isPending && <PlaylistSkeletons />}

      {playlists.isError && (
        <EmptyState
          icon={Unplug}
          title={`Connect your ${activeLabel} account`}
          description="We couldn’t load your playlists. Link the account above, then try again."
        />
      )}

      {playlists.data && playlists.data.length === 0 && (
        <EmptyState
          icon={ListMusic}
          title="No playlists found"
          description={`We didn’t find any playlists on ${activeLabel}.`}
        />
      )}

      {playlists.data && playlists.data.length > 0 && (
        <ul className={GRID}>
          {playlists.data.map((pl) => (
            <li key={pl.id}>
              <Card className="h-full">
                <CardHeader>
                  <CardTitle className="text-base">{pl.name}</CardTitle>
                  <CardDescription>{pl.trackCount} tracks</CardDescription>
                </CardHeader>
                <CardFooter>
                  <Button
                    size="sm"
                    disabled={generate.isPending}
                    onClick={() =>
                      generate.mutate({ platform, playlistId: pl.id })
                    }
                  >
                    Aurify it
                  </Button>
                </CardFooter>
              </Card>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function PlaylistSkeletons() {
  return (
    <ul className={GRID}>
      {Array.from({ length: 6 }).map((_, i) => (
        <li key={i}>
          <Card className="h-full">
            <CardHeader className="gap-2">
              <Skeleton className="h-5 w-2/3" />
              <Skeleton className="h-4 w-1/3" />
            </CardHeader>
            <CardFooter>
              <Skeleton className="h-9 w-24 rounded-lg" />
            </CardFooter>
          </Card>
        </li>
      ))}
    </ul>
  )
}
