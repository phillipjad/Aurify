import { useEffect, useRef } from 'react'
import { Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'

interface LoadMoreProps {
  hasNextPage: boolean
  isFetchingNextPage: boolean
  fetchNextPage: () => void
  /** Accessible label for the fallback button, e.g. "Load more covers". */
  label: string
}

/**
 * Infinite-scroll trigger: an off-screen sentinel loads the next page as it
 * nears the viewport, with a real button as the keyboard / no-observer fallback
 * (and the visible affordance that more exists).
 */
export function LoadMore({ hasNextPage, isFetchingNextPage, fetchNextPage, label }: LoadMoreProps) {
  const sentinel = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = sentinel.current
    // No observer (SSR, jsdom, ancient browsers) → the button is the fallback,
    // so auto-loading is a progressive enhancement, not a requirement.
    if (!el || !hasNextPage || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && !isFetchingNextPage) fetchNextPage()
      },
      { rootMargin: '400px' },
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  if (!hasNextPage && !isFetchingNextPage) return null

  return (
    <div ref={sentinel} className="flex justify-center pt-2">
      {isFetchingNextPage ? (
        <span className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 aria-hidden="true" data-motion="busy" className="size-4 animate-spin" />
          Loading more…
        </span>
      ) : (
        <Button variant="outline" size="sm" onClick={fetchNextPage}>
          {label}
        </Button>
      )}
    </div>
  )
}
