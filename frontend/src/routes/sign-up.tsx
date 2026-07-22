import { createFileRoute } from '@tanstack/react-router'

import { AuthStub } from '@/features/auth/auth-stub'

export const Route = createFileRoute('/sign-up')({
  component: () => <AuthStub mode="sign-up" />,
})
