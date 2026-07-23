import { createFileRoute } from '@tanstack/react-router'

import { ForgotPasswordForm } from '@/features/auth/password-forms'

export const Route = createFileRoute('/forgot-password')({
  component: () => <ForgotPasswordForm />,
})
