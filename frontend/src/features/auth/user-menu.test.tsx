import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ThemeProvider } from '@/components/theme-provider'
import { UserMenu } from '@/features/auth/user-menu'
import { apiFetch } from '@/lib/api/client'
import { renderWithProviders } from '@/test/render'

vi.mock('@/lib/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/client')>()
  return { ...actual, apiFetch: vi.fn() }
})

const mockFetch = apiFetch as unknown as Mock

function renderMenu() {
  return renderWithProviders(
    <ThemeProvider>
      <UserMenu />
    </ThemeProvider>,
  )
}

beforeEach(() => {
  mockFetch.mockReset()
  // ThemeProvider derives its initial theme from <html>; start each test light.
  document.documentElement.classList.remove('dark')
})

describe('UserMenu', () => {
  it('offers sign-in, sign-up and the theme switch when signed out', async () => {
    mockFetch.mockResolvedValue(null)
    renderMenu()

    await userEvent.click(await screen.findByRole('button', { name: 'Account' }))

    expect(screen.getByRole('link', { name: 'Sign in' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Create account' })).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Dark mode' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument()
  })

  it('shows initials and sign-out when signed in', async () => {
    mockFetch.mockResolvedValue({ email: 'ada@example.com', displayName: 'Ada Lovelace' })
    renderMenu()

    const trigger = await screen.findByRole('button', { name: 'Account: Ada Lovelace' })
    expect(trigger).toHaveTextContent('AL')

    await userEvent.click(trigger)
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Sign in' })).not.toBeInTheDocument()
  })

  it('toggles the theme from the switch without closing the menu', async () => {
    mockFetch.mockResolvedValue(null)
    renderMenu()

    await userEvent.click(await screen.findByRole('button', { name: 'Account' }))
    const themeSwitch = screen.getByRole('switch', { name: 'Dark mode' })
    expect(themeSwitch).toHaveAttribute('aria-checked', 'false')

    await userEvent.click(themeSwitch)
    expect(document.documentElement).toHaveClass('dark')
    expect(screen.getByRole('switch', { name: 'Dark mode' })).toHaveAttribute('aria-checked', 'true')

    await userEvent.click(screen.getByRole('switch', { name: 'Dark mode' }))
    expect(document.documentElement).not.toHaveClass('dark')
  })

  it('closes on Escape and on an outside click', async () => {
    mockFetch.mockResolvedValue(null)
    renderMenu()

    const trigger = await screen.findByRole('button', { name: 'Account' })

    await userEvent.click(trigger)
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('link', { name: 'Sign in' })).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()

    await userEvent.click(trigger)
    await userEvent.click(document.body)
    expect(screen.queryByRole('link', { name: 'Sign in' })).not.toBeInTheDocument()
  })
})
