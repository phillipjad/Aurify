import { useEffect, useId, useRef, useState } from 'react'
import { LogIn, LogOut, Moon, User, UserPlus } from 'lucide-react'
import { Link, useNavigate } from '@tanstack/react-router'

import { useSession, useSignOut } from '@/lib/api/auth'
import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'

const itemClass = [
  'flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm text-foreground',
  'transition-colors hover:bg-muted',
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
].join(' ')

/**
 * The header's only account control: a squircle that opens a menu holding the
 * sign-in links or sign-out, plus the theme switch.
 *
 * A disclosure, not an ARIA menu — the rows are ordinary links, buttons and a
 * switch, so Tab already moves between them and no roving-focus code is needed.
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
  const rootRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuId = useId()

  // Close on an outside click or Escape. Escape also returns focus to the
  // trigger, so keyboard users are not dropped back at the top of the document.
  useEffect(() => {
    if (!open) return

    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      setOpen(false)
      triggerRef.current?.focus()
    }

    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  if (isPending) {
    return <span className="size-9" aria-hidden="true" />
  }

  const isDark = theme === 'dark'

  return (
    <div ref={rootRef} className="relative">
      <button
        ref={triggerRef}
        type="button"
        onClick={() => setOpen((wasOpen) => !wasOpen)}
        aria-expanded={open}
        aria-controls={menuId}
        aria-label={session ? `Account: ${session.displayName?.trim() || session.email}` : 'Account'}
        className={cn(
          'grid size-9 place-items-center rounded-[0.7rem] text-xs font-semibold transition-colors',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
          session
            ? 'bg-primary text-primary-foreground hover:bg-primary-hover'
            : 'border border-border text-muted-foreground hover:bg-muted hover:text-foreground',
        )}
      >
        {session ? initialsOf(session.displayName, session.email) : <User aria-hidden="true" className="size-[18px]" />}
      </button>

      {open && (
        <div
          id={menuId}
          className="absolute right-0 top-full z-[var(--z-dropdown)] mt-2 flex w-56 flex-col gap-px rounded-xl border border-border bg-card p-1 shadow-lg"
        >
          {!session && (
            <>
              <Link to="/sign-in" className={itemClass} onClick={() => setOpen(false)}>
                <LogIn aria-hidden="true" className="size-4 text-muted-foreground" />
                Sign in
              </Link>
              <Link to="/sign-up" className={itemClass} onClick={() => setOpen(false)}>
                <UserPlus aria-hidden="true" className="size-4 text-muted-foreground" />
                Create account
              </Link>
              <Rule />
            </>
          )}

          {/* Deliberately leaves the menu open: the point of a switch is
              watching it land, and the page behind it recolor. */}
          <button type="button" role="switch" aria-checked={isDark} onClick={toggleTheme} className={itemClass}>
            <Moon aria-hidden="true" className="size-4 text-muted-foreground" />
            Dark mode
            <span
              aria-hidden="true"
              className={cn(
                'ml-auto h-5 w-9 shrink-0 rounded-full p-0.5 transition-colors',
                isDark ? 'bg-primary' : 'bg-border',
              )}
            >
              <span
                className={cn(
                  'block size-4 rounded-full bg-card shadow-sm transition-transform motion-reduce:transition-none',
                  isDark && 'translate-x-4',
                )}
              />
            </span>
          </button>

          {session && (
            <>
              <Rule />
              <button
                type="button"
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
                className={cn(itemClass, 'text-destructive disabled:opacity-50')}
              >
                <LogOut aria-hidden="true" className="size-4 text-destructive" />
                {signOut.isPending ? 'Signing out…' : 'Sign out'}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  )
}

function Rule() {
  return <span className="my-1 h-px bg-border" aria-hidden="true" />
}

/** Up to two initials, from the display name if there is one, else the email. */
function initialsOf(displayName: string | undefined, email: string | undefined): string {
  const words = (displayName?.trim() || email?.split('@')[0] || '?').split(/[\s._-]+/).filter(Boolean)
  return words
    .slice(0, 2)
    .map((word) => word[0]?.toUpperCase() ?? '')
    .join('')
}
