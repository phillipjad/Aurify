// READ side. TanStack Query options + hooks for the API's query endpoints.
// Mirrors the backend's CQRS query handlers (see backend/internal/app/query).
import { queryOptions, useQuery } from '@tanstack/react-query'

import { apiFetch } from './client'
import type { Cover, Platform, Playlist } from './types'

export const queryKeys = {
  playlists: (platform: Platform) => ['playlists', platform] as const,
  covers: () => ['covers'] as const,
  cover: (id: string) => ['covers', id] as const,
}

export const playlistsQuery = (platform: Platform) =>
  queryOptions({
    queryKey: queryKeys.playlists(platform),
    queryFn: () =>
      apiFetch<Playlist[]>(
        `/playlists?platform=${encodeURIComponent(platform)}`,
      ),
  })

export const coversQuery = () =>
  queryOptions({
    queryKey: queryKeys.covers(),
    queryFn: () => apiFetch<Cover[]>('/covers'),
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
