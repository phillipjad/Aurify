import type { CSSProperties } from 'react'
import { Toaster as Sonner, type ToasterProps } from 'sonner'

/**
 * shadcn's Sonner wrapper, adapted to this design system: colors come from the
 * app's own tokens rather than next-themes — the variables flip with `.dark`,
 * so the toasts follow the theme with no theme prop at all.
 */
export function Toaster(props: ToasterProps) {
  return (
    <Sonner
      className="toaster group"
      // Clear of the footer, which is always on screen in the bounded shell.
      offset={{ bottom: 56 }}
      mobileOffset={{ bottom: 56 }}
      // A toast that lingers 8s needs a dismissal that doesn't require knowing
      // about swipe.
      closeButton
      toastOptions={{
        // Sonner hardcodes several of its own grays, which do not follow this
        // app's theme: the description lands near 1.5:1 on the dark card, and
        // the close button paints a near-black glyph (--gray12) on it, so the
        // control is invisible until its hover background gives it away. Both
        // are re-pointed at real tokens. The `!` beats sonner's own selectors,
        // which are more specific than a utility class.
        classNames: {
          description: 'text-muted-foreground!',
          closeButton: [
            // 24px rather than sonner's 20, which is under the minimum target.
            'size-6! bg-card! border-border! text-muted-foreground!',
            'hover:bg-muted! hover:text-foreground! hover:border-border! transition-colors',
            // Its default focus ring is a black shadow, invisible on dark.
            'focus-visible:shadow-none! focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
          ].join(' '),
        },
        style: {
          '--normal-bg': 'var(--card)',
          '--normal-text': 'var(--card-foreground)',
          '--normal-border': 'var(--border)',
        } as CSSProperties,
      }}
      {...props}
    />
  )
}
