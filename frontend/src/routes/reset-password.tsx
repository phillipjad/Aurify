import { createFileRoute } from '@tanstack/react-router'

import { ResetPasswordForm } from '@/features/auth/password-forms'

export const Route = createFileRoute('/reset-password')({
  validateSearch: (search: Record<string, unknown>) => ({
    token: typeof search.token === 'string' ? search.token : '',
  }),
  component: ResetPasswordPage,
})

function ResetPasswordPage() {
  const { token } = Route.useSearch()
  return <ResetPasswordForm token={token} />
}
