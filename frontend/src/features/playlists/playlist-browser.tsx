import { useEffect, useRef, useState } from 'react'
import { AlertTriangle, Check, ListMusic, RefreshCw, SearchX, Unplug } from 'lucide-react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'

import { getAppScrollElement, useScrollMargin } from '@/lib/app-scroll'

import { EmptyState } from '@/components/empty-state'
import { ImageWithFallback } from '@/components/image-with-fallback'
import { LoadMore } from '@/components/load-more'
import { SearchInput } from '@/components/search-input'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useSession } from '@/lib/api/auth'
import { isApiError } from '@/lib/api/client'
import { connectDsp, useCoverGenerations, useGenerateCover, type CoverGeneration } from '@/lib/api/commands'
import { usePlaylists, type PlaylistSort } from '@/lib/api/queries'
import { useStageLabel } from '@/features/covers/cover-status'
import { useDebouncedValue } from '@/lib/use-debounced-value'
import type { Platform, Playlist } from '@/lib/api/types'
import { PlatformPicker } from './platform-picker'
import {
  connectErrorMessage,
  DEFAULT_PLATFORM,
  PLATFORMS,
  platformLabel,
  readLastPlatform,
  rememberLastPlatform,
  toPlatform,
  toPlaylistSort,
} from './platforms'

/**
 * Starting height for an unmeasured row: a 56px thumbnail inside 12px padding
 * plus the 8px gap. Real heights replace it as rows are measured, so this only
 * has to be close enough to keep the scrollbar honest before that happens.
 */
const ROW_ESTIMATE = 88

export function PlaylistBrowser() {
  const navigate = useNavigate()
  // strict: false so this reads whatever route it is mounted under, which is what
  // keeps it renderable in isolation. The values are narrowed again here rather
  // than trusted: at this looseness their shape genuinely is not known, and the
  // route's own validateSearch only covers navigation into the route.
  const params = useSearch({ strict: false }) as Record<string, unknown>

  const connected = toPlatform(params.connected)
  const connectError = typeof params.connect_error === 'string' ? params.connect_error : undefined
  const urlSearch = typeof params.q === 'string' ? params.q : ''
  const sort = toPlaylistSort(params.sort) ?? 'name'

  const session = useSession()

  // A platform the user has actually linked, preferred over the hardcoded
  // default. Without this someone who has connected only YouTube Music opens the
  // page on Spotify and is told to "Connect Spotify" — which reads as the
  // connection having failed, when the session knew about it the whole time. It
  // bites on any device with no remembered preference: a new browser, cleared
  // storage, or a private window.
  const firstConnected = PLATFORMS.find((p) => session.data?.connections?.includes(p))

  // Resolution order matters. An explicit platform in the URL wins, so a shared
  // link shows what it says; then the platform a callback just connected; then
  // whatever was used last, which is what makes the header's Playlists link
  // return somewhere useful; then anything connected; then the default.
  const platform = toPlatform(params.platform) ?? connected ?? readLastPlatform() ?? firstConnected ?? DEFAULT_PLATFORM

  // The search box keeps its own copy so typing stays instant. The URL gets the
  // settled term, since navigating on every keystroke would put a history entry
  // behind each letter.
  const [input, setInput] = useState(urlSearch)
  const settledInput = useDebouncedValue(input.trim())

  useEffect(() => {
    if (settledInput === urlSearch) return
    // replace, not push: a search term is a refinement, and pushing one entry per
    // pause would make leaving the page a matter of pressing back repeatedly.
    void navigate({
      to: '/playlists',
      search: (prev) => ({ ...prev, q: settledInput || undefined }),
      replace: true,
    })
  }, [settledInput, urlSearch, navigate])

  // Remembered on every resolved value, not just on a click, so a deep link or a
  // returning OAuth callback also becomes the thing to come back to.
  //
  // Not while the session is still loading, though: the resolution above falls
  // back to Spotify until the connection list arrives, and writing that would
  // make the remembered value beat the connected one on the very next read,
  // permanently pinning the user to a platform they never chose.
  useEffect(() => {
    if (session.isPending) return
    rememberLastPlatform(platform)
  }, [platform, session.isPending])

  const playlists = usePlaylists({ platform, search: urlSearch, sort })
  const generate = useGenerateCover()
  const generationFor = useCoverGenerations()

  const activeLabel = platformLabel(platform)
  // The session reports which platforms are linked, so the button can say so
  // outright instead of leaving the user to infer it from whether a list
  // appeared. An empty list is not the same answer as "not connected".
  const isConnected = session.data?.connections?.includes(platform) ?? false
  const items = playlists.data?.pages.flat() ?? []
  const hasQuery = urlSearch.length > 0

  // Whether what is on screen corresponds to what is in the box. Between a
  // keystroke and the debounce landing it does not, and the list still holds the
  // previous term's results — which is how "No playlists match <new term>" used
  // to appear over the old term's empty result set.
  const resultsMatchInput = input.trim() === urlSearch && !playlists.isFetching

  // --- windowing ---
  //
  // Rows are rendered only around the viewport, so the DOM stays a fixed size no
  // matter how many pages have been loaded. The scroll container is the app
  // shell's <main>, not the window: the shell is bounded to the viewport so the
  // document itself never scrolls (see lib/app-scroll.ts).
  const listRef = useRef<HTMLDivElement>(null)
  const scrollMargin = useScrollMargin(listRef)

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: getAppScrollElement,
    estimateSize: () => ROW_ESTIMATE,
    // A few rows of slack above and below, so scrolling reveals rendered rows
    // rather than blank space that fills in a frame later.
    overscan: 6,
    scrollMargin,
    // Rows have variable height (a name can wrap to two lines, a description is
    // optional), so real measurements replace the estimate. Falling back to the
    // estimate when layout reports zero keeps this working where there is no
    // layout at all, which is every test environment.
    measureElement: (el) => el.getBoundingClientRect().height || ROW_ESTIMATE,
  })

  function selectPlatform(next: Platform) {
    // A push, unlike search and sort: switching source is a change of view, and
    // back should undo it.
    void navigate({
      to: '/playlists',
      search: (prev) => ({
        ...prev,
        platform: next,
        // A deliberate switch retires the last connect outcome; leaving it would
        // report a stale result against a platform the user moved on from.
        connected: undefined,
        connect_error: undefined,
      }),
    })
  }

  function selectSort(next: PlaylistSort) {
    void navigate({ to: '/playlists', search: (prev) => ({ ...prev, sort: next }), replace: true })
  }

  // No platform comparison needed any more. The callback redirects with the
  // platform it was for, that value is the selection, and switching away strips
  // connect_error from the URL — so a failed YouTube Music attempt cannot follow a
  // switch to Spotify by construction rather than by a check that could be
  // forgotten.
  const failedConnect = connectError ? connectErrorMessage(connectError, activeLabel) : undefined

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <PlatformPicker value={platform} onChange={selectPlatform} />
        <div className="flex flex-col items-end gap-1">
          {isConnected ? (
            <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <Check aria-hidden="true" className="size-4 text-primary" />
              {activeLabel} connected
              {/* Reconnecting is how a user re-grants a revoked or expired
                  authorization, so it stays reachable, just demoted. */}
              <button
                type="button"
                onClick={() => void connectDsp(platform)}
                className="rounded-sm underline underline-offset-2 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
              >
                Reconnect
              </button>
            </p>
          ) : (
            <Button variant="outline" size="sm" onClick={() => void connectDsp(platform)}>
              Connect {activeLabel}
            </Button>
          )}
          {failedConnect && (
            <p role="alert" className="max-w-xs text-right text-xs text-destructive-text">
              {failedConnect}
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
              <Button size="sm" onClick={() => void connectDsp(platform)}>
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
            <SearchInput className="min-w-56 flex-1" value={input} onChange={setInput} label="Search playlists" />
            <label className="flex items-center gap-2 text-sm text-muted-foreground">
              Sort
              <select
                value={sort}
                onChange={(event) => selectSort(event.target.value as PlaylistSort)}
                className="h-9 rounded-lg border border-border bg-card px-2 text-sm text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
              >
                <option value="name">Name</option>
                <option value="tracks">Tracks</option>
              </select>
            </label>
          </div>

          {items.length > 0 ? (
            <div ref={listRef}>
              {/* The list keeps its full height so the scrollbar reflects every
                  loaded playlist, while only the visible slice exists in the DOM.
                  aria-setsize and aria-posinset carry the real position, which a
                  partial list cannot convey on its own. */}
              <ul aria-label="Playlists" className="relative" style={{ height: virtualizer.getTotalSize() }}>
                {virtualizer.getVirtualItems().map((row) => {
                  const playlist = items[row.index]
                  if (!playlist) return null
                  return (
                    <li
                      key={playlist.id}
                      data-index={row.index}
                      ref={virtualizer.measureElement}
                      aria-setsize={items.length}
                      aria-posinset={row.index + 1}
                      className="absolute inset-x-0 top-0 pb-2"
                      style={{ transform: `translateY(${row.start - scrollMargin}px)` }}
                    >
                      <PlaylistRow
                        playlist={playlist}
                        // Each row reads its own generation, so several can run at
                        // once and report separately. Derived from the mutation
                        // cache rather than one shared hook result, which only ever
                        // described the most recent click.
                        generation={generationFor(playlist.id)}
                        onGenerate={() => generate.mutate({ platform, playlistId: playlist.id })}
                      />
                    </li>
                  )
                })}
              </ul>
            </div>
          ) : resultsMatchInput ? (
            // Only a settled list may return a verdict, and it names the term the
            // results are actually for.
            <EmptyState icon={SearchX} title="No matches" description={`No playlists match “${urlSearch}”.`} />
          ) : (
            // Mid-debounce the list still holds the previous term's results, so
            // there is nothing truthful to say about them yet.
            <PlaylistSkeletons />
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
  generation: CoverGeneration
  onGenerate: () => void
}

/**
 * One row's content. The surrounding <li> belongs to the windowed list, which has
 * to position it absolutely, so this renders the card and nothing structural.
 */
function PlaylistRow({ playlist, generation, onGenerate }: PlaylistRowProps) {
  const generating = generation.status === 'pending'
  const error = generation.status === 'error' ? generation.error : undefined
  const succeeded = generation.status === 'success'
  // Called unconditionally, as hooks must be. Before the stream has named a
  // stage there is nothing to cycle, and 'pending' is the label that says so.
  // The button is not a live region, so only the visible half is used here.
  const { visible: stage } = useStageLabel(generation.stage ?? 'pending')

  return (
    <>
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
          {/* Width reserved for the longest label, so the row does not jerk.
              The label now cycles every four seconds, so this has to clear the
              widest verb plus the spinner: "Composing…" measures 131px. */}
          <Button size="sm" className="min-w-[8.5rem]" loading={generating} onClick={onGenerate}>
            {generating ? `${generation.stage ? stage : 'Aurifying'}…` : 'Aurify it'}
          </Button>
          {succeeded && (
            <Link to="/covers" className="text-xs font-medium text-primary underline underline-offset-2">
              Cover ready, view it
            </Link>
          )}
        </div>
      </Card>

      {error != null && (
        <p role="alert" className="px-3 pt-1.5 text-xs text-destructive-text">
          {errorMessage(error, 'Generation failed. Try again in a moment.')}
        </p>
      )}
    </>
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
  // A failed generation reports the cover row's error, a bare string.
  if (typeof error === 'string' && error) return error
  return isApiError(error) ? (error.detail ?? fallback) : fallback
}
