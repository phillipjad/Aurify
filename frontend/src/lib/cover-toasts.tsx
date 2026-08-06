// "Your cover is ready" toasts, fed by the SSE stream (see
// lib/api/cover-events.ts) and rendered by the shadcn/Sonner <Toaster> mounted
// in the root layout. This module owns only the firing decision; everything
// visual is Sonner's.
import { Link } from '@tanstack/react-router'
import { Sparkles } from 'lucide-react'
import { toast } from 'sonner'

import type { Cover } from './api/types'

// Covers already announced this session. One toast per generation: a
// reconnecting stream can replay a ready snapshot, and the user finished one
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
 * Toast a cover event if it deserves one: the cover just became ready, and the
 * user is not already looking at the covers section — the gallery and the
 * detail page render the same stream live, so a toast there would announce
 * what is already on screen.
 *
 * The route check reads location at fire time on purpose. The toast's job is
 * "you are elsewhere, here is a way over"; whether it fires is a property of
 * where the user is when the event lands.
 */
export function maybeToastCoverReady(cover: Cover): void {
  if (cover.status !== 'ready') return
  if (window.location.pathname.startsWith('/covers')) return
  if (toastedCovers.has(cover.id)) return
  toastedCovers.add(cover.id)

  toast('Your cover is ready', {
    id: cover.id,
    description: cover.playlistName || 'Untitled playlist',
    duration: 8_000,
    icon: <Sparkles aria-hidden="true" className="size-4 text-primary" />,
    // A real hyperlink rather than Sonner's action button: it navigates
    // client-side instantly, and middle-click/new-tab keep working. It renders
    // inside the root layout's <Toaster>, so router context is available.
    action: (
      <Link
        to="/covers/$coverId"
        params={{ coverId: cover.id }}
        onClick={() => toast.dismiss(cover.id)}
        className="ml-auto shrink-0 rounded-md px-2.5 py-1.5 text-sm font-medium text-primary transition-colors hover:bg-primary/10 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card focus-visible:outline-none"
      >
        View it
      </Link>
    ),
  })
}
