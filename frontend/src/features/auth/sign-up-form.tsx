import type { FormEvent } from 'react'
import { Link } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { useSignUp } from '@/lib/api/auth'
import { isApiError } from '@/lib/api/client'

import { AuthShell, Field, FormError, FormSuccess, textField } from './auth-shell'
import { AuthDivider, GoogleButton } from './google-button'

/** Mirrors the API's minimum. Length is the only rule either side enforces. */
const MIN_PASSWORD_LENGTH = 8

export function SignUpForm() {
  const signUp = useSignUp()

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    signUp.mutate({
      email: textField(form, 'email'),
      password: textField(form, 'password'),
      displayName: textField(form, 'displayName'),
    })
  }

  // Shown verbatim. A duplicate address gets a 409 whose body says an account
  // already exists and to sign in instead — the API discloses that on purpose,
  // so softening it here would leave the user with no idea why nothing arrived.
  const error = signUp.isError
    ? isApiError(signUp.error)
      ? signUp.error.message
      : 'Something went wrong. Try again.'
    : undefined

  if (signUp.isSuccess) {
    return (
      <AuthShell title="Check your inbox" description="One more step before you can sign in.">
        <div className="space-y-4">
          <FormSuccess message={signUp.data.message} />
          <p className="text-pretty text-sm text-muted-foreground">
            The link expires in 24 hours. If it does not arrive, check your spam folder.
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
    <AuthShell
      title="Create an account"
      description="Aurify turns a playlist's sound and lyrics into a cover."
      footer={
        <>
          Already have an account?{' '}
          <Link to="/sign-in" className="font-medium text-foreground underline underline-offset-4">
            Sign in
          </Link>
        </>
      }
    >
      <div className="space-y-4">
        <GoogleButton label="Sign up with Google" />
        <AuthDivider />

        <form onSubmit={handleSubmit} className="space-y-4" noValidate>
          <FormError message={error} />

          <Field id="displayName" label="Display name" autoComplete="name" hint="Optional." />
          <Field id="email" label="Email" type="email" autoComplete="email" required />
          <Field
            id="password"
            label="Password"
            type="password"
            autoComplete="new-password"
            required
            hint={`At least ${MIN_PASSWORD_LENGTH} characters. Length beats punctuation — a passphrase is ideal.`}
          />

          <Button type="submit" className="w-full" disabled={signUp.isPending}>
            {signUp.isPending ? 'Creating account…' : 'Sign up'}
          </Button>
        </form>
      </div>
    </AuthShell>
  )
}
