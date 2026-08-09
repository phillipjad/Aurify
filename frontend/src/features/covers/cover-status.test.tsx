import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'

import { StatusBadge } from '@/features/covers/cover-status'

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }))
afterEach(() => vi.useRealTimers())

/** The visible word, read off the node the live region is told to ignore. */
function shown(container: HTMLElement) {
  return container.querySelector('[aria-hidden="true"]')?.textContent
}

// The analyzing stage is a rate-limited feature lookup, a minute or two at the
// current cap. One unchanging word for that long reads as a hung generation.
it('cycles the word while analyzing', () => {
  const { container } = render(<StatusBadge status="analyzing" />)
  expect(shown(container)).toBe('Listening')

  act(() => void vi.advanceTimersByTime(4000))
  expect(shown(container)).toBe('Judging')
})

// A stage anchored to the wall clock could open on any word, and crossing
// pending → analyzing → generating re-derived the index against a different
// verb list each time, so a fresh generation appeared to race through several
// words before settling. Every stage now opens on its own first verb.
it('opens every stage on its first verb', () => {
  for (const [status, first] of [
    ['pending', 'Queued'],
    ['analyzing', 'Listening'],
    ['generating', 'Painting'],
  ] as const) {
    // A clock deliberately off any four-second boundary: the anchor is the
    // moment the stage began, not the time of day.
    vi.setSystemTime(1_733_000_000_123)
    const { container, unmount } = render(<StatusBadge status={status} />)
    expect(shown(container)).toBe(first)
    unmount()
  }
})

// The cycle is a sign of life, not progress. aria-live carries the real stage so
// a screen reader hears the stage rather than a synonym every four seconds.
it('announces the stage, not the synonym', () => {
  const { container } = render(<StatusBadge status="analyzing" />)
  act(() => void vi.advanceTimersByTime(4000))

  // Hidden by an ancestor rather than on the word itself, so this asks the
  // question that matters: is the cycling word inside the live region's blind
  // spot, wherever the markup puts it.
  expect(screen.getByText('Judging').closest('[aria-hidden="true"]')).not.toBeNull()
  expect(container.querySelector('.sr-only')).toHaveTextContent('Listening')
})

// Silence for two minutes reads as a dead page to anyone who cannot see the
// words move, so the stage re-announces itself with the one honest number this
// screen has: how long it has been going.
it('re-announces with elapsed time, far more rarely than it cycles', () => {
  const { container } = render(<StatusBadge status="analyzing" />)
  const announced = () => container.querySelector('.sr-only')?.textContent

  act(() => void vi.advanceTimersByTime(8000))
  expect(announced()).toBe('Listening')

  act(() => void vi.advanceTimersByTime(30_000))
  expect(announced()).toBe('Listening, 30 seconds')

  act(() => void vi.advanceTimersByTime(60_000))
  expect(announced()).toBe('Listening, 1 minute 30 seconds')

  act(() => void vi.advanceTimersByTime(30_000))
  expect(announced()).toBe('Listening, 2 minutes')
})

// Painting is shorter than listening but still long enough to look stuck.
it('cycles the painting stage too', () => {
  const { container } = render(<StatusBadge status="generating" />)
  expect(shown(container)).toBe('Painting')

  act(() => void vi.advanceTimersByTime(4000))
  expect(shown(container)).toBe('Sketching')
})

// A verb holds for one full ellipsis before handing over, and the dots live in
// a fixed-width box so the verb never slides left as they appear.
it('grows an ellipsis before changing the verb', () => {
  const { container } = render(<StatusBadge status="analyzing" />)

  expect(shown(container)).toBe('Listening')
  act(() => void vi.advanceTimersByTime(1000))
  expect(shown(container)).toBe('Listening.')
  act(() => void vi.advanceTimersByTime(1000))
  expect(shown(container)).toBe('Listening..')
  act(() => void vi.advanceTimersByTime(1000))
  expect(shown(container)).toBe('Listening...')

  // Fourth second hands over to the next verb, with the ellipsis reset.
  act(() => void vi.advanceTimersByTime(1000))
  expect(shown(container)).toBe('Judging')
})

// Queued has nothing to cycle through, but it is still waiting, so it still
// earns the ellipsis. Only settled states sit completely still.
it('animates a queued cover without cycling it', () => {
  const { container } = render(<StatusBadge status="pending" />)

  expect(shown(container)).toBe('Queued')
  act(() => void vi.advanceTimersByTime(2000))
  expect(shown(container)).toBe('Queued..')
  act(() => void vi.advanceTimersByTime(2000))
  expect(shown(container)).toBe('Queued')
})

// Every other status is one node: there is nothing to cycle, so there is nothing
// to hide from the announcement either.
it('leaves a settled status alone', () => {
  render(<StatusBadge status="ready" />)
  expect(screen.getByText('Ready')).not.toHaveAttribute('aria-hidden')
})

// The gallery renders one badge per tile. Several generations at once meant
// several live regions talking over each other about covers they never named,
// while the ready toast was already announcing completion once, by name.
it('is not a live region unless asked', () => {
  const { container, rerender } = render(<StatusBadge status="analyzing" />)
  expect(container.querySelector('[aria-live]')).toBeNull()

  rerender(<StatusBadge status="analyzing" live />)
  expect(container.querySelector('[aria-live="polite"]')).not.toBeNull()
})
