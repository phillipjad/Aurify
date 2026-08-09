// Cover toasts, fed by the SSE stream and rendered by the <Toaster> in the root
// layout. This owns the firing decision and the content; the rest is Sonner's.
import { Link } from '@tanstack/react-router'
import { AlertTriangle, Sparkles } from 'lucide-react'
import { toast } from 'sonner'

import { ImageWithFallback } from '@/components/image-with-fallback'
import { markCoverUnread } from './cover-unread'
import type { Cover } from './api/types'

// Brief because the Covers nav badge keeps the record (see cover-unread.ts).
const TOAST_DURATION_MS = 8_000

// Gap between each toast leaving when the whole stack is retired at once.
const DISMISS_STAGGER_MS = 250

/**
 * One toast per generation: a reconnecting stream can replay a terminal
 * snapshot. This keeps every id for the life of the tab, which is how long a
 * replay stays possible — cover-events.ts closes a cover's stream once it
 * settles, but a component mounting later reopens one and the server resends
 * the terminal snapshot. Only a finished generation adds an id, so it grows at
 * human pace and dies with the page.
 */
export const toastedCovers = new Set<string>()

/**
 * The ones still on screen, which is what a mass dismissal staggers over. Kept
 * apart from the guard above because it has the opposite lifetime: sonner
 * empties it through its own callbacks whichever way a toast goes, whether that
 * is the timeout, the close button, a swipe or toast.dismiss.
 */
export const outstandingCoverToasts = new Set<string>()

/**
 * Retire outstanding toasts on entering the covers section, where they would
 * point at what is already on screen.
 *
 * One at a time rather than all together: five cards going in the same frame
 * reads as a glitch, where a cascade reads as the section clearing itself.
 * Oldest first, which on a bottom-anchored stack is top down, and that is the
 * calm direction: a toast's offset is measured from the cards in front of it,
 * so removing the one above never shifts the ones below, and nothing has to
 * move over while its neighbour is still leaving.
 *
 * The set is emptied up front, so reaching /covers and then a cover's own page
 * does not schedule the same toast twice on a fresh clock.
 */
export function dismissCoverToasts(): void {
  const leaving = [...outstandingCoverToasts]
  outstandingCoverToasts.clear()
  leaving.forEach((id, i) => setTimeout(() => toast.dismiss(id), i * DISMISS_STAGGER_MS))
}

/**
 * Announce a settled cover, unless the user is already in the covers section
 * watching the same stream. Failure is announced as loudly as success: it is
 * the outcome that needs them to act. Location is read at fire time, since that
 * is when where-they-are is decided.
 */
export function maybeToastCoverSettled(cover: Cover): void {
  if (cover.status !== 'ready' && cover.status !== 'failed') return
  if (window.location.pathname.startsWith('/covers')) return
  if (toastedCovers.has(cover.id)) return
  toastedCovers.add(cover.id)
  // Counted before this one joins them, so the first toast keeps the plain duration.
  const ahead = outstandingCoverToasts.size
  outstandingCoverToasts.add(cover.id)
  markCoverUnread()

  const playlist = cover.playlistName || 'Untitled playlist'
  const forget = () => outstandingCoverToasts.delete(cover.id)
  const common = {
    id: cover.id,
    description: playlist,
    // Generations that settle together would otherwise expire together eight
    // seconds later, which is the same wall of cards leaving at once that
    // dismissCoverToasts exists to break up. Each toast already waiting pushes
    // this one a gap further out, so a burst leaves in the same rhythm however
    // it is retired. Arrivals that are already further apart than the gap are
    // unaffected in any way anyone can see.
    duration: TOAST_DURATION_MS + ahead * DISMISS_STAGGER_MS,
    // Every way out of the stack, so nothing that has left is still counted as
    // on screen: onDismiss covers the close button, a swipe and toast.dismiss.
    onDismiss: forget,
    onAutoClose: forget,
  }

  if (cover.status === 'failed') {
    toast('Generation didn’t finish', {
      ...common,
      icon: <AlertTriangle aria-hidden="true" className="size-5 text-destructive-text" />,
      // The detail page already holds the reason and the Regenerate control.
      action: <ToastLink coverID={cover.id}>See why</ToastLink>,
    })
    return
  }

  toast('Your cover is ready', {
    ...common,
    // The artwork is what they waited for, so the toast carries it.
    icon: <CoverThumbnail cover={cover} />,
    action: <ToastLink coverID={cover.id}>View it</ToastLink>,
  })
}

/** The generated artwork, or the product mark while it cannot be loaded. */
function CoverThumbnail({ cover }: { cover: Cover }) {
  return (
    <ImageWithFallback
      src={cover.imageUrl}
      // Decorative: the title and playlist name beside it already say this.
      alt=""
      className="size-11 shrink-0 rounded-md object-cover"
      fallback={
        <span
          aria-hidden="true"
          className="flex size-11 shrink-0 items-center justify-center rounded-md bg-muted text-primary"
        >
          <Sparkles className="size-5" />
        </span>
      }
    />
  )
}

/** A real link rather than Sonner's action button, so middle-click and
 * open-in-new-tab keep working. Router context comes from the root layout. */
function ToastLink({ coverID, children }: { coverID: string; children: string }) {
  return (
    <Link
      to="/covers/$coverId"
      params={{ coverId: coverID }}
      onClick={() => toast.dismiss(coverID)}
      className="ml-auto shrink-0 rounded-md px-2.5 py-1.5 text-sm font-medium text-primary transition-colors hover:bg-primary/10 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card focus-visible:outline-none"
    >
      {children}
    </Link>
  )
}
