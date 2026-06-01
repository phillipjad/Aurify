import { beforeEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ThemeProvider } from '@/components/theme-provider'
import { ThemeToggle } from '@/components/theme-toggle'

function renderToggle() {
  return render(
    <ThemeProvider>
      <ThemeToggle />
    </ThemeProvider>,
  )
}

describe('ThemeToggle', () => {
  beforeEach(() => {
    // ThemeProvider derives its initial theme from <html>; start each test light.
    document.documentElement.classList.remove('dark')
  })

  it('starts on the light theme and offers to switch to dark', () => {
    renderToggle()
    expect(screen.getByRole('button', { name: 'Switch to dark theme' })).toBeInTheDocument()
    expect(document.documentElement).not.toHaveClass('dark')
  })

  it('toggles the theme and the <html> dark class on click', async () => {
    renderToggle()

    await userEvent.click(screen.getByRole('button', { name: 'Switch to dark theme' }))
    expect(document.documentElement).toHaveClass('dark')
    expect(screen.getByRole('button', { name: 'Switch to light theme' })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Switch to light theme' }))
    expect(document.documentElement).not.toHaveClass('dark')
  })
})
