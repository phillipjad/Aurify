import { useEffect, useRef } from 'react'
import { Link } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { useVerifyEmail } from '@/lib/api/auth'
import { isApiError } from '@/lib/api/client'

import { AuthShell, FormError, FormSuccess } from './auth-shell'

/**
 * Consumes the token from a verification link on mount.
 *
 * The token is single-use, so this fires exactly once — a second submission
 * would spend a token that is already gone and report a failure for a
 * verification that actually succeeded. The ref guards against React 19's
 * double-invoked effects in development doing precisely that.
 */
export function VerifyEmail({ token }: { token: string }) {
  const verify = useVerifyEmail()
  const attempted = useRef(false)
  const { mutate } = verify

  useEffect(() => {
    if (!token || attempted.current) return
    attempted.current = true
    mutate(token)
  }, [token, mutate])

  if (!token) {
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

  if (verify.isSuccess) {
    return (
      <AuthShell title="Email verified">
        <div className="space-y-4">
          <FormSuccess message={verify.data.message} />
          <Link to="/sign-in" className="block">
            <Button className="w-full">Sign in</Button>
          </Link>
        </div>
      </AuthShell>
    )
  }

  if (verify.isError) {
    return (
      <AuthShell title="Could not verify">
        <div className="space-y-4">
          <FormError message={isApiError(verify.error) ? verify.error.message : 'Something went wrong. Try again.'} />
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

  return (
    <AuthShell title="Verifying your email">
      <p role="status" className="text-sm text-muted-foreground">
        One moment…
      </p>
    </AuthShell>
  )
}
