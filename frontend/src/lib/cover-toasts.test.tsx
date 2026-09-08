import { beforeEach, describe, expect, it, vi } from 'vite-plus/test'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'

import { CoverGenerationWatcher } from '@/components/cover-generation-watcher'
import { Toaster } from '@/components/ui/sonner'
import { dismissCoverToasts, maybeToastCoverSettled, outstandingCoverToasts, toastedCovers } from '@/lib/cover-toasts'
import { clearUnreadCovers, getUnreadCoverCount } from '@/lib/cover-unread'
import { renderWithProviders } from '@/test/render'
import type { Cover } from '@/lib/api/types'

const READY_COVER: Cover = {
  id: 'cover1',
  status: 'ready',
  platform: 'spotify',
  playlistId: 'sp1',
  playlistName: 'Morning Coffee',
  runCount: 1,
  createdAt: '2026-08-06T00:00:00Z',
  updatedAt: '2026-08-06T00:05:00Z',
}

/**
 * The id a toast is filed under: the cover names the playlist, so what
 * identifies one announcement is the run that produced it.
 */
const runId = (coverId: string, updatedAt = READY_COVER.updatedAt) => `${coverId}:${updatedAt}`

beforeEach(() => {
  // Sonner, the once-per-cover guard, the on-screen set and the unread badge
  // all keep module state; reset all four.
  toast.dismiss()
  toastedCovers.clear()
  outstandingCoverToasts.clear()
  clearUnreadCovers()
  window.history.replaceState(null, '', '/playlists')
})

/**
 * Render the Toaster inside the router harness and wait for it to mount. The
 * harness mounts routes asynchronously, and Sonner does not replay toasts
 * fired before its Toaster exists — a test-only race: the app mounts the
 * Toaster in the root layout at first paint, long before any SSE event.
 */
async function renderToaster(path = '/playlists') {
  const utils = renderWithProviders(
    <>
      <CoverGenerationWatcher />
      <Toaster />
    </>,
    { path },
  )
  await screen.findByRole('region', { name: /notifications/i })
  return utils
}

describe('maybeToastCoverSettled', () => {
  it('announces a ready cover with a link straight to it', async () => {
    const { router } = await renderToaster()
    act(() => maybeToastCoverSettled(READY_COVER))

    expect(await screen.findByText('Your cover is ready')).toBeInTheDocument()
    expect(screen.getByText('Morning Coffee')).toBeInTheDocument()

    const link = screen.getByRole('link', { name: 'View it' })
    expect(link).toHaveAttribute('href', '/covers/cover1')

    // The link is a client-side navigation: clicking reroutes immediately.
    await userEvent.click(link)
    expect(router.state.location.pathname).toBe('/covers/cover1')
  })

  it('stays quiet for a cover that is still working', async () => {
    await renderToaster()
    act(() => maybeToastCoverSettled({ ...READY_COVER, status: 'generating' }))

    expect(screen.queryByText('Your cover is ready')).not.toBeInTheDocument()
    expect(getUnreadCoverCount()).toBe(0)
  })

  // Failure is the outcome that needs the user to do something, and it used to
  // be the only one that arrived in silence.
  it('announces a failed generation with a way to find out why', async () => {
    await renderToaster()
    act(() => maybeToastCoverSettled({ ...READY_COVER, status: 'failed', error: 'imagegen refused the prompt' }))

    expect(await screen.findByText(/didn.t finish/i)).toBeInTheDocument()
    expect(screen.getByText('Morning Coffee')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'See why' })).toHaveAttribute('href', '/covers/cover1')
  })

  // The toast expires in 8 seconds; the badge is what makes that acceptable.
  it('counts a settled cover as unread until the user reaches the gallery', async () => {
    const { router } = await renderToaster()
    act(() => maybeToastCoverSettled(READY_COVER))
    expect(getUnreadCoverCount()).toBe(1)

    await act(async () => {
      await router.navigate({ to: '/covers' })
    })

    await waitFor(() => expect(getUnreadCoverCount()).toBe(0))
  })

  // The gallery and the detail page render the same stream live, so a toast
  // there would announce what is already on screen.
  it('stays quiet while the user is in the covers section', async () => {
    await renderToaster('/covers')

    window.history.replaceState(null, '', '/covers')
    act(() => maybeToastCoverSettled(READY_COVER))
    window.history.replaceState(null, '', '/covers/cover1')
    act(() => maybeToastCoverSettled(READY_COVER))

    expect(screen.queryByText('Your cover is ready')).not.toBeInTheDocument()
  })

  // A generation can finish while the user is on their way to the gallery. The
  // toast fired correctly at the time, but once they arrive it points at a
  // cover already on screen, so it must not follow them in.
  it('retires an outstanding toast when the user reaches the covers section', async () => {
    const { router } = await renderToaster()
    act(() => maybeToastCoverSettled(READY_COVER))
    expect(await screen.findByText('Your cover is ready')).toBeInTheDocument()

    await act(async () => {
      await router.navigate({ to: '/covers' })
    })

    await waitFor(() => expect(screen.queryByText('Your cover is ready')).not.toBeInTheDocument())
  })

  // Entering /covers retires every outstanding toast, and five cards vanishing
  // in the same frame reads as a glitch rather than as the section clearing.
  // Driven through the store rather than the DOM: what is pinned here is the
  // order and the spacing, and jsdom runs none of the animation either way.
  it('retires a stack one at a time, oldest first', () => {
    for (const id of ['first', 'second', 'third']) {
      maybeToastCoverSettled({ ...READY_COVER, id })
    }

    const dismiss = vi.spyOn(toast, 'dismiss')
    vi.useFakeTimers()
    try {
      dismissCoverToasts()
      const dismissed = () => dismiss.mock.calls.map(([id]) => id)

      // Nothing leaves synchronously, and then one every 250ms.
      vi.advanceTimersByTime(0)
      expect(dismissed()).toEqual([runId('first')])

      vi.advanceTimersByTime(249)
      expect(dismissed()).toEqual([runId('first')])

      vi.advanceTimersByTime(1)
      expect(dismissed()).toEqual([runId('first'), runId('second')])

      vi.advanceTimersByTime(250)
      expect(dismissed()).toEqual([runId('first'), runId('second'), runId('third')])
    } finally {
      vi.useRealTimers()
      dismiss.mockRestore()
    }
  })

  // The stagger above only covers the route change. Covers that settle together
  // would still expire together 8s later, so the gap is built into the duration
  // at fire time and the same rhythm holds however the stack is retired.
  it('spreads the expiry of a burst rather than letting it land at once', () => {
    for (const id of ['one', 'two', 'three']) {
      maybeToastCoverSettled({ ...READY_COVER, id })
    }

    const burst = ['one', 'two', 'three'].map((id) => runId(id))
    const durations = toast
      .getToasts()
      .filter((t) => burst.includes(String(t.id)))
      .map((t) => ('duration' in t ? t.duration : undefined))

    expect(durations).toEqual([8_000, 8_250, 8_500])
  })

  // A reconnecting stream can replay a ready snapshot; the user finished one
  // generation, not two. The replay carries the same updatedAt, which is what
  // makes it recognisable as the same run.
  it('fires once per run', async () => {
    await renderToaster()
    act(() => {
      maybeToastCoverSettled(READY_COVER)
      maybeToastCoverSettled(READY_COVER)
    })

    expect(await screen.findAllByText('Your cover is ready')).toHaveLength(1)
  })

  // A cover's id is now the playlist's, so it is the same across every
  // regeneration. Guarding on it would have made every regeneration finish in
  // silence, and told Sonner to update the previous card rather than raise a
  // new one.
  it('announces every regeneration of the same cover', async () => {
    await renderToaster()
    act(() => maybeToastCoverSettled(READY_COVER))
    expect(await screen.findAllByText('Your cover is ready')).toHaveLength(1)

    act(() => maybeToastCoverSettled({ ...READY_COVER, updatedAt: '2026-08-06T09:30:00Z' }))
    // waitFor rather than findAllByText: that resolves on the first match, which
    // is the card already on screen, and would pass whether or not a second one
    // ever arrives.
    await waitFor(() => expect(screen.getAllByText('Your cover is ready')).toHaveLength(2))
  })

  // The link goes to the playlist's cover; only the dismissal is per-run.
  it('links a regenerated cover to the same detail page', async () => {
    await renderToaster()
    act(() => maybeToastCoverSettled({ ...READY_COVER, updatedAt: '2026-08-06T09:30:00Z' }))

    expect(await screen.findByRole('link', { name: 'View it' })).toHaveAttribute('href', '/covers/cover1')
  })
})
