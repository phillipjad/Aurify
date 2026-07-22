// WRITE side. TanStack Query mutations for the API's command endpoints.
// Mirrors the backend's CQRS command handlers (see backend/internal/app/command).
import { useMutation, useQueryClient } from '@tanstack/react-query'

import { apiFetch } from './client'
import { queryKeys } from './queries'
import type { Cover, GenerateCoverRequest, Platform } from './types'

/** Start the DSP OAuth flow: fetch the provider auth URL and redirect to it. */
export function useConnectDsp() {
  return useMutation({
    mutationFn: (platform: Platform) => apiFetch<{ authUrl: string }>(`/auth/${encodeURIComponent(platform)}/login`),
    onSuccess: ({ authUrl }) => {
      if (authUrl) window.location.href = authUrl
    },
  })
}

/** Analyze a playlist and generate a cover, then refresh the covers list. */
export function useGenerateCover() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: GenerateCoverRequest) =>
      apiFetch<Cover>('/covers', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.covers() })
    },
  })
}

/** Delete a cover, then refresh the covers list. Returns 204 (no body). */
export function useDeleteCover() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/covers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    onSuccess: (_data, id) => {
      // Drop the detail cache for the gone cover and refetch the list.
      qc.removeQueries({ queryKey: queryKeys.cover(id) })
      void qc.invalidateQueries({ queryKey: queryKeys.covers() })
    },
  })
}
