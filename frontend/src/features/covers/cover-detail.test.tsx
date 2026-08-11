import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CoverDetail } from '@/features/covers/cover-detail'
import { ApiError, apiFetch } from '@/lib/api/client'
import { renderWithProviders } from '@/test/render'

vi.mock('@/lib/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockFetch = apiFetch as unknown as Mock

const COVER = {
  id: 'c1',
  status: 'ready',
  platform: 'spotify',
  playlistId: 'pl1',
  playlistName: 'Morning Coffee',
  imageUrl: 'https://cdn.test/c1.png',
  runCount: 1,
  createdAt: '2026-01-02T00:00:00Z',
  updatedAt: '2026-01-02T00:00:00Z',
  palette: [
    { dimension: 'organic', hexColor: '#7fb069', weight: 0.42 },
    { dimension: 'melancholic', hexColor: '#5c4d7d', weight: 0.31 },
    { dimension: 'introspective', hexColor: '#4f86c6', weight: 0.27 },
  ],
}

// Two successful runs and one that produced nothing, which is what the failed
// filter is about. The numbers are the server's, and #3 being the failed one is
// the point: a number is a run's identity, not its place in the list.
const READY_RUN = {
  id: 'r2',
  number: 2,
  status: 'ready',
  imageUrl: 'https://cdn.test/r2.png',
  completedAt: '2026-01-02T00:00:00Z',
  palette: [{ dimension: 'organic', hexColor: '#7fb069', weight: 1 }],
}
const OLDER_RUN = {
  id: 'r1',
  number: 1,
  status: 'ready',
  imageUrl: 'https://cdn.test/r1.png',
  completedAt: '2026-01-01T00:00:00Z',
  palette: [{ dimension: 'melancholic', hexColor: '#5c4d7d', weight: 1 }],
}
const FAILED_RUN = {
  id: 'r3',
  number: 3,
  status: 'failed',
  error: 'imagegen refused the prompt',
  completedAt: '2026-01-03T00:00:00Z',
}

const WITH_HISTORY = { ...COVER, runCount: 2, revisions: [READY_RUN, OLDER_RUN] }

// Route GET /covers/:id to the cover; POST and DELETE resolve like the API.
function routeApi(cover: unknown = COVER) {
  mockFetch.mockImplementation((_path: string, init?: RequestInit) => {
    if (init?.method === 'DELETE') return Promise.resolve(undefined)
    if (init?.method === 'POST') return Promise.resolve({ ...COVER, id: 'c2', status: 'pending' })
    return Promise.resolve(cover)
  })
}

const artwork = () => screen.getByAltText('Cover generated for Morning Coffee')
const revisionList = () => screen.getByRole('heading', { name: /revisions/i }).closest('section') as HTMLElement
// "Current" and "Revision #1" appear in two places by design: the stepper says
// which run is on screen, the list badges which one is the cover's current
// artwork. Tests say which they mean.
const stepper = () => screen.getByRole('status')

beforeEach(() => {
  mockFetch.mockReset()
})

describe('CoverDetail', () => {
  it('renders the full palette breakdown', async () => {
    routeApi()
    renderWithProviders(<CoverDetail coverId="c1" />)

    expect(await screen.findByRole('heading', { level: 1, name: 'Morning Coffee' })).toBeInTheDocument()
    // Every dimension is shown, not just the top few.
    for (const dim of ['organic', 'melancholic', 'introspective']) {
      expect(screen.getByText(dim)).toBeInTheDocument()
    }
    expect(screen.getByText('42%')).toBeInTheDocument()
  })

  // Deleting the cover now takes every run with it, so the confirmation has to
  // say so rather than sounding like it removes one picture.
  it('confirms that a delete takes every run with it, then calls DELETE', async () => {
    routeApi(WITH_HISTORY)
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    // First click only reveals the confirmation — nothing is deleted yet.
    await userEvent.click(screen.getByRole('button', { name: /^delete$/i }))
    expect(screen.getByText(/delete this cover and all 2 versions\?/i)).toBeInTheDocument()
    expect(mockFetch).not.toHaveBeenCalledWith('/covers/c1', { method: 'DELETE' })

    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(mockFetch).toHaveBeenCalledWith('/covers/c1', { method: 'DELETE' })
  })

  it('can back out of a delete', async () => {
    routeApi()
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    await userEvent.click(screen.getByRole('button', { name: /^delete$/i }))
    await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(screen.queryByText(/delete this cover/i)).not.toBeInTheDocument()
    expect(mockFetch).not.toHaveBeenCalledWith('/covers/c1', { method: 'DELETE' })
  })

  it('regenerates by generating a fresh cover for the same playlist', async () => {
    routeApi()
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    await userEvent.click(screen.getByRole('button', { name: /regenerate/i }))
    expect(mockFetch).toHaveBeenCalledWith('/covers', {
      method: 'POST',
      body: JSON.stringify({ platform: 'spotify', playlistId: 'pl1' }),
    })
  })

  // One run at a time per playlist: the API refuses a second with a 409, and
  // offering a control that can only produce that error is a worse way to say so.
  it('will not start a second run while one is in flight', async () => {
    routeApi({ ...COVER, status: 'analyzing' })
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    expect(screen.getByRole('button', { name: /regenerate/i })).toBeDisabled()

    await userEvent.click(screen.getByRole('button', { name: /regenerate/i }))
    expect(mockFetch).not.toHaveBeenCalledWith('/covers', expect.objectContaining({ method: 'POST' }))
  })

  it('explains a deleted / missing cover instead of erroring blank', async () => {
    mockFetch.mockRejectedValue(new ApiError(404, 'not found', 'Not Found'))
    renderWithProviders(<CoverDetail coverId="gone" />)

    expect(await screen.findByText(/doesn.t exist/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /all covers/i })).toHaveAttribute('href', '/covers')
  })

  it('offers a download link for a ready cover', async () => {
    routeApi()
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    const download = screen.getByRole('link', { name: /download/i })
    expect(download).toHaveAttribute('href', 'https://cdn.test/c1.png')
    expect(download).toHaveAttribute('download')
  })

  // Earlier artwork staying reachable is the reason runs are kept at all rather
  // than being overwritten.
  it('lists every earlier run with its own thumbnail and number', async () => {
    routeApi(WITH_HISTORY)
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    const list = revisionList()
    expect(within(list).getAllByRole('listitem')).toHaveLength(2)
    expect(within(list).getByText('Revision #2')).toBeInTheDocument()
    expect(within(list).getByText('Revision #1')).toBeInTheDocument()
    // Only the newest successful run is the current artwork.
    expect(within(list).getAllByText('Current')).toHaveLength(1)

    const thumbnails = within(list)
      .getAllByRole('presentation', { hidden: true })
      .map((img) => img.getAttribute('src'))
    expect(thumbnails).toEqual(['https://cdn.test/r2.png', 'https://cdn.test/r1.png'])
  })

  // A cover with nothing finished yet has no history to show, and a heading over
  // an empty list is noise on the page that matters least.
  it('leaves the history out entirely until a run has finished', async () => {
    routeApi({ ...COVER, status: 'analyzing', revisions: undefined })
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    expect(screen.queryByRole('heading', { name: /revisions/i })).not.toBeInTheDocument()
  })

  // Failed runs are recorded because their error is the only evidence explaining
  // an image-provider refusal, but they are not what the history is for. The
  // server decides, so the toggle has to reach it.
  it('asks the server for failed runs only when they are turned on', async () => {
    mockFetch.mockImplementation((path: string) =>
      Promise.resolve(
        path.includes('allow_failed_revisions')
          ? { ...WITH_HISTORY, revisions: [FAILED_RUN, READY_RUN, OLDER_RUN] }
          : WITH_HISTORY,
      ),
    )
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })
    expect(screen.queryByText('imagegen refused the prompt')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /show failed runs/i }))

    expect(await screen.findByText('imagegen refused the prompt')).toBeInTheDocument()
    expect(mockFetch).toHaveBeenCalledWith('/covers/c1?allow_failed_revisions=true')
  })

  it('confirms before deleting a single run, then calls its own endpoint', async () => {
    routeApi(WITH_HISTORY)
    renderWithProviders(<CoverDetail coverId="c1" />)
    await screen.findByRole('heading', { level: 1 })

    const older = within(revisionList()).getAllByRole('listitem')[1] as HTMLElement

    await userEvent.click(within(older).getByRole('button', { name: /delete revision 1/i }))
    expect(mockFetch).not.toHaveBeenCalledWith('/covers/c1/revisions/r1', { method: 'DELETE' })

    await userEvent.click(within(older).getByRole('button', { name: 'Delete' }))
    // The cover survives: only the run is removed.
    expect(mockFetch).toHaveBeenCalledWith('/covers/c1/revisions/r1', { method: 'DELETE' })
    expect(mockFetch).not.toHaveBeenCalledWith('/covers/c1', { method: 'DELETE' })
  })

  describe('stepping through revisions', () => {
    it('walks back to the previous run, swapping the artwork and the palette', async () => {
      routeApi(WITH_HISTORY)
      renderWithProviders(<CoverDetail coverId="c1" />)
      await screen.findByRole('heading', { level: 1 })

      expect(within(stepper()).getByText('Current')).toBeInTheDocument()
      expect(artwork()).toHaveAttribute('src', 'https://cdn.test/r2.png')

      await userEvent.click(screen.getByRole('button', { name: /older revision/i }))

      expect(artwork()).toHaveAttribute('src', 'https://cdn.test/r1.png')
      // The palette belongs to the run being viewed, not to the cover.
      expect(screen.getByText('100%')).toBeInTheDocument()

      // The label stays put so the row does not reflow on every step; the run
      // it saves is carried by the href and the filename instead.
      const download = screen.getByRole('link', { name: 'Download' })
      expect(download).toHaveAttribute('href', 'https://cdn.test/r1.png')
      expect(download).toHaveAttribute('download', 'aurify-Morning Coffee-1.png')
    })

    // Which revision you are looking at is view state, and view state that
    // survives a refresh or a shared link has to be in the URL.
    it('records the revision in the URL, and clears it again on the current one', async () => {
      routeApi(WITH_HISTORY)
      const { router } = renderWithProviders(<CoverDetail coverId="c1" />)
      await screen.findByRole('heading', { level: 1 })

      await userEvent.click(screen.getByRole('button', { name: /older revision/i }))
      expect(router.state.location.search).toEqual({ rev: 1 })

      await userEvent.click(screen.getByRole('button', { name: /newer revision/i }))
      // Back on the default view, so nothing is pinned: the next generation
      // would otherwise leave the URL pointing at an older run.
      expect(router.state.location.search).toEqual({})
    })

    it('opens on the revision named by the URL', async () => {
      routeApi(WITH_HISTORY)
      renderWithProviders(<CoverDetail coverId="c1" />, { path: '/covers/c1?rev=1' })
      await screen.findByRole('heading', { level: 1 })

      expect(artwork()).toHaveAttribute('src', 'https://cdn.test/r1.png')
      expect(within(stepper()).getByText('Revision #1')).toBeInTheDocument()
    })

    // A link outliving the run it names is the ordinary case once a revision is
    // deleted, and it should still open the cover.
    it('falls back to the current artwork when the URL names a run that is gone', async () => {
      routeApi(WITH_HISTORY)
      renderWithProviders(<CoverDetail coverId="c1" />, { path: '/covers/c1?rev=99' })
      await screen.findByRole('heading', { level: 1 })

      expect(artwork()).toHaveAttribute('src', 'https://cdn.test/r2.png')
      expect(within(stepper()).getByText('Current')).toBeInTheDocument()
    })

    // A URL is typed by anyone, and a value that is not a revision number at all
    // must not blank the page.
    it('ignores a nonsense revision in the URL', async () => {
      routeApi(WITH_HISTORY)
      renderWithProviders(<CoverDetail coverId="c1" />, { path: '/covers/c1?rev=not-a-number' })
      await screen.findByRole('heading', { level: 1 })

      expect(artwork()).toHaveAttribute('src', 'https://cdn.test/r2.png')
    })

    // The chevrons only move between successful runs, so with one there is
    // nowhere to go and the control would be dead furniture under the artwork.
    it('is not offered until there is more than one successful run', async () => {
      routeApi({ ...COVER, revisions: [READY_RUN] })
      renderWithProviders(<CoverDetail coverId="c1" />)
      await screen.findByRole('heading', { level: 1 })

      expect(screen.queryByRole('button', { name: /older revision/i })).not.toBeInTheDocument()
    })

    // A failed run has no artwork, so it is listed but not selectable: it is not
    // a control at all, which keeps its error message out of an element that
    // announces itself as unavailable.
    it('lists a failed run without making it a control', async () => {
      mockFetch.mockImplementation(() =>
        Promise.resolve({ ...WITH_HISTORY, revisions: [FAILED_RUN, READY_RUN, OLDER_RUN] }),
      )
      renderWithProviders(<CoverDetail coverId="c1" />)
      await screen.findByRole('heading', { level: 1 })
      await userEvent.click(screen.getByRole('button', { name: /show failed runs/i }))

      const failedRow = (await within(revisionList()).findByText('Revision #3')).closest('li') as HTMLElement
      // Nothing in the row selects it: the only controls explain it or delete it.
      const names = within(failedRow)
        .getAllByRole('button')
        .map((b) => b.getAttribute('aria-label') ?? b.textContent?.trim())
      expect(names).toEqual([
        expect.stringMatching(/why revision 3 failed/i),
        expect.stringMatching(/delete revision 3/i),
      ])
    })

    // The error used to print into the row, where its length set the row's
    // shape. A failed run now shows the same two facts as any other, with the
    // error behind an info button.
    //
    // This jsdom implements no part of the Popover API: showPopover is absent
    // and an invoker click does nothing, so opening it cannot be exercised here.
    // What is asserted is the wiring and the row's shape; opening, dismissing
    // and the top layer are verified in a real browser.
    it('keeps a failed run’s error out of the row, behind its info button', async () => {
      mockFetch.mockImplementation(() =>
        Promise.resolve({ ...WITH_HISTORY, revisions: [FAILED_RUN, READY_RUN, OLDER_RUN] }),
      )
      renderWithProviders(<CoverDetail coverId="c1" />)
      await screen.findByRole('heading', { level: 1 })
      await userEvent.click(screen.getByRole('button', { name: /show failed runs/i }))

      const failedRow = (await within(revisionList()).findByText('Revision #3')).closest('li') as HTMLElement

      // The same date every other run shows, rather than the error.
      expect(failedRow.querySelector('time')).toHaveAttribute('datetime', FAILED_RUN.completedAt)

      // The error is off the row, in the panel the button targets, and none of
      // it is visible until that button is used.
      const info = within(failedRow).getByRole('button', { name: /why revision 3 failed/i })
      const panel = document.getElementById(info.getAttribute('popovertarget') ?? '')
      expect(panel).toHaveTextContent('imagegen refused the prompt')
      expect(panel).not.toBeVisible()
    })
  })
})
