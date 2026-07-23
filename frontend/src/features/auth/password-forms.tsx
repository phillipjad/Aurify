import type { FormEvent } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { useForgotPassword, useResetPassword } from '@/lib/api/auth'
import { isApiError } from '@/lib/api/client'

import { AuthShell, Field, FormError, FormSuccess, textField } from './auth-shell'

function errorMessage(isError: boolean, err: unknown): string | undefined {
  if (!isError) return undefined
  return isApiError(err) ? err.message : 'Something went wrong. Try again.'
}

/**
 * Request a reset link.
 *
 * The API answers identically whether or not the address is registered, and the
 * UI must not undo that: showing "no such account" here would turn this into a
 * cheap way to enumerate who has an account.
 */
export function ForgotPasswordForm() {
  const forgot = useForgotPassword()

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    forgot.mutate({ email: textField(form, 'email') })
  }

  if (forgot.isSuccess) {
    return (
      <AuthShell title="Check your inbox">
        <div className="space-y-4">
          <FormSuccess message={forgot.data.message} />
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
    <AuthShell
      title="Reset your password"
      description="Enter your address and we will send you a link."
      footer={
        <Link to="/sign-in" className="font-medium text-foreground underline underline-offset-4">
          Back to sign in
        </Link>
      }
    >
      <form onSubmit={handleSubmit} className="space-y-4" noValidate>
        <FormError message={errorMessage(forgot.isError, forgot.error)} />
        <Field id="email" label="Email" type="email" autoComplete="email" required />
        <Button type="submit" className="w-full" disabled={forgot.isPending}>
          {forgot.isPending ? 'Sending…' : 'Send reset link'}
        </Button>
      </form>
    </AuthShell>
  )
}

/**
 * Set a new password using the token from the emailed link.
 *
 * Completing this revokes every session the account had, so there is no session
 * to land in afterwards — the user is sent to sign in with the new password.
 */
export function ResetPasswordForm({ token }: { token: string }) {
  const navigate = useNavigate()
  const reset = useResetPassword()

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    reset.mutate({ token, newPassword: textField(form, 'newPassword') })
  }

  if (!token) {
    return (
      <AuthShell title="Link is invalid">
        <div className="space-y-4">
          <FormError message="This reset link is missing its token. Request a new one." />
          <Link to="/forgot-password" className="block">
            <Button variant="outline" className="w-full">
              Request a new link
            </Button>
          </Link>
        </div>
      </AuthShell>
    )
  }

  if (reset.isSuccess) {
    return (
      <AuthShell title="Password updated">
        <div className="space-y-4">
          <FormSuccess message={reset.data.message} />
          <p className="text-pretty text-sm text-muted-foreground">
            Every device signed in to this account was signed out, including this one.
          </p>
          <Button
            className="w-full"
            onClick={() => {
              void navigate({ to: '/sign-in' })
            }}
          >
            Sign in
          </Button>
        </div>
      </AuthShell>
    )
  }

  return (
    <AuthShell title="Choose a new password">
      <form onSubmit={handleSubmit} className="space-y-4" noValidate>
        <FormError message={errorMessage(reset.isError, reset.error)} />
        <Field
          id="newPassword"
          label="New password"
          type="password"
          autoComplete="new-password"
          required
          hint="At least 8 characters."
        />
        <Button type="submit" className="w-full" disabled={reset.isPending}>
          {reset.isPending ? 'Updating…' : 'Update password'}
        </Button>
      </form>
    </AuthShell>
  )
}
