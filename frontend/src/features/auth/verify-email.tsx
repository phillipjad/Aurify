import { Link } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'

import { AuthShell, FormError, FormSuccess } from './auth-shell'

/** Outcome of spending the verification token, decided in the route loader. */
export type VerifyEmailResult =
  | { status: 'missing' }
  | { status: 'verified'; message: string }
  | { status: 'failed'; message: string }

/**
 * Reports the outcome of a verification link.
 *
 * It renders a decided result and never fetches: the token is single-use, so
 * the route loader spends it exactly once (see routes/verify-email.tsx). There
 * is deliberately no pending branch here — reaching this component means the
 * loader has already settled.
 */
export function VerifyEmail({ result }: { result: VerifyEmailResult }) {
  if (result.status === 'missing') {
    return (
      <AuthShell title="Link is invalid">
        <div className="space-y-4">
          <FormError message="This verification link is missing its token." />
          <Link to="/sign-in" className="block">
            <Button variant="outline" className="w-full">
              Back to sign in
            </Button>
          </Link>
        </div>
      </AuthShell>
    )
  }

  if (result.status === 'verified') {
    return (
      <AuthShell title="Email verified">
        <div className="space-y-4">
          <FormSuccess message={result.message} />
          <Link to="/sign-in" className="block">
            <Button className="w-full">Sign in</Button>
          </Link>
        </div>
      </AuthShell>
    )
  }

  return (
    <AuthShell title="Could not verify">
      <div className="space-y-4">
        <FormError message={result.message} />
        <p className="text-pretty text-sm text-muted-foreground">
          Verification links expire after 24 hours, and each one can only be used once. Signing up again with the same
          address will send a fresh link.
        </p>
        <Link to="/sign-in" className="block">
          <Button variant="outline" className="w-full">
            Back to sign in
          </Button>
        </Link>
      </div>
    </AuthShell>
  )
}
