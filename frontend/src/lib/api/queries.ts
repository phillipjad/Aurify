// READ side. TanStack Query options + hooks for the API's query endpoints.
// Mirrors the backend's CQRS query handlers (see backend/internal/app/query).
import { queryOptions, useQuery } from '@tanstack/react-query'

import { apiFetch } from './client'
import type { Cover, CoverStatus, Platform, Playlist } from './types'

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

export const coversQuery = () =>
  queryOptions({
    queryKey: queryKeys.covers(),
    queryFn: () => apiFetch<Cover[]>('/covers'),
    // Generation is a multi-stage pipeline, so a cover's status changes server
    // side with no client event to hang off. Poll while anything is still
    // moving and stop as soon as everything has settled — otherwise the status
    // badge shows "generating" indefinitely and quietly lies to the user.
    refetchInterval: (query) => {
      const covers = query.state.data
      if (!covers) return false
      return covers.some((cover) => !TERMINAL_STATUSES.has(cover.status)) ? 3_000 : false
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
  return useQuery(coversQuery())
}

export function useCover(id: string) {
  return useQuery(coverQuery(id))
}
