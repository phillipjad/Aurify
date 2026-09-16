import type { ComponentProps } from 'react'
import type { VariantProps } from 'class-variance-authority'
import { Loader2 } from 'lucide-react'
import { Slot } from 'radix-ui'

import { cn } from '@/lib/utils'
import { buttonVariants } from './button-variants'

// A trimmed shadcn/ui "button". Variants live in ./button-variants so this file
// only exports a component.
export type ButtonProps = ComponentProps<'button'> &
  VariantProps<typeof buttonVariants> & {
    /**
     * Shows a spinner and blocks interaction while an action is in flight.
     * Distinct from `disabled`: a loading button announces `aria-busy` and keeps
     * full contrast, because it is working rather than unavailable.
     */
    loading?: boolean
    /**
     * Renders the single child (a router Link, a download anchor) with button
     * styling instead of wrapping it: an anchor inside a button is invalid.
     */
    asChild?: boolean
  }

export function Button({
  className,
  variant,
  size,
  asChild = false,
  loading = false,
  disabled,
  children,
  ...props
}: ButtonProps) {
  if (asChild) {
    return (
      <Slot.Root className={cn(buttonVariants({ variant, size }), className)} {...props}>
        {children}
      </Slot.Root>
    )
  }

  return (
    <button
      // Still `disabled` so the browser blocks the click, but opting out of the
      // dimming that goes with it, as the docstring above promises.
      className={cn(buttonVariants({ variant, size }), className, loading && 'opacity-100!')}
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
