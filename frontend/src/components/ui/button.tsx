import type { ButtonHTMLAttributes } from 'react'
import type { VariantProps } from 'class-variance-authority'
import { Loader2 } from 'lucide-react'

import { cn } from '@/lib/utils'
import { buttonVariants } from './button-variants'

// A trimmed shadcn/ui "button". Run `pnpm dlx shadcn@latest add button` to
// replace this with the full component (including the Slot/asChild support).
// Variants live in ./button-variants so this file only exports a component.
export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof buttonVariants> & {
    /**
     * Shows a spinner and blocks interaction while an action is in flight.
     * Distinct from `disabled`: a loading button announces `aria-busy` and keeps
     * full contrast, because it is working rather than unavailable.
     */
    loading?: boolean
  }

export function Button({ className, variant, size, loading = false, disabled, children, ...props }: ButtonProps) {
  return (
    <button
      // A loading button is still `disabled`, so the browser blocks the click
      // for free, but it opts out of the dimming that goes with it: the promise
      // above is full contrast while working, and a generation that runs for
      // two minutes spends that whole time as the row's only status readout.
      className={cn(buttonVariants({ variant, size, className }), loading && 'opacity-100!')}
      disabled={disabled ?? loading}
      aria-busy={loading || undefined}
      {...props}
    >
      {/* data-motion="busy" keeps this turning under prefers-reduced-motion,
          where the global reset would otherwise freeze it on frame one and make
          a working button look like a broken one (see styles.css). */}
      {loading && <Loader2 aria-hidden="true" data-motion="busy" className="size-4 shrink-0 animate-spin" />}
      {children}
    </button>
  )
}
