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
      className={cn(buttonVariants({ variant, size, className }))}
      disabled={disabled ?? loading}
      aria-busy={loading || undefined}
      {...props}
    >
      {loading && <Loader2 aria-hidden="true" className="size-4 shrink-0 animate-spin" />}
      {children}
    </button>
  )
}
