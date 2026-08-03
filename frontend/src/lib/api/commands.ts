// WRITE side. TanStack Query mutations for the API's command endpoints.
// Mirrors the backend's CQRS command handlers (see backend/internal/app/command).
import { useMutation, useMutationState, useQueryClient } from '@tanstack/react-query'

import { apiFetch, BASE_URL } from './client'
import { queryKeys } from './queries'
import type { Cover, GenerateCoverRequest, Platform } from './types'

/**
 * Start the DSP OAuth flow.
 *
 * Deliberately not a mutation: both legs are top-level navigations the browser
 * drives, exactly like Sign in with Google. The API answers the login route with
 * a redirect to the provider, and the callback comes back to /playlists carrying
 * `connected` or `connect_error`, so there is no response for the SPA to hold.
 */
export async function connectDsp(platform: Platform) {
  // Touch the session first. The login route requires a signed-in caller, and
  // it is reached by leaving the app — so an access token that expired while the
  // page sat open would answer a bare 401 document instead of the silent refresh
  // an in-app fetch gets. This rotates the pair when it needs to; a failure is
  // not worth blocking on, since the API is the one that decides either way.
  try {
    await apiFetch('/auth/session')
  } catch {
    // Ignored on purpose: navigate and let the API answer.
  }
  window.location.assign(`${BASE_URL}/auth/${encodeURIComponent(platform)}/login`)
}

/**
 * Key every cover generation shares, so they can be read back individually.
 *
 * Each call to mutate creates its own entry in the mutation cache, but the hook's
 * own state only ever reflects the most recent one. Reading the cache under this
 * key is what lets several generations be in flight and be reported separately.
 */
export const coverGenerationKey = ['covers', 'generate'] as const

/** Analyze a playlist and generate a cover, then refresh the covers list. */
export function useGenerateCover() {
  const qc = useQueryClient()
  return useMutation({
    mutationKey: coverGenerationKey,
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

/** The state of one playlist's generation, as the row needs to show it. */
export interface CoverGeneration {
  status: 'pending' | 'success' | 'error' | 'idle'
  error: unknown
}

/**
 * Per-playlist generation state, read from the mutation cache rather than from a
 * single hook's result.
 *
 * Two things follow from the cache being the source. Starting a second generation
 * no longer blanks the first one's spinner, because each row reads its own
 * request instead of a shared "last variables" value. And the state survives a row
 * scrolling out of the windowed list and back, because the cache outlives the
 * component.
 */
export function useCoverGenerations(): (playlistId: string) => CoverGeneration {
  const entries = useMutationState({
    filters: { mutationKey: coverGenerationKey },
    select: (mutation) => ({
      playlistId: (mutation.state.variables as GenerateCoverRequest | undefined)?.playlistId,
      status: mutation.state.status,
      error: mutation.state.error as unknown,
    }),
  })

  return (playlistId: string) => {
    // Last wins: retrying a playlist should report the retry, not the attempt
    // before it.
    const matches = entries.filter((entry) => entry.playlistId === playlistId)
    const latest = matches[matches.length - 1]
    return { status: latest?.status ?? 'idle', error: latest?.error }
  }
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
