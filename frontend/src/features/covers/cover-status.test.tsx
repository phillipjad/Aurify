import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'

import { StatusBadge } from '@/features/covers/cover-status'

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }))
afterEach(() => vi.useRealTimers())

// The analyzing stage is a rate-limited feature lookup, a minute or two at the
// current cap. One unchanging word for that long reads as a hung generation.
it('cycles the word while analyzing', () => {
  // Off "Listening", which the sr-only label also carries.
  vi.setSystemTime(4000)
  render(<StatusBadge status="analyzing" />)
  expect(screen.getByText('Judging')).toBeInTheDocument()

  act(() => void vi.advanceTimersByTime(4000))
  expect(screen.getByText('Enjoying')).toBeInTheDocument()
  expect(screen.queryByText('Judging')).not.toBeInTheDocument()
})

// The cycle is a sign of life, not progress. aria-live carries the real stage so
// a screen reader hears the stage rather than a synonym every four seconds.
it('announces the stage, not the synonym', () => {
  vi.setSystemTime(4000)
  const { container } = render(<StatusBadge status="analyzing" />)

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
  vi.setSystemTime(4000)
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
  vi.setSystemTime(4000)
  render(<StatusBadge status="generating" />)
  expect(screen.getByText('Sketching')).toBeInTheDocument()

  act(() => void vi.advanceTimersByTime(4000))
  expect(screen.getByText('Mixing')).toBeInTheDocument()
})

// A verb holds for one full ellipsis before handing over, and the dots live in
// a fixed-width box so the verb never slides left as they appear.
it('grows an ellipsis before changing the verb', () => {
  vi.setSystemTime(4000)
  const { container } = render(<StatusBadge status="analyzing" />)
  const shown = () => container.querySelector('[aria-hidden="true"]')?.textContent

  expect(shown()).toBe('Judging')
  act(() => void vi.advanceTimersByTime(1000))
  expect(shown()).toBe('Judging.')
  act(() => void vi.advanceTimersByTime(1000))
  expect(shown()).toBe('Judging..')
  act(() => void vi.advanceTimersByTime(1000))
  expect(shown()).toBe('Judging...')

  // Fourth second hands over to the next verb, with the ellipsis reset.
  act(() => void vi.advanceTimersByTime(1000))
  expect(shown()).toBe('Enjoying')
})

// Queued has nothing to cycle through, but it is still waiting, so it still
// earns the ellipsis. Only settled states sit completely still.
it('animates a queued cover without cycling it', () => {
  vi.setSystemTime(4000)
  const { container } = render(<StatusBadge status="pending" />)
  const shown = () => container.querySelector('[aria-hidden="true"]')?.textContent

  expect(shown()).toBe('Queued')
  act(() => void vi.advanceTimersByTime(2000))
  expect(shown()).toBe('Queued..')
  act(() => void vi.advanceTimersByTime(2000))
  expect(shown()).toBe('Queued')
})

// Every other status is one node: there is nothing to cycle, so there is nothing
// to hide from the announcement either.
it('leaves a settled status alone', () => {
  render(<StatusBadge status="ready" />)
  expect(screen.getByText('Ready')).not.toHaveAttribute('aria-hidden')
})
