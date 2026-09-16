import type { ComponentProps } from 'react'
import { ChevronDown } from 'lucide-react'

import { cn } from '@/lib/utils'

/**
 * A styled native <select>. Native rather than a custom listbox: phones get
 * their own picker, and there is nothing here a listbox would add.
 */
export function NativeSelect({ className, ...props }: ComponentProps<'select'>) {
  return (
    <span className={cn('relative inline-flex', className)}>
      <select
        data-slot="native-select"
        className="h-control-sm w-full appearance-none rounded-lg border border-border bg-card pr-8 pl-2.5 text-sm text-foreground disabled:cursor-not-allowed disabled:opacity-50"
        {...props}
      />
      <ChevronDown
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 right-2.5 size-4 -translate-y-1/2 text-muted-foreground"
      />
    </span>
  )
}
