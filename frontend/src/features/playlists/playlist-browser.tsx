import { useState } from 'react'
import { AlertTriangle, ListMusic, RefreshCw, SearchX, Unplug } from 'lucide-react'
import { Link } from '@tanstack/react-router'

import { EmptyState } from '@/components/empty-state'
import { ImageWithFallback } from '@/components/image-with-fallback'
import { LoadMore } from '@/components/load-more'
import { SearchInput } from '@/components/search-input'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { isApiError } from '@/lib/api/client'
import { useConnectDsp, useGenerateCover } from '@/lib/api/commands'
import { usePlaylists, type PlaylistSort } from '@/lib/api/queries'
import { useDebouncedValue } from '@/lib/use-debounced-value'
import type { Platform, Playlist } from '@/lib/api/types'
import { PlatformPicker } from './platform-picker'
import { platformLabel } from './platforms'

export function PlaylistBrowser() {
  const [platform, setPlatform] = useState<Platform>('spotify')
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<PlaylistSort>('name')
  // Debounce so typing doesn't fire a request per keystroke; the search runs on
  // the server (see the /playlists q param) so it covers every page, not just
  // the ones already loaded.
  const search = useDebouncedValue(query.trim())

  const playlists = usePlaylists({ platform, search, sort })
  const connect = useConnectDsp()
  const generate = useGenerateCover()

  const activeLabel = platformLabel(platform)
  const items = playlists.data?.pages.flat() ?? []
  const hasQuery = search.length > 0

  // The mutation is shared across the list, so derive per-row state from the
  // variables it was last called with. Without this, one in-flight generation
  // would disable every row's button with no sign of which one is running.
  const inFlightId = generate.isPending ? generate.variables?.playlistId : undefined

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <PlatformPicker value={platform} onChange={setPlatform} />
        <div className="flex flex-col items-end gap-1">
          <Button variant="outline" size="sm" loading={connect.isPending} onClick={() => connect.mutate(platform)}>
            Connect {activeLabel}
          </Button>
          {connect.isError && (
            <p role="alert" className="text-xs text-destructive">
              {errorMessage(connect.error, `Couldn’t reach ${activeLabel}.`)}
            </p>
          )}
        </div>
      </div>

      {playlists.isError ? (
        isApiError(playlists.error) && playlists.error.isUnauthorized ? (
          <EmptyState
            icon={Unplug}
            title={`Connect your ${activeLabel} account`}
            description={`Aurify needs access to your ${activeLabel} library before it can list your playlists.`}
            action={
              <Button size="sm" loading={connect.isPending} onClick={() => connect.mutate(platform)}>
                Connect {activeLabel}
              </Button>
            }
          />
        ) : (
          <EmptyState
            icon={AlertTriangle}
            title="Couldn’t load your playlists"
            description={errorMessage(playlists.error, 'The request failed before it reached your library.')}
            action={
              <Button
                variant="outline"
                size="sm"
                loading={playlists.isFetching}
                onClick={() => void playlists.refetch()}
              >
                <RefreshCw aria-hidden="true" className="size-4" />
                Try again
              </Button>
            }
          />
        )
      ) : playlists.isPending ? (
        <PlaylistSkeletons />
      ) : !hasQuery && items.length === 0 ? (
        <EmptyState
          icon={ListMusic}
          title="No playlists found"
          description={`We didn’t find any playlists on ${activeLabel}. Create one there and it’ll show up here.`}
        />
      ) : (
        <div className="space-y-4" aria-busy={playlists.isFetching || undefined}>
          <div className="flex flex-wrap items-center gap-3">
            <SearchInput className="min-w-56 flex-1" value={query} onChange={setQuery} label="Search playlists" />
            <label className="flex items-center gap-2 text-sm text-muted-foreground">
              Sort
              <select
                value={sort}
                onChange={(event) => setSort(event.target.value as PlaylistSort)}
                className="h-9 rounded-lg border border-border bg-card px-2 text-sm text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
              >
                <option value="name">Name</option>
                <option value="tracks">Tracks</option>
              </select>
            </label>
          </div>

          {items.length > 0 ? (
            <ul className="space-y-2">
              {items.map((playlist) => (
                <PlaylistRow
                  key={playlist.id}
                  playlist={playlist}
                  generating={inFlightId === playlist.id}
                  // Only the row that was acted on reports the outcome.
                  error={
                    generate.isError && generate.variables?.playlistId === playlist.id ? generate.error : undefined
                  }
                  succeeded={generate.isSuccess && generate.variables?.playlistId === playlist.id}
                  onGenerate={() => generate.mutate({ platform, playlistId: playlist.id })}
                />
              ))}
            </ul>
          ) : (
            <EmptyState icon={SearchX} title="No matches" description={`No playlists match “${query.trim()}”.`} />
          )}

          <LoadMore
            label="Load more playlists"
            hasNextPage={playlists.hasNextPage}
            isFetchingNextPage={playlists.isFetchingNextPage}
            fetchNextPage={() => void playlists.fetchNextPage()}
          />
        </div>
      )}
    </div>
  )
}

interface PlaylistRowProps {
  playlist: Playlist
  generating: boolean
  error?: unknown
  succeeded: boolean
  onGenerate: () => void
}

function PlaylistRow({ playlist, generating, error, succeeded, onGenerate }: PlaylistRowProps) {
  return (
    <li>
      <Card className="flex flex-wrap items-center gap-4 p-3 sm:flex-nowrap">
        <PlaylistArtwork playlist={playlist} />

        <div className="min-w-0 flex-1">
          {/* line-clamp rather than truncate: at 375px a single-line clamp cut
              names to ~18 characters, hiding the one piece of information the
              choice actually depends on. Short names still render on one line. */}
          <h3 className="line-clamp-2 font-display font-semibold tracking-tight">{playlist.name}</h3>
          <p className="text-sm text-muted-foreground">
            {playlist.trackCount} {playlist.trackCount === 1 ? 'track' : 'tracks'}
          </p>
          {playlist.description && (
            <p className="mt-0.5 line-clamp-1 text-sm text-muted-foreground">{playlist.description}</p>
          )}
        </div>

        <div className="flex shrink-0 flex-col items-end gap-1">
          <Button size="sm" loading={generating} onClick={onGenerate}>
            {generating ? 'Aurifying…' : 'Aurify it'}
          </Button>
          {succeeded && (
            <Link to="/covers" className="text-xs font-medium text-primary underline underline-offset-2">
              Cover started — view it
            </Link>
          )}
        </div>
      </Card>

      {error != null && (
        <p role="alert" className="px-3 pt-1.5 text-xs text-destructive">
          {errorMessage(error, 'Generation failed. Try again in a moment.')}
        </p>
      )}
    </li>
  )
}

function PlaylistArtwork({ playlist }: { playlist: Playlist }) {
  return (
    <ImageWithFallback
      src={playlist.imageUrl}
      alt=""
      loading="lazy"
      className="size-14 shrink-0 rounded-md object-cover"
      fallback={
        <span
          aria-hidden="true"
          className="flex size-14 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground"
        >
          <ListMusic className="size-5" />
        </span>
      }
    />
  )
}

function PlaylistSkeletons() {
  return (
    <ul className="space-y-2">
      {Array.from({ length: 6 }, (_, i) => (
        <li key={i}>
          <Card className="flex items-center gap-4 p-3">
            <Skeleton className="size-14 shrink-0 rounded-md" />
            <div className="flex-1 space-y-2">
              <Skeleton className="h-4 w-2/5" />
              <Skeleton className="h-3.5 w-24" />
            </div>
            <Skeleton className="h-9 w-24 shrink-0 rounded-lg" />
          </Card>
        </li>
      ))}
    </ul>
  )
}

/** Read a user-facing message off an unknown error, falling back to plain copy. */
function errorMessage(error: unknown, fallback: string): string {
  return isApiError(error) ? (error.detail ?? fallback) : fallback
}
