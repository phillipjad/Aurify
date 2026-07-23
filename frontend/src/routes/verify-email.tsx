import { createFileRoute } from '@tanstack/react-router'

import { VerifyEmail } from '@/features/auth/verify-email'

export const Route = createFileRoute('/verify-email')({
  validateSearch: (search: Record<string, unknown>) => ({
    token: typeof search.token === 'string' ? search.token : '',
  }),
  component: VerifyEmailPage,
})

function VerifyEmailPage() {
  const { token } = Route.useSearch()
  return <VerifyEmail token={token} />
}
