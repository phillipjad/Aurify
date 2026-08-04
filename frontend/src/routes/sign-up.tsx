import { createFileRoute } from '@tanstack/react-router'

import { SignUpForm } from '@/features/auth/sign-up-form'
import { redirectIfSignedIn } from '@/lib/auth-guard'

export const Route = createFileRoute('/sign-up')({
  // Creating a second account while signed in is not a thing this app does.
  beforeLoad: ({ context }) => redirectIfSignedIn(context.queryClient),
  component: () => <SignUpForm />,
})
