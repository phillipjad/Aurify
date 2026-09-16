import type { ComponentProps } from 'react'
import { Popover as PopoverPrimitive } from 'radix-ui'

import { cn } from '@/lib/utils'

export const Popover = PopoverPrimitive.Root
export const PopoverTrigger = PopoverPrimitive.Trigger

/** Dismisses on Escape and outside click, returns focus to the trigger, and stays inside the viewport. */
export function PopoverContent({
  className,
  sideOffset = 8,
  collisionPadding = 8,
  ...props
}: ComponentProps<typeof PopoverPrimitive.Content>) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-slot="popover-content"
        sideOffset={sideOffset}
        collisionPadding={collisionPadding}
        className={cn(
          'z-(--z-dropdown) w-72 max-w-(--radix-popover-content-available-width) rounded-xl border border-border bg-card p-3 text-card-foreground shadow-lg',
          className,
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  )
}
