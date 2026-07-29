import { createFileRoute } from '@tanstack/react-router'

import { VerifyEmail } from '@/features/auth/verify-email'
import { verifyEmail } from '@/lib/api/auth'
import { isApiError } from '@/lib/api/client'

/**
 * The verification token is spent here, in the loader.
 *
 * It has to happen exactly once — the token is single-use, and a second attempt
 * reports a failure for a verification that actually succeeded. A loader runs
 * once per navigation and its result is held on the route match, so the outcome
 * survives a remount. Doing it from an effect did not: the ref guarding the
 * double-invoke outlived the mutation state, which is reset on remount, and the
 * page sat on "One moment…" forever with the account already verified.
 */
export const Route = createFileRoute('/verify-email')({
  validateSearch: (search: Record<string, unknown>) => ({
    token: typeof search.token === 'string' ? search.token : '',
  }),
  loaderDeps: ({ search: { token } }) => ({ token }),
  loader: async ({ deps: { token } }) => {
    if (!token) return { status: 'missing' as const }
    try {
      const { message } = await verifyEmail(token)
      return { status: 'verified' as const, message }
    } catch (err) {
      // Returned rather than thrown: a spent or expired link is an ordinary
      // outcome the page explains, not a crash for the error boundary.
      return {
        status: 'failed' as const,
        message: isApiError(err) ? err.message : 'Something went wrong. Try again.',
      }
    }
  },
  component: VerifyEmailPage,
})

function VerifyEmailPage() {
  return <VerifyEmail result={Route.useLoaderData()} />
}
