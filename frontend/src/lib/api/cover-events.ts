// Live cover updates over Server-Sent Events.
//
// GET /covers/{id}/events streams a complete cover snapshot on connect and on
// every change, so there is nothing to poll: events land in the TanStack Query
// cache and every subscribed component re-renders from there. EventSource
// reconnects on its own, and because the first event after a reconnect is a
// full snapshot, a dropped connection can never leave stale state behind.
//
// Streams are shared and refcounted per cover id: the gallery, a playlist row
// and a detail view watching the same generation hold one connection between
// them, which keeps a burst of generations inside the browser's connection
// budget on HTTP/1.1.
import type { InfiniteData, QueryClient } from '@tanstack/react-query'

import { BASE_URL } from './client'
import { queryKeys, TERMINAL_STATUSES } from './queries'
import type { Cover } from './types'

interface CoverStream {
  source: EventSource
  refs: number
}

const streams = new Map<string, CoverStream>()

/**
 * Watch one cover's stream, writing every event into the query cache. Returns
 * an unwatch function; the underlying connection closes when the last watcher
 * leaves or the cover reaches a terminal status, whichever comes first.
 */
export function watchCover(queryClient: QueryClient, id: string): () => void {
  const existing = streams.get(id)
  if (existing) {
    existing.refs += 1
  } else {
    // Cookie-authenticated, like every other API call; EventSource cannot set
    // headers, so withCredentials is the whole auth story.
    const source = new EventSource(`${BASE_URL}/covers/${encodeURIComponent(id)}/events`, {
      withCredentials: true,
    })
    source.addEventListener('cover', (event) => {
      const cover = JSON.parse((event as MessageEvent<string>).data) as Cover
      applyCover(queryClient, cover)
      // The server closes after a terminal event; closing here too stops
      // EventSource from treating that close as an error and reconnecting.
      if (TERMINAL_STATUSES.has(cover.status)) closeStream(id)
    })
    streams.set(id, { source, refs: 1 })
  }

  return () => {
    const stream = streams.get(id)
    if (!stream) return
    stream.refs -= 1
    if (stream.refs <= 0) closeStream(id)
  }
}

function closeStream(id: string) {
  const stream = streams.get(id)
  if (!stream) return
  stream.source.close()
  streams.delete(id)
}

/** Write one cover snapshot into the detail cache and every loaded list page. */
function applyCover(queryClient: QueryClient, cover: Cover) {
  queryClient.setQueryData(queryKeys.cover(cover.id), cover)
  queryClient.setQueriesData<InfiniteData<Cover[]>>({ queryKey: ['covers', 'list'] }, (data) => {
    if (!data) return data
    return {
      ...data,
      pages: data.pages.map((page) => page.map((entry) => (entry.id === cover.id ? cover : entry))),
    }
  })
}
