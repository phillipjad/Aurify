import { beforeEach, describe, expect, it } from 'vitest'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'

import { CoverGenerationWatcher } from '@/components/cover-generation-watcher'
import { Toaster } from '@/components/ui/sonner'
import { maybeToastCoverReady, toastedCovers } from '@/lib/cover-toasts'
import { renderWithProviders } from '@/test/render'
import type { Cover } from '@/lib/api/types'

const READY_COVER: Cover = {
  id: 'cover1',
  status: 'ready',
  platform: 'spotify',
  playlistId: 'sp1',
  playlistName: 'Morning Coffee',
  createdAt: '2026-08-06T00:00:00Z',
}

beforeEach(() => {
  // Sonner and the once-per-cover guard both keep module state; reset both.
  toast.dismiss()
  toastedCovers.clear()
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

describe('maybeToastCoverReady', () => {
  it('announces a ready cover with a link straight to it', async () => {
    const { router } = await renderToaster()
    act(() => maybeToastCoverReady(READY_COVER))

    expect(await screen.findByText('Your cover is ready')).toBeInTheDocument()
    expect(screen.getByText('Morning Coffee')).toBeInTheDocument()

    const link = screen.getByRole('link', { name: 'View it' })
    expect(link).toHaveAttribute('href', '/covers/cover1')

    // The link is a client-side navigation: clicking reroutes immediately.
    await userEvent.click(link)
    expect(router.state.location.pathname).toBe('/covers/cover1')
  })

  it('fires only for covers that just became ready', async () => {
    await renderToaster()
    act(() => maybeToastCoverReady({ ...READY_COVER, status: 'generating' }))

    expect(screen.queryByText('Your cover is ready')).not.toBeInTheDocument()
  })

  // The gallery and the detail page render the same stream live, so a toast
  // there would announce what is already on screen.
  it('stays quiet while the user is in the covers section', async () => {
    await renderToaster('/covers')

    window.history.replaceState(null, '', '/covers')
    act(() => maybeToastCoverReady(READY_COVER))
    window.history.replaceState(null, '', '/covers/cover1')
    act(() => maybeToastCoverReady(READY_COVER))

    expect(screen.queryByText('Your cover is ready')).not.toBeInTheDocument()
  })

  // A generation can finish while the user is on their way to the gallery. The
  // toast fired correctly at the time, but once they arrive it points at a
  // cover already on screen, so it must not follow them in.
  it('retires an outstanding toast when the user reaches the covers section', async () => {
    const { router } = await renderToaster()
    act(() => maybeToastCoverReady(READY_COVER))
    expect(await screen.findByText('Your cover is ready')).toBeInTheDocument()

    await act(async () => {
      await router.navigate({ to: '/covers' })
    })

    await waitFor(() => expect(screen.queryByText('Your cover is ready')).not.toBeInTheDocument())
  })

  // A reconnecting stream can replay a ready snapshot; the user finished one
  // generation, not two.
  it('fires once per cover', async () => {
    await renderToaster()
    act(() => {
      maybeToastCoverReady(READY_COVER)
      maybeToastCoverReady(READY_COVER)
    })

    expect(await screen.findAllByText('Your cover is ready')).toHaveLength(1)
  })
})
