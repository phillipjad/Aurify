// READ side. TanStack Query definitions for the API's query endpoints.
import { infiniteQueryOptions, keepPreviousData, queryOptions, useInfiniteQuery, useQuery } from '@tanstack/react-query'

import { apiFetch } from './client'
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
  cover: (id: string) => ['covers', 'detail', id] as const,
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
const TERMINAL_STATUSES: ReadonlySet<CoverStatus> = new Set<CoverStatus>(['ready', 'failed'])

export const coversInfiniteQuery = (filter: CoverFilter = 'all') =>
  infiniteQueryOptions({
    queryKey: queryKeys.coversList(filter),
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(pageParam) })
      if (filter !== 'all') params.set('status', filter)
      return apiFetch<Cover[]>(`/covers?${params.toString()}`)
    },
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => (lastPage.length < PAGE_SIZE ? undefined : allPages.length * PAGE_SIZE),
    // Generation is a multi-stage pipeline, so a cover's status changes server
    // side with no client event to hang off. Poll (refetching every loaded page)
    // while anything is still moving and stop once everything has settled —
    // otherwise the status badge shows "generating" indefinitely and lies.
    refetchInterval: (query) => {
      const pages = query.state.data?.pages
      if (!pages) return false
      const stillWorking = pages.some((page) => page.some((cover) => !TERMINAL_STATUSES.has(cover.status)))
      return stillWorking ? 3_000 : false
    },
    // Keep the current grid on screen while switching status filters.
    placeholderData: keepPreviousData,
  })

export const coverQuery = (id: string) =>
  queryOptions({
    queryKey: queryKeys.cover(id),
    queryFn: () => apiFetch<Cover>(`/covers/${encodeURIComponent(id)}`),
  })

export function usePlaylists(args: PlaylistsArgs) {
  return useInfiniteQuery(playlistsInfiniteQuery(args))
}

export function useCovers(filter: CoverFilter = 'all') {
  return useInfiniteQuery(coversInfiniteQuery(filter))
}

export function useCover(id: string) {
  return useQuery(coverQuery(id))
}
