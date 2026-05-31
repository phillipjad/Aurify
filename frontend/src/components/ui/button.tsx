import type { ButtonHTMLAttributes } from 'react'
import type { VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'
import { buttonVariants } from './button-variants'

// A trimmed shadcn/ui "button". Run `pnpm dlx shadcn@latest add button` to
// replace this with the full component (including the Slot/asChild support).
// Variants live in ./button-variants so this file only exports a component.
export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof buttonVariants>

export function Button({ className, variant, size, ...props }: ButtonProps) {
  return (
    <button
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}
