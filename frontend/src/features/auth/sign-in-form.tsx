import type { FormEvent } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { useSignIn } from '@/lib/api/auth'
import { isApiError } from '@/lib/api/client'

import { AuthShell, Field, FormError, textField } from './auth-shell'
import { AuthDivider, GoogleButton } from './google-button'

/**
 * Messages for the `?error=` codes the federated callback redirects back with.
 *
 * The callback is a top-level navigation, so it cannot return a JSON problem
 * document — it carries a short code instead, and the wording lives here.
 */
const CALLBACK_ERRORS: Record<string, string> = {
  cancelled: 'Google sign-in was cancelled.',
  expired: 'That sign-in attempt expired. Try again.',
  state: 'That sign-in attempt could not be verified. Try again.',
  exchange: 'We could not complete sign-in with Google. Try again.',
  blocked:
    'Too many failed attempts from this location. This block is permanent and only we can lift it — please contact support.',
  verify_existing_account:
    'An Aurify account already uses this address. Verify it from the link we emailed you, then Google sign-in will link to it.',
  google_email_unverified: 'Google has not verified that address, so it cannot be used to sign in.',
  signin_failed: 'Something went wrong signing you in. Try again.',
}

export function SignInForm({ redirectTo, callbackError }: { redirectTo?: string; callbackError?: string }) {
  const navigate = useNavigate()
  const signIn = useSignIn()

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    signIn.mutate(
      {
        email: textField(form, 'email'),
        password: textField(form, 'password'),
      },
      {
        onSuccess: () => {
          void navigate({ to: redirectTo ?? '/playlists' })
        },
      },
    )
  }

  const error = signIn.isError
    ? isApiError(signIn.error)
      ? signIn.error.message
      : 'Something went wrong. Try again.'
    : callbackError
      ? (CALLBACK_ERRORS[callbackError] ?? 'Sign-in failed. Try again.')
      : undefined

  return (
    <AuthShell
      title="Sign in"
      description="Welcome back. Sign in to generate and keep your covers."
      footer={
        <>
          No account?{' '}
          <Link to="/sign-up" className="font-medium text-foreground underline underline-offset-4">
            Create one
          </Link>
        </>
      }
    >
      <div className="space-y-4">
        <GoogleButton redirectTo={redirectTo} />
        <AuthDivider />

        <form onSubmit={handleSubmit} className="space-y-4" noValidate>
          <FormError message={error} />

          <Field id="email" label="Email" type="email" autoComplete="email" required />
          <Field id="password" label="Password" type="password" autoComplete="current-password" required />

          <Button type="submit" className="w-full" disabled={signIn.isPending}>
            {signIn.isPending ? 'Signing in…' : 'Sign in'}
          </Button>

          <p className="text-center text-sm">
            <Link to="/forgot-password" className="text-muted-foreground underline underline-offset-4">
              Forgot your password?
            </Link>
          </p>
        </form>
      </div>
    </AuthShell>
  )
}
