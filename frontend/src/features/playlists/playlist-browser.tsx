import { useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardFooter,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { useConnectDsp, useGenerateCover } from '@/lib/api/commands'
import { usePlaylists } from '@/lib/api/queries'
import type { Platform } from '@/lib/api/types'

const PLATFORMS: { id: Platform; label: string }[] = [
  { id: 'spotify', label: 'Spotify' },
  { id: 'apple_music', label: 'Apple Music' },
  { id: 'youtube_music', label: 'YouTube Music' },
]

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

      {playlists.isPending && (
        <p className="text-muted-foreground">Loading playlists…</p>
      )}
      {playlists.isError && (
        <p className="text-muted-foreground">
          Couldn’t load playlists. Connect your {activeLabel} account, then try
          again.
        </p>
      )}

      <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {playlists.data?.map((pl) => (
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
    </div>
  )
}
