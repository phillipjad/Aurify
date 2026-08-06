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
      // Clear of the footer, which is always on screen in the bounded shell and
      // is taller on mobile, where its links wrap to a second line. Measured:
      // 29px tall at desktop widths, 65px at 375px.
      offset={{ bottom: 56 }}
      mobileOffset={{ bottom: 76 }}
      // Concurrent generations are a designed-for case, so their toasts stack
      // rather than collapsing into a pile whose top card is the only readable
      // one. Sonner's default is 3 visible, collapsed.
      expand
      visibleToasts={5}
      // A toast that lingers 8s needs a dismissal that doesn't require knowing
      // about swipe. "Close toast" is Sonner's default, and "toast" is a word
      // for the people who build these, not the people who hear them read out.
      closeButton
      toastOptions={{
        closeButtonAriaLabel: 'Dismiss',
        // Sonner hardcodes several of its own grays, which do not follow this
        // app's theme: the description lands near 1.5:1 on the dark card, and
        // the close button paints a near-black glyph (--gray12) on it, so the
        // control is invisible until its hover background gives it away. Both
        // are re-pointed at real tokens. The `!` beats sonner's own selectors,
        // which are more specific than a utility class.
        classNames: {
          // The card itself is focusable (Sonner gives it tabindex=0) and
          // carried its default rgba(0,0,0,0.2) ring, which measures 1.05:1 on
          // the dark card: a focus indicator nobody can see is not one.
          toast:
            'focus-visible:shadow-lg! focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
          // Sonner's own scale puts title and description at the same 13px and
          // separates them by colour alone, which left the action link the
          // largest text in a notification about a picture. Run it downward.
          title: 'text-sm! font-semibold!',
          description: 'text-xs! text-muted-foreground!',
          // Wide enough for cover artwork; the default slot is a 16px glyph.
          icon: 'size-11! shrink-0! self-center!',
          closeButton: [
            // 24px rather than sonner's 20, which is under the minimum target.
            'size-6! bg-card! border-border! text-muted-foreground!',
            'hover:bg-muted! hover:text-foreground! hover:border-border! transition-colors',
            // Its default focus ring is a black shadow, invisible on dark. The
            // offset is the card, which is the surface the button sits on.
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
