import { QueryClient } from '@tanstack/react-query'

import { onSessionExpired } from './api/client'
import { authKeys } from './api/auth'

// Single shared QueryClient. Tuned conservatively for a PWA: data is considered
// fresh for a short window and refetched on focus so covers/playlists stay live.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
      refetchOnWindowFocus: true,
    },
  },
})

// When a 401 survives a token refresh the session is genuinely over, and it can
// surface from anywhere — including a background refetch with no component
// waiting on it. Reacting centrally is what makes the UI fall back to signed-out
// consistently instead of only when the user happens to click something.
//
// Everything the cache holds was fetched for a user who is no longer signed in,
// and on a shared machine it would otherwise still be on screen for whoever
// signs in next — so it is dropped.
//
// Everything *except* the session entry, which is set to null instead. A plain
// clear() also evicts the session query that is failing at this very moment,
// and any mounted useSession then treats it as absent and refetches — which
// 401s, fails to refresh, clears again, and loops. Setting it to null leaves a
// definite "signed out" answer in place and the loop has nothing to feed on.
onSessionExpired(() => {
  queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== 'auth' })
  queryClient.setQueryData(authKeys.session(), null)
})
