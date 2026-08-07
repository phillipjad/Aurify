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

  expect(screen.getByText('Judging')).toHaveAttribute('aria-hidden', 'true')
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

// Every other status is one node: there is nothing to cycle, so there is nothing
// to hide from the announcement either.
it('leaves a settled status alone', () => {
  render(<StatusBadge status="ready" />)
  expect(screen.getByText('Ready')).not.toHaveAttribute('aria-hidden')
})
