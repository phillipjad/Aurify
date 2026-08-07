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
// a screen reader hears status changes rather than four synonyms a minute.
it('announces the stage, not the synonym', () => {
  vi.setSystemTime(4000)
  const { container } = render(<StatusBadge status="analyzing" />)

  expect(screen.getByText('Judging')).toHaveAttribute('aria-hidden', 'true')
  expect(container.querySelector('.sr-only')).toHaveTextContent('Listening')
})

// Every other status is one node: there is nothing to cycle, so there is nothing
// to hide from the announcement either.
it('leaves a settled status alone', () => {
  render(<StatusBadge status="ready" />)
  expect(screen.getByText('Ready')).not.toHaveAttribute('aria-hidden')
})
