import { QueryClient } from '@tanstack/react-query'

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
