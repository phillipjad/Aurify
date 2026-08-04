import { createFileRoute } from '@tanstack/react-router'

import { SignInForm } from '@/features/auth/sign-in-form'
import { redirectIfSignedIn } from '@/lib/auth-guard'

interface SignInSearch {
  /** Where to go after signing in, set by the route guard. */
  redirect?: string
  /** Short failure code from the federated sign-in callback. */
  error?: string
}

export const Route = createFileRoute('/sign-in')({
  validateSearch: (search: Record<string, unknown>): SignInSearch => ({
    // Confined to in-app paths. The value reaches here from a redirect the user
    // may not have initiated, so anything that could name another origin is
    // dropped rather than followed after sign-in.
    redirect: typeof search.redirect === 'string' && isInternalPath(search.redirect) ? search.redirect : undefined,
    error: typeof search.error === 'string' ? search.error : undefined,
  }),
  // Already signed in? Then this page has nothing to offer: go on to wherever
  // the guard was sending them, or home.
  beforeLoad: ({ context, search }) => redirectIfSignedIn(context.queryClient, search.redirect),
  component: SignInPage,
})

function isInternalPath(value: string): boolean {
  return value.startsWith('/') && !value.startsWith('//')
}

function SignInPage() {
  const { redirect, error } = Route.useSearch()
  return <SignInForm redirectTo={redirect} callbackError={error} />
}
