import type { ComponentProps } from 'react'
import { RadioGroup } from 'radix-ui'

import { cn } from '@/lib/utils'

/**
 * One choice from a few, shown side by side. A radiogroup underneath: one tab
 * stop, arrow keys move the selection, and a selection can never be cleared.
 *
 * The selected segment is marked three ways, none of them color alone:
 * `aria-checked`, a heavier weight, and a raised surface.
 */
export function SegmentedControl({ className, ...props }: ComponentProps<typeof RadioGroup.Root>) {
  return (
    <RadioGroup.Root
      data-slot="segmented-control"
      className={cn('inline-flex h-control-sm rounded-lg border border-border bg-muted/60 p-0.5', className)}
      {...props}
    />
  )
}

export function SegmentedControlItem({ className, ...props }: ComponentProps<typeof RadioGroup.Item>) {
  return (
    <RadioGroup.Item
      data-slot="segmented-control-item"
      className={cn(
        'rounded-md px-3 text-sm font-medium text-muted-foreground transition-colors duration-150 hover:text-foreground',
        'data-[state=checked]:bg-card data-[state=checked]:font-semibold data-[state=checked]:text-foreground data-[state=checked]:shadow-sm',
        className,
      )}
      {...props}
    />
  )
}
