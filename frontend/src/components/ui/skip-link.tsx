import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

/** Hidden until a keyboard user tabs onto it, then jumps past the header. */
export function SkipLink({ className, ...props }: ComponentProps<'a'>) {
  return (
    <a
      className={cn(
        'sr-only focus:not-sr-only focus:absolute focus:top-4 focus:left-4 focus:z-(--z-toast) focus:rounded-md focus:bg-card focus:px-4 focus:py-3 focus:text-sm focus:font-medium focus:shadow-md',
        className,
      )}
      {...props}
    />
  )
}
