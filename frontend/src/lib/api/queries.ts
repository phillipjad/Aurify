// READ side. TanStack Query definitions for the API's query endpoints.
import { useEffect } from 'react'
import {
  infiniteQueryOptions,
  keepPreviousData,
  queryOptions,
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'

import { apiFetch } from './client'
import { watchCover } from './cover-events'
import type { Cover, CoverStatus, Platform, Playlist } from './types'

/** Page size for the paginated list endpoints (matches the API's default). */
export const PAGE_SIZE = 20

/** Ordering accepted by the playlists endpoint. */
export type PlaylistSort = 'name' | 'tracks'

/** Covers list filter: a single lifecycle status, or all of them. */
export type CoverFilter = CoverStatus | 'all'

export const queryKeys = {
  playlists: (platform: Platform, search: string, sort: PlaylistSort) => ['playlists', platform, search, sort] as const,
  // Broad prefix used to invalidate every covers query (list + detail).
  covers: () => ['covers'] as const,
  coversList: (filter: CoverFilter) => ['covers', 'list', filter] as const,
  // Keyed on the failed-runs toggle as well, so flipping it refetches rather
  // than reading a cached response that answers the other question.
  cover: (id: string, showFailedRuns = false) => ['covers', 'detail', id, showFailedRuns] as const,
}

interface PlaylistsArgs {
  platform: Platform
  search: string
  sort: PlaylistSort
}

export const playlistsInfiniteQuery = ({ platform, search, sort }: PlaylistsArgs) =>
  infiniteQueryOptions({
    queryKey: queryKeys.playlists(platform, search, sort),
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({
        platform,
        sort,
        limit: String(PAGE_SIZE),
        offset: String(pageParam),
      })
      if (search) params.set('q', search)
      return apiFetch<Playlist[]>(`/playlists?${params.toString()}`)
    },
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => (lastPage.length < PAGE_SIZE ? undefined : allPages.length * PAGE_SIZE),
    // Keep the current results on screen while a new search or sort loads, rather
    // than flashing skeletons on every keystroke — but only within one platform.
    // Held across a platform change, the previous service's playlists stay
    // visible under the new tab, labelled as something they are not.
    placeholderData: (previous, previousQuery) => (previousQuery?.queryKey[1] === platform ? previous : undefined),
  })

/** Cover statuses that will never change again without a new user action. */
export const TERMINAL_STATUSES: ReadonlySet<CoverStatus> = new Set<CoverStatus>(['ready', 'failed'])

/**
 * The keyset position of the next page: the last cover of the one before it.
 * Null is the first page.
 *
 * Offset paging cannot survive this list. A regeneration moves its playlist to
 * the front, so by the time page two is asked for, the row that was at index 20
 * has shifted and offset paging either repeats a tile or skips one. Naming the
 * exact row to continue after has no such window. The cursor is read off the
 * response rather than handed back in an envelope, since every cover already
 * carries both halves of it.
 */
type CoverCursor = { before: string; beforeId: string } | null

const nextCoverCursor = (lastPage: Cover[]): CoverCursor => {
  if (lastPage.length < PAGE_SIZE) return null
  const last = lastPage[lastPage.length - 1]
  return last ? { before: last.updatedAt, beforeId: last.id } : null
}

export const coversInfiniteQuery = (filter: CoverFilter = 'all') =>
  infiniteQueryOptions({
    queryKey: queryKeys.coversList(filter),
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ limit: String(PAGE_SIZE) })
      if (filter !== 'all') params.set('status', filter)
      if (pageParam) {
        params.set('before', pageParam.before)
        params.set('before_id', pageParam.beforeId)
      }
      return apiFetch<Cover[]>(`/covers?${params.toString()}`)
    },
    initialPageParam: null as CoverCursor,
    getNextPageParam: nextCoverCursor,
    // No refetchInterval: changes arrive over the SSE streams useCovers opens.
    // But a stream only reports *changes*, and a generation started elsewhere
    // has usually already announced its last one for the next half minute by
    // the time the user arrives, so a cached list left the grid 30s stale and
    // skipped "Listening" entirely. Arriving must go back to the server.
    staleTime: 0,
    // Keep the current grid on screen while switching status filters.
    placeholderData: keepPreviousData,
  })

/**
 * One cover with its run history. `showFailedRuns` widens the history to runs
 * that produced no artwork; they are kept because their error is the only thing
 * that explains an image-provider refusal.
 */
export const coverQuery = (id: string, showFailedRuns = false) =>
  queryOptions({
    queryKey: queryKeys.cover(id, showFailedRuns),
    queryFn: () => {
      const query = showFailedRuns ? '?allow_failed_revisions=true' : ''
      return apiFetch<Cover>(`/covers/${encodeURIComponent(id)}${query}`)
    },
  })

/**
 * Covers still running, asked once per page load. A generation outlives the tab
 * that started it but the mutation cache does not, so a reload would leave
 * nothing watching. One page is enough: covers come back newest-first.
 */
export const runningCoversQuery = () =>
  queryOptions({
    queryKey: ['covers', 'running'] as const,
    queryFn: () => apiFetch<Cover[]>(`/covers?limit=${PAGE_SIZE}`),
    // Seeds the streams at startup; from then on they are the source of truth.
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  })

export function usePlaylists(args: PlaylistsArgs) {
  return useInfiniteQuery(playlistsInfiniteQuery(args))
}

export function useCovers(filter: CoverFilter = 'all') {
  const queryClient = useQueryClient()
  const query = useInfiniteQuery(coversInfiniteQuery(filter))

  // Every non-terminal cover on screen gets a stream. Keyed on the joined ids so
  // the effect re-runs only when that set changes, not on every refetch.
  const liveIds = (query.data?.pages.flat() ?? [])
    .filter((cover) => !TERMINAL_STATUSES.has(cover.status))
    .map((cover) => cover.id)
  const liveKey = liveIds.join(' ')
  useEffect(() => {
    const unwatch = liveKey === '' ? [] : liveKey.split(' ').map((id) => watchCover(queryClient, id))
    return () => unwatch.forEach((stop) => stop())
  }, [queryClient, liveKey])

  return query
}

export function useCover(id: string, showFailedRuns = false) {
  const queryClient = useQueryClient()
  const query = useQuery(coverQuery(id, showFailedRuns))

  // isLive flips once, so the stream opens and closes once per visit.
  const isLive = query.data != null && !TERMINAL_STATUSES.has(query.data.status)
  useEffect(() => {
    if (!isLive) return
    return watchCover(queryClient, id)
  }, [queryClient, id, isLive])

  return query
}
