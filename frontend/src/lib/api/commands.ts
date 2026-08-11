// WRITE side. TanStack Query mutations for the API's command endpoints.
// Mirrors the backend's CQRS command handlers (see backend/internal/app/command).
import { useEffect } from 'react'
import { useMutation, useMutationState, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'

import { useSession } from './auth'
import { apiFetch, BASE_URL } from './client'
import { watchCover } from './cover-events'
import { coverQuery, queryKeys, runningCoversQuery, TERMINAL_STATUSES } from './queries'
import type { Cover, CoverStatus, GenerateCoverRequest, Platform } from './types'

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
  /** The live stage behind `pending`, so the row says what is happening. */
  stage?: CoverStatus
}

/** Every generation this session attempted, from the mutation cache. */
function useAcceptedGenerations() {
  return useMutationState({
    filters: { mutationKey: coverGenerationKey },
    select: (mutation) => ({
      playlistId: (mutation.state.variables as GenerateCoverRequest | undefined)?.playlistId,
      coverId: (mutation.state.data as Cover | undefined)?.id,
      status: mutation.state.status,
      error: mutation.state.error as unknown,
    }),
  })
}

/**
 * Hold a stream open for every running cover. Lives in the root layout, because
 * the streams must survive route changes: the ready toast fires off an event,
 * which only arrives if someone is still listening. Fed by the mutation cache
 * for this tab's generations, and a startup query for ones a reload forgot.
 */
export function useWatchCoverGenerations(): void {
  const queryClient = useQueryClient()
  const entries = useAcceptedGenerations()

  const coverIds = [...new Set(entries.map((entry) => entry.coverId).filter((id): id is string => id != null))]
  const coverResults = useQueries({ queries: coverIds.map((id) => coverQuery(id)) })
  const coverById = new Map(coverIds.map((id, i) => [id, coverResults[i]?.data]))

  // Anonymously this is a guaranteed 401 in front of every visit.
  const session = useSession()
  const running = useQuery({ ...runningCoversQuery(), enabled: session.data != null })

  // The joined key changes only when a generation starts or settles, so streams
  // are not churned on unrelated re-renders. watchCover refcounts the overlap.
  const live = new Set<string>()
  for (const id of coverIds) {
    const status = coverById.get(id)?.status
    if (!status || !TERMINAL_STATUSES.has(status)) live.add(id)
  }
  for (const cover of running.data ?? []) {
    // The stream outranks the startup snapshot once it has said anything.
    const current = queryClient.getQueryData<Cover>(queryKeys.cover(cover.id)) ?? cover
    if (!TERMINAL_STATUSES.has(current.status)) live.add(cover.id)
  }

  const liveKey = [...live].sort().join(' ')
  useEffect(() => {
    const unwatch = liveKey === '' ? [] : liveKey.split(' ').map((id) => watchCover(queryClient, id))
    return () => unwatch.forEach((stop) => stop())
  }, [queryClient, liveKey])
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
 *
 * The POST returns a *pending* cover, so a settled mutation is an accepted job,
 * not a finished one: the row reports the cover's status, not the request's.
 * Read-only. The streams are held at the root, because holding them here closed
 * them on the very navigation the toast exists for.
 */
export function useCoverGenerations(): (playlistId: string) => CoverGeneration {
  const entries = useAcceptedGenerations()

  const coverIds = [...new Set(entries.map((entry) => entry.coverId).filter((id): id is string => id != null))]
  // Subscribes to what the stream writes; the fetch is only the first snapshot.
  const coverResults = useQueries({ queries: coverIds.map((id) => coverQuery(id)) })
  const coverById = new Map(coverIds.map((id, i) => [id, coverResults[i]?.data]))

  return (playlistId: string) => {
    // Last wins: retrying a playlist should report the retry, not the attempt
    // before it.
    const matches = entries.filter((entry) => entry.playlistId === playlistId)
    const latest = matches[matches.length - 1]
    if (!latest) return { status: 'idle', error: undefined }
    if (latest.status !== 'success') return { status: latest.status, error: latest.error }

    // The row keeps its spinner until the cover itself settles.
    const cover = latest.coverId ? coverById.get(latest.coverId) : undefined
    if (!cover || !TERMINAL_STATUSES.has(cover.status)) {
      return { status: 'pending', error: undefined, stage: cover?.status }
    }
    if (cover.status === 'failed') {
      return { status: 'error', error: cover.error ?? 'Generation failed. Try again in a moment.' }
    }
    return { status: 'success', error: undefined }
  }
}

/**
 * Delete a cover and every run it has had, then refresh the covers list.
 * Returns 204 (no body).
 */
export function useDeleteCover() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/covers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    onSuccess: (_data, id) => {
      // Drop the detail cache for the gone cover and refetch the list. The key
      // is a prefix, so both variants of the failed-runs toggle go with it.
      qc.removeQueries({ queryKey: ['covers', 'detail', id] })
      void qc.invalidateQueries({ queryKey: queryKeys.covers() })
    },
  })
}

/**
 * Delete one run, keeping the cover. Returns 204 (no body).
 *
 * The tile can change as a result — dropping the newest successful run falls
 * back to the one before it — so this refreshes the list as well as the detail.
 */
export function useDeleteCoverRevision(coverId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (revisionId: string) =>
      apiFetch<void>(`/covers/${encodeURIComponent(coverId)}/revisions/${encodeURIComponent(revisionId)}`, {
        method: 'DELETE',
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.covers() })
    },
  })
}
