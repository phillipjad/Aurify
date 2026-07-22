import { createFileRoute } from '@tanstack/react-router'

import { AuthStub } from '@/features/auth/auth-stub'

export const Route = createFileRoute('/sign-in')({
  component: () => <AuthStub mode="sign-in" />,
})
