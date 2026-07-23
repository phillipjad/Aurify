import { startGoogleSignIn } from '@/lib/api/auth'
import { Button } from '@/components/ui/button'

/** Google's brand mark, inlined so it needs no network request and no CDN. */
function GoogleMark() {
  return (
    <svg aria-hidden="true" viewBox="0 0 18 18" className="size-4">
      <path
        fill="#4285F4"
        d="M17.64 9.2c0-.64-.06-1.25-.16-1.84H9v3.48h4.84a4.14 4.14 0 0 1-1.8 2.72v2.26h2.92c1.7-1.57 2.68-3.88 2.68-6.62Z"
      />
      <path
        fill="#34A853"
        d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.92-2.26c-.8.54-1.84.86-3.04.86-2.34 0-4.32-1.58-5.03-3.7H.96v2.33A9 9 0 0 0 9 18Z"
      />
      <path fill="#FBBC05" d="M3.97 10.72a5.4 5.4 0 0 1 0-3.44V4.95H.96a9 9 0 0 0 0 8.1l3.01-2.33Z" />
      <path
        fill="#EA4335"
        d="M9 3.58c1.32 0 2.5.45 3.44 1.35l2.58-2.58C13.46.9 11.43 0 9 0A9 9 0 0 0 .96 4.95l3.01 2.33C4.68 5.16 6.66 3.58 9 3.58Z"
      />
    </svg>
  )
}

/**
 * Starts Sign in with Google.
 *
 * A plain button doing a full-page navigation, not a fetch: the flow is a
 * top-level redirect out to Google and back, and the cookies carrying `state`,
 * `nonce` and the PKCE verifier are only sent on a navigation of that kind.
 */
export function GoogleButton({ redirectTo, label = 'Continue with Google' }: { redirectTo?: string; label?: string }) {
  return (
    <Button
      type="button"
      variant="outline"
      className="w-full gap-2"
      onClick={() => {
        startGoogleSignIn(redirectTo)
      }}
    >
      <GoogleMark />
      {label}
    </Button>
  )
}

/** "or" rule between the federated and password paths. */
export function AuthDivider() {
  return (
    <div className="flex items-center gap-3" aria-hidden="true">
      <span className="h-px flex-1 bg-border" />
      <span className="text-xs uppercase tracking-wide text-muted-foreground">or</span>
      <span className="h-px flex-1 bg-border" />
    </div>
  )
}
