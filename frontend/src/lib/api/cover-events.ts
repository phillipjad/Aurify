// Live cover updates over SSE. Full snapshots land in the query cache, so a
// reconnect cannot leave stale state, and streams are refcounted per cover so
// the gallery, a row and a detail view share one connection.
import type { InfiniteData, QueryClient } from '@tanstack/react-query'

import { maybeToastCoverSettled } from '../cover-toasts'
import { BASE_URL } from './client'
import { queryKeys, TERMINAL_STATUSES } from './queries'
import type { Cover } from './types'

interface CoverStream {
  source: EventSource
  refs: number
}

const streams = new Map<string, CoverStream>()

// Covers seen this tab, so applyCover can spot a gallery that never listed one.
export const seenCovers = new Set<string>()

/**
 * Watch one cover, writing events into the query cache. The connection closes
 * when the last watcher leaves or the cover settles, whichever is first.
 */
export function watchCover(queryClient: QueryClient, id: string): () => void {
  const existing = streams.get(id)
  if (existing) {
    existing.refs += 1
  } else {
    // EventSource cannot set headers, so withCredentials is the whole auth story.
    const source = new EventSource(`${BASE_URL}/covers/${encodeURIComponent(id)}/events`, {
      withCredentials: true,
    })
    source.addEventListener('cover', (event) => {
      const cover = JSON.parse((event as MessageEvent<string>).data) as Cover
      applyCover(queryClient, cover)
      maybeToastCoverSettled(cover)
      // Closing here too stops EventSource treating the server's close as an
      // error and reconnecting.
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

  // A generation started after the gallery loaded is in none of its pages, so
  // updating in place cannot reach it. Refetch once per cover and let the server
  // place the row, rather than splicing into an offset-paginated list.
  if (!seenCovers.has(cover.id)) {
    seenCovers.add(cover.id)
    void queryClient.invalidateQueries({ queryKey: ['covers', 'list'] })
  }
}
