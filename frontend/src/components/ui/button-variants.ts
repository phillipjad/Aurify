import { cva } from 'class-variance-authority'

// Variant definitions for <Button>, kept in their own module so button.tsx
// exports only a component. That satisfies react-refresh/only-export-components
// and preserves React Fast Refresh for the component file.
//
// Every variant carries the full interactive set (default, hover, active,
// disabled), so no surface ships a half-stated control. Focus is drawn once for
// the whole app in styles.css. Hover and active use dedicated tokens rather
// than an opacity fade, which would wash out the label along with the
// background. Heights come from the control-* spacing tokens.
export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-medium transition-colors duration-150 disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-active',
        outline: 'border border-border bg-transparent hover:bg-muted active:bg-secondary',
        ghost: 'hover:bg-muted active:bg-secondary',
        destructive: 'border border-destructive/40 text-destructive hover:bg-destructive/10 active:bg-destructive/15',
        // Site navigation. The current page is marked by weight and an
        // underline as well as color, so it survives color blindness and a
        // forced-colors mode.
        nav: [
          'text-muted-foreground hover:text-foreground',
          'aria-[current=page]:font-semibold aria-[current=page]:text-foreground aria-[current=page]:underline',
          'aria-[current=page]:decoration-primary aria-[current=page]:decoration-2 aria-[current=page]:underline-offset-8',
        ].join(' '),
        link: 'text-foreground underline underline-offset-4 hover:decoration-primary',
      },
      size: {
        default: 'h-control px-4',
        sm: 'h-control-sm px-3',
        lg: 'h-control-lg px-6',
        // Height and width separately, so a caller's h-auto leaves the width.
        icon: 'h-control w-control',
        'icon-sm': 'h-control-sm w-control-sm',
        // No box at all, for a link inside a sentence.
        inline: 'h-auto p-0',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)
