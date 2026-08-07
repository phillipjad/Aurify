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

// One toast per generation: a reconnecting stream can replay a terminal snapshot.
export const toastedCovers = new Set<string>()

/** Retire outstanding toasts on entering the covers section, where they would
 * point at what is already on screen. Dismissing a gone id is a no-op. */
export function dismissCoverToasts(): void {
  for (const id of toastedCovers) toast.dismiss(id)
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
  markCoverUnread()

  const playlist = cover.playlistName || 'Untitled playlist'

  if (cover.status === 'failed') {
    toast('Generation didn’t finish', {
      id: cover.id,
      description: playlist,
      duration: TOAST_DURATION_MS,
      icon: <AlertTriangle aria-hidden="true" className="size-5 text-destructive-text" />,
      // The detail page already holds the reason and the Regenerate control.
      action: <ToastLink coverID={cover.id}>See why</ToastLink>,
    })
    return
  }

  toast('Your cover is ready', {
    id: cover.id,
    description: playlist,
    duration: TOAST_DURATION_MS,
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
