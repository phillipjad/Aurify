import { useId, useState } from 'react'
import { LogIn, LogOut, Moon, User, UserPlus } from 'lucide-react'
import { Link, useNavigate } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { useSession, useSignOut } from '@/lib/api/auth'
import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'

const itemClass = 'w-full justify-start gap-2.5 px-2.5 font-normal'

/**
 * The header's only account control: a squircle that opens a menu holding the
 * sign-in links or sign-out, plus the theme switch.
 *
 * A popover, not an ARIA menu: the rows are ordinary links, buttons and a
 * switch, so Tab already moves between them and no roving focus is needed.
 *
 * While the session is still loading it renders a same-size placeholder rather
 * than guessing. Showing a signed-out button first and swapping it for initials
 * a moment later reads as a flicker and, worse, as having been signed out.
 */
export function UserMenu() {
  const navigate = useNavigate()
  const { data: session, isPending } = useSession()
  const signOut = useSignOut()
  const { theme, toggleTheme } = useTheme()
  const [open, setOpen] = useState(false)
  const switchId = useId()

  if (isPending) {
    return <span className="size-control-sm" aria-hidden="true" />
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant={session ? 'default' : 'outline'}
          size="icon-sm"
          aria-label={session ? `Account: ${session.displayName?.trim() || session.email}` : 'Account'}
          className={cn('text-xs font-semibold', !session && 'text-muted-foreground')}
        >
          {session ? initialsOf(session.displayName, session.email) : <User aria-hidden="true" className="size-4" />}
        </Button>
      </PopoverTrigger>

      <PopoverContent align="end" className="flex w-56 flex-col gap-px p-1">
        {!session && (
          <>
            <Button asChild variant="ghost" className={itemClass}>
              <Link to="/sign-in" onClick={() => setOpen(false)}>
                <LogIn aria-hidden="true" className="size-4 text-muted-foreground" />
                Sign in
              </Link>
            </Button>
            <Button asChild variant="ghost" className={itemClass}>
              <Link to="/sign-up" onClick={() => setOpen(false)}>
                <UserPlus aria-hidden="true" className="size-4 text-muted-foreground" />
                Create account
              </Link>
            </Button>
            <Separator className="my-1" />
          </>
        )}

        {/* Deliberately leaves the menu open: the point of a switch is
            watching it land, and the page behind it recolor. */}
        <Button asChild variant="ghost" className={itemClass}>
          <label htmlFor={switchId}>
            <Moon aria-hidden="true" className="size-4 text-muted-foreground" />
            Dark mode
            <Switch id={switchId} checked={theme === 'dark'} onCheckedChange={toggleTheme} className="ml-auto" />
          </label>
        </Button>

        {session && (
          <>
            <Separator className="my-1" />
            <Button
              variant="ghost"
              disabled={signOut.isPending}
              onClick={() => {
                signOut.mutate(undefined, {
                  // onSettled, not onSuccess: the cookies are cleared and the
                  // cache is dropped either way, so the UI must leave the
                  // signed-in state even if the request itself failed.
                  onSettled: () => {
                    setOpen(false)
                    void navigate({ to: '/' })
                  },
                })
              }}
              className={cn(itemClass, 'text-destructive')}
            >
              <LogOut aria-hidden="true" className="size-4" />
              {signOut.isPending ? 'Signing out…' : 'Sign out'}
            </Button>
          </>
        )}
      </PopoverContent>
    </Popover>
  )
}

/** Up to two initials, from the display name if there is one, else the email. */
function initialsOf(displayName: string | undefined, email: string | undefined): string {
  const words = (displayName?.trim() || email?.split('@')[0] || '?').split(/[\s._-]+/).filter(Boolean)
  return words
    .slice(0, 2)
    .map((word) => word[0]?.toUpperCase() ?? '')
    .join('')
}
