// READ side. TanStack Query options + hooks for the API's query endpoints.
// Mirrors the backend's CQRS query handlers (see backend/internal/app/query).
import { infiniteQueryOptions, queryOptions, useInfiniteQuery, useQuery } from '@tanstack/react-query'

import { apiFetch } from './client'
import type { Cover, CoverStatus, Platform, Playlist } from './types'

/** How many covers a single page requests (matches the API's default limit). */
export const COVERS_PAGE_SIZE = 20

export const queryKeys = {
  playlists: (platform: Platform) => ['playlists', platform] as const,
  covers: () => ['covers'] as const,
  cover: (id: string) => ['covers', id] as const,
}

export const playlistsQuery = (platform: Platform) =>
  queryOptions({
    queryKey: queryKeys.playlists(platform),
    queryFn: () => apiFetch<Playlist[]>(`/playlists?platform=${encodeURIComponent(platform)}`),
  })

/** Cover statuses that will never change again without a new user action. */
const TERMINAL_STATUSES: ReadonlySet<CoverStatus> = new Set<CoverStatus>(['ready', 'failed'])

export const coversInfiniteQuery = () =>
  infiniteQueryOptions({
    queryKey: queryKeys.covers(),
    queryFn: ({ pageParam }) => apiFetch<Cover[]>(`/covers?limit=${COVERS_PAGE_SIZE}&offset=${pageParam}`),
    initialPageParam: 0,
    // A short page means the end; otherwise the next offset is one page further.
    getNextPageParam: (lastPage, allPages) =>
      lastPage.length < COVERS_PAGE_SIZE ? undefined : allPages.length * COVERS_PAGE_SIZE,
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
  })

export const coverQuery = (id: string) =>
  queryOptions({
    queryKey: queryKeys.cover(id),
    queryFn: () => apiFetch<Cover>(`/covers/${encodeURIComponent(id)}`),
  })

export function usePlaylists(platform: Platform) {
  return useQuery(playlistsQuery(platform))
}

export function useCovers() {
  return useInfiniteQuery(coversInfiniteQuery())
}

export function useCover(id: string) {
  return useQuery(coverQuery(id))
}
