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
      // Clear of the always-on-screen footer: 29px desktop, 65px at 375px.
      offset={{ bottom: 56 }}
      mobileOffset={{ bottom: 76 }}
      // Concurrent generations stack rather than collapsing into an unreadable
      // pile, which Sonner's default (3, collapsed) makes of them.
      expand
      visibleToasts={5}
      // Dismissal that does not require knowing about swipe.
      closeButton
      toastOptions={{
        // Sonner's default is "Close toast", and "toast" is a word for the
        // people who build these, not the ones who hear them read out.
        closeButtonAriaLabel: 'Dismiss',
        // Sonner hardcodes greys that ignore this theme: the description landed
        // near 1.5:1 on the dark card and the close glyph was invisible on it.
        // `!` beats its own selectors, which outrank a bare utility class.
        classNames: {
          // Its focusable card carried a 1.05:1 focus ring on the dark card.
          toast:
            'focus-visible:shadow-lg! focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
          // Sonner puts both at 13px, separated by colour alone, which left the
          // action link the largest text in a notification about a picture.
          title: 'text-sm! font-semibold!',
          description: 'text-xs! text-muted-foreground!',
          // Wide enough for cover artwork; the default slot is a 16px glyph.
          icon: 'size-11! shrink-0! self-center!',
          closeButton: [
            // 24px rather than Sonner's 20, under the minimum target size.
            'size-6! bg-card! border-border! text-muted-foreground!',
            'hover:bg-muted! hover:text-foreground! hover:border-border! transition-colors',
            // Its own focus ring is a black shadow, invisible on dark.
            'focus-visible:shadow-none! focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card',
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
