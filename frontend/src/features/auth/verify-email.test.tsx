import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'

import { VerifyEmail } from '@/features/auth/verify-email'
import { renderWithProviders } from '@/test/render'

// The component renders a decided outcome and nothing else. There is no pending
// branch on purpose: the route loader spends the single-use token before this
// mounts. The previous version consumed it from an effect, and the ref guarding
// React's double-invoked effects outlived the mutation state that a remount
// resets — leaving the page on "One moment…" forever with the address already
// verified on the server.
describe('VerifyEmail', () => {
  it('confirms a verified address and offers sign-in', async () => {
    renderWithProviders(<VerifyEmail result={{ status: 'verified', message: 'Email verified.' }} />)

    expect(await screen.findByText('Email verified')).toBeInTheDocument()
    expect(screen.getByText('Email verified.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Sign in' })).toBeInTheDocument()
  })

  it('explains a spent or expired link', async () => {
    renderWithProviders(<VerifyEmail result={{ status: 'failed', message: 'Link is invalid or expired' }} />)

    expect(await screen.findByText('Could not verify')).toBeInTheDocument()
    expect(screen.getByText('Link is invalid or expired')).toBeInTheDocument()
    expect(screen.getByText(/only be used once/i)).toBeInTheDocument()
  })

  it('says so when the link carries no token at all', async () => {
    renderWithProviders(<VerifyEmail result={{ status: 'missing' }} />)

    expect(await screen.findByText('Link is invalid')).toBeInTheDocument()
    expect(screen.getByText(/missing its token/i)).toBeInTheDocument()
  })
})
