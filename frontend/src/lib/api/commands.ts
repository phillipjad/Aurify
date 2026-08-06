// WRITE side. TanStack Query mutations for the API's command endpoints.
// Mirrors the backend's CQRS command handlers (see backend/internal/app/command).
import { useEffect } from 'react'
import { useMutation, useMutationState, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'

import { useSession } from './auth'
import { apiFetch, BASE_URL } from './client'
import { watchCover } from './cover-events'
import { coverQuery, queryKeys, runningCoversQuery, TERMINAL_STATUSES } from './queries'
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

/** Every generation this session has attempted, read from the mutation cache. */
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
 * Hold an SSE stream open for every cover still being generated, from wherever
 * this hook is mounted. It lives in the root layout (see
 * CoverGenerationWatcher), because the streams must survive route changes: the
 * ready toast fires off an event, and the event only arrives if someone is
 * still listening after the user has wandered elsewhere.
 *
 * Two sources feed it. Generations accepted in this tab come from the mutation
 * cache. Generations still running from *before* this page loaded come from a
 * single startup query, since a reload drops the mutation cache while the work
 * carries on server side.
 */
export function useWatchCoverGenerations(): void {
  const queryClient = useQueryClient()
  const entries = useAcceptedGenerations()

  const coverIds = [...new Set(entries.map((entry) => entry.coverId).filter((id): id is string => id != null))]
  const coverResults = useQueries({ queries: coverIds.map((id) => coverQuery(id)) })
  const coverById = new Map(coverIds.map((id, i) => [id, coverResults[i]?.data]))

  // Only for a signed-in caller: the covers list is authenticated, and asking
  // anonymously would put a guaranteed 401 in front of every visit.
  const session = useSession()
  const running = useQuery({ ...runningCoversQuery(), enabled: session.data != null })

  // One stream per still-running cover. The joined key changes only when a
  // generation starts or settles, so streams are not churned on unrelated
  // re-renders, and watchCover refcounts anything watched from both sources.
  const live = new Set<string>()
  for (const id of coverIds) {
    const status = coverById.get(id)?.status
    if (!status || !TERMINAL_STATUSES.has(status)) live.add(id)
  }
  for (const cover of running.data ?? []) {
    // The stream is authoritative once open, so a resumed cover that has since
    // finished is filtered by its live status rather than the startup snapshot.
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
 * The POST returns a *pending* cover and the pipeline runs server side, so a
 * settled mutation is an accepted job, not a finished one. Each accepted cover
 * is followed over its SSE stream (see cover-events.ts) until the status is
 * terminal, and that status — not the request's — is what a row reports.
 *
 * This hook only *reads*; the streams themselves are held open by
 * useWatchCoverGenerations at the root. Holding them here was a real bug:
 * navigating off the playlists page unmounted the only subscriber, closed the
 * stream, and the ready toast never fired — on exactly the route change it
 * exists for.
 */
export function useCoverGenerations(): (playlistId: string) => CoverGeneration {
  const entries = useAcceptedGenerations()

  const coverIds = [...new Set(entries.map((entry) => entry.coverId).filter((id): id is string => id != null))]
  // The cache subscription: events written by the stream land here. The fetch
  // is only the fallback snapshot for a cover accepted before this component
  // mounted; from then on the stream keeps it current.
  const coverResults = useQueries({ queries: coverIds.map((id) => coverQuery(id)) })
  const coverById = new Map(coverIds.map((id, i) => [id, coverResults[i]?.data]))

  return (playlistId: string) => {
    // Last wins: retrying a playlist should report the retry, not the attempt
    // before it.
    const matches = entries.filter((entry) => entry.playlistId === playlistId)
    const latest = matches[matches.length - 1]
    if (!latest) return { status: 'idle', error: undefined }
    if (latest.status !== 'success') return { status: latest.status, error: latest.error }

    // Accepted. The row keeps its spinner until the cover itself settles;
    // before the first detail fetch lands, the job is at best still pending.
    const cover = latest.coverId ? coverById.get(latest.coverId) : undefined
    if (!cover || !TERMINAL_STATUSES.has(cover.status)) return { status: 'pending', error: undefined }
    if (cover.status === 'failed') {
      return { status: 'error', error: cover.error ?? 'Generation failed. Try again in a moment.' }
    }
    return { status: 'success', error: undefined }
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
