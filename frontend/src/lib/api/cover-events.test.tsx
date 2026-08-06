import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { screen, waitFor } from '@testing-library/react'

import { CoverGenerationWatcher } from '@/components/cover-generation-watcher'
import { Toaster } from '@/components/ui/sonner'
import { apiFetch } from '@/lib/api/client'
import { toastedCovers } from '@/lib/cover-toasts'
import { renderWithProviders } from '@/test/render'

vi.mock('@/lib/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockFetch = apiFetch as unknown as Mock

/** Records every EventSource the app opens, and lets a test push events back. */
class FakeEventSource {
  static opened: FakeEventSource[] = []
  listeners = new Map<string, (event: MessageEvent<string>) => void>()
  closed = false

  constructor(readonly url: string) {
    FakeEventSource.opened.push(this)
  }

  addEventListener(type: string, fn: (event: MessageEvent<string>) => void) {
    this.listeners.set(type, fn)
  }
  removeEventListener() {}
  close() {
    this.closed = true
  }

  emit(cover: unknown) {
    this.listeners.get('cover')?.({ data: JSON.stringify(cover) } as MessageEvent<string>)
  }
}

const RUNNING_COVER = {
  id: 'cover-running',
  status: 'generating',
  platform: 'spotify',
  playlistId: 'sp1',
  playlistName: 'Deep Focus',
  createdAt: '2026-08-06T00:00:00Z',
}

beforeEach(() => {
  mockFetch.mockReset()
  FakeEventSource.opened = []
  toastedCovers.clear()
  vi.stubGlobal('EventSource', FakeEventSource)
})

// A generation outlives the tab that started it, but the mutation cache does
// not. Reloading mid-run used to leave nothing watching the cover, so the ready
// event — and the toast that depends on it — never arrived.
describe('resuming generations after a reload', () => {
  it('watches covers that were already running when the page loaded', async () => {
    mockFetch.mockImplementation((path: string) => {
      if (path.startsWith('/auth/session')) return Promise.resolve({ user: { id: 'u1' } })
      if (path.startsWith('/covers?')) return Promise.resolve([RUNNING_COVER])
      return Promise.resolve(RUNNING_COVER)
    })

    renderWithProviders(
      <>
        <CoverGenerationWatcher />
        <Toaster />
      </>,
      { path: '/playlists' },
    )

    await waitFor(() => expect(FakeEventSource.opened).toHaveLength(1))
    expect(FakeEventSource.opened[0]?.url).toContain('cover-running/events')

    // The stream finishes the job the reload interrupted, and the toast fires.
    FakeEventSource.opened[0]?.emit({ ...RUNNING_COVER, status: 'ready' })
    expect(await screen.findByText('Your cover is ready')).toBeInTheDocument()
    expect(screen.getByText('Deep Focus')).toBeInTheDocument()
  })

  it('opens nothing for a cover that had already finished', async () => {
    mockFetch.mockImplementation((path: string) => {
      if (path.startsWith('/auth/session')) return Promise.resolve({ user: { id: 'u1' } })
      if (path.startsWith('/covers?')) return Promise.resolve([{ ...RUNNING_COVER, status: 'ready' }])
      return Promise.resolve({ ...RUNNING_COVER, status: 'ready' })
    })

    renderWithProviders(<CoverGenerationWatcher />, { path: '/playlists' })

    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('/covers?')))
    expect(FakeEventSource.opened).toHaveLength(0)
  })

  // The covers list is authenticated; asking anonymously is a guaranteed 401 in
  // front of every visit.
  it('asks for nothing when nobody is signed in', async () => {
    mockFetch.mockImplementation((path: string) => {
      if (path.startsWith('/auth/session')) return Promise.resolve(null)
      return Promise.reject(new Error(`unexpected request: ${path}`))
    })

    renderWithProviders(<CoverGenerationWatcher />, { path: '/playlists' })

    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith('/auth/session'))
    expect(mockFetch).not.toHaveBeenCalledWith(expect.stringContaining('/covers?'))
    expect(FakeEventSource.opened).toHaveLength(0)
  })
})
