import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

/**
 * A patch of data-driven color: pass it as `style`. The hairline ring keeps a
 * swatch that matches the surface behind it from disappearing into it.
 */
export function Swatch({ className, ...props }: ComponentProps<'span'>) {
  return (
    <span
      aria-hidden="true"
      data-slot="swatch"
      className={cn('block shrink-0 rounded-full ring-1 ring-border', className)}
      {...props}
    />
  )
}
