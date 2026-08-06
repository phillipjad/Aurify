// "Your cover is ready" toasts, fed by the SSE stream (see
// lib/api/cover-events.ts) and rendered by the shadcn/Sonner <Toaster> mounted
// in the root layout. This module owns the firing decision and the toast's
// content; everything structural is Sonner's.
import { Link } from '@tanstack/react-router'
import { AlertTriangle, Sparkles } from 'lucide-react'
import { toast } from 'sonner'

import { ImageWithFallback } from '@/components/image-with-fallback'
import { markCoverUnread } from './cover-unread'
import type { Cover } from './api/types'

/**
 * Long enough to read and reach for, short enough that a burst of finished
 * generations clears itself. It can be this brief because the Covers nav
 * carries an unread badge until the user actually looks (see cover-unread.ts).
 */
const TOAST_DURATION_MS = 8_000

// Covers already announced this session. One toast per generation: a
// reconnecting stream can replay a terminal snapshot, and the user finished one
// cover, not two. Exported so tests can reset it.
export const toastedCovers = new Set<string>()

/**
 * Retire any cover toasts still on screen. Called when the user enters the
 * covers section: a toast that fired correctly elsewhere should not follow
 * them into the one place it is redundant. Dismissing an id that has already
 * gone is a no-op, so the whole session's set is safe to sweep.
 */
export function dismissCoverToasts(): void {
  for (const id of toastedCovers) toast.dismiss(id)
}

/**
 * Announce a cover that has just settled, if it deserves announcing: it
 * reached a terminal status, and the user is not already looking at the covers
 * section, where the gallery and the detail view render the same stream live.
 *
 * Failure is announced as loudly as success. It is the outcome that needs the
 * user to do something, and before this it was the only one that arrived in
 * silence.
 *
 * The route check reads location at fire time on purpose: whether this fires is
 * a property of where the user is when the event lands.
 */
export function maybeToastCoverSettled(cover: Cover): void {
  if (cover.status !== 'ready' && cover.status !== 'failed') return
  if (window.location.pathname.startsWith('/covers')) return
  if (toastedCovers.has(cover.id)) return
  toastedCovers.add(cover.id)
  markCoverUnread(cover.id)

  const playlist = cover.playlistName || 'Untitled playlist'

  if (cover.status === 'failed') {
    toast('Generation didn’t finish', {
      id: cover.id,
      description: playlist,
      duration: TOAST_DURATION_MS,
      icon: <AlertTriangle aria-hidden="true" className="size-5 text-destructive-text" />,
      // The detail page carries the reason and the Regenerate control, so the
      // recovery path is one link rather than a second copy of both.
      action: <ToastLink coverID={cover.id}>See why</ToastLink>,
    })
    return
  }

  toast('Your cover is ready', {
    id: cover.id,
    description: playlist,
    duration: TOAST_DURATION_MS,
    // The artwork is the whole point of the product and the thing the user
    // waited for, so the notification carries it rather than describing it.
    icon: <CoverThumbnail cover={cover} />,
    action: <ToastLink coverID={cover.id}>View it</ToastLink>,
  })
}

/** The generated artwork, or the product mark while it cannot be loaded. */
function CoverThumbnail({ cover }: { cover: Cover }) {
  return (
    <ImageWithFallback
      src={cover.imageUrl}
      // Decorative: the title and playlist name next to it already say what
      // this is, and a second reading of the same fact is noise on a toast.
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

/**
 * A real hyperlink rather than Sonner's action button: it navigates
 * client-side instantly, and middle-click and open-in-new-tab keep working. It
 * renders inside the root layout's <Toaster>, so router context is available.
 */
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
