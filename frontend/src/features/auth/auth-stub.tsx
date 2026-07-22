import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

const fieldClass =
  'h-9 w-full rounded-lg border border-border bg-card px-3 text-sm text-foreground placeholder:text-muted-foreground disabled:opacity-50'

interface AuthStubProps {
  mode: 'sign-in' | 'sign-up'
}

// SCAFFOLD: accounts aren't wired up yet (see AGENTS.md — auth is a
// deliberate TODO). Aurify works per-session today via the DSP OAuth connect
// flow (see PlaylistBrowser's "Connect Spotify/Apple Music" button); this is
// a disabled preview of the eventual sign-in surface.
export function AuthStub({ mode }: AuthStubProps) {
  const isSignUp = mode === 'sign-up'

  return (
    <div className="mx-auto max-w-sm space-y-6">
      <div className="space-y-1.5 text-center">
        <h1 className="font-display text-2xl font-bold tracking-tight">{isSignUp ? 'Create an account' : 'Sign in'}</h1>
        <p className="text-pretty text-sm text-muted-foreground">
          Coming soon. Aurify runs per-session for now, no account needed.
        </p>
      </div>

      <Card>
        <CardContent className="pt-5">
          <fieldset disabled className="space-y-4">
            <div className="space-y-1.5">
              <label htmlFor="email" className="text-sm font-medium">
                Email
              </label>
              <input id="email" type="email" placeholder="you@example.com" className={fieldClass} />
            </div>
            <div className="space-y-1.5">
              <label htmlFor="password" className="text-sm font-medium">
                Password
              </label>
              <input id="password" type="password" placeholder="••••••••" className={fieldClass} />
            </div>
            <Button type="submit" disabled className="w-full">
              {isSignUp ? 'Sign up' : 'Sign in'}
            </Button>
          </fieldset>
        </CardContent>
      </Card>
    </div>
  )
}
