import { cva } from 'class-variance-authority'

// Variant definitions for <Button>, kept in their own module so button.tsx
// exports only a component. That satisfies react-refresh/only-export-components
// and preserves React Fast Refresh for the component file.
//
// Every variant carries the full interactive set — default, hover, active,
// focus-visible, disabled — so no surface ships a half-stated control. Hover
// and active use dedicated tokens rather than an opacity fade, which would
// wash out the label along with the background.
export const buttonVariants = cva(
  [
    'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-medium',
    'transition-colors duration-150',
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
    'disabled:pointer-events-none disabled:opacity-50',
  ].join(' '),
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-active',
        outline: 'border border-border bg-transparent hover:bg-muted active:bg-secondary',
        ghost: 'hover:bg-muted active:bg-secondary',
      },
      size: {
        default: 'h-10 px-4 py-2',
        sm: 'h-9 px-3',
        lg: 'h-11 px-6',
        icon: 'h-10 w-10',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)
