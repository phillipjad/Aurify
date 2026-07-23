import { Link, useNavigate } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { useSession, useSignOut } from '@/lib/api/auth'

const navLinkClass = [
  'rounded-md px-3 py-2 text-sm font-medium text-muted-foreground',
  'transition-colors hover:text-foreground',
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
].join(' ')

/**
 * Header account controls: sign-in links when signed out, the user and a
 * sign-out button when signed in.
 *
 * While the session is still loading it renders nothing rather than guessing.
 * Showing "Sign in" first and swapping it for a name a moment later reads as a
 * flicker and, worse, as having been signed out.
 */
export function UserMenu() {
  const navigate = useNavigate()
  const { data: session, isPending } = useSession()
  const signOut = useSignOut()

  if (isPending) {
    // Reserve the space so the header does not jump when this resolves.
    return <span className="h-9 w-24" aria-hidden="true" />
  }

  if (!session) {
    return (
      <>
        <Link to="/sign-in" className={navLinkClass}>
          Sign in
        </Link>
        <Link to="/sign-up" className={buttonVariants({ variant: 'outline', size: 'sm' })}>
          Sign up
        </Link>
      </>
    )
  }

  return (
    <>
      <span className="max-w-[12rem] truncate px-3 py-2 text-sm text-muted-foreground" title={session.email}>
        {session.displayName?.trim() || session.email}
      </span>
      <Button
        variant="outline"
        size="sm"
        disabled={signOut.isPending}
        onClick={() => {
          signOut.mutate(undefined, {
            // onSettled, not onSuccess: the cookies are cleared and the cache is
            // dropped either way, so the UI must leave the signed-in state even
            // if the request itself failed.
            onSettled: () => {
              void navigate({ to: '/' })
            },
          })
        }}
      >
        {signOut.isPending ? 'Signing out…' : 'Sign out'}
      </Button>
    </>
  )
}
