import type { ComponentProps } from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { AlertCircle, CheckCircle2 } from 'lucide-react'

import { cn } from '@/lib/utils'

const alertVariants = cva('flex items-start gap-2 rounded-lg border px-3 py-2 text-sm', {
  variants: {
    variant: {
      destructive: 'border-destructive/40 bg-destructive/10 text-destructive-text',
      success: 'border-primary/40 bg-primary/10 text-foreground',
    },
  },
  defaultVariants: {
    variant: 'destructive',
  },
})

export type AlertProps = ComponentProps<'div'> & VariantProps<typeof alertVariants>

/**
 * A message about something that just happened. A failure is announced at
 * once (`role="alert"`); a success waits its turn (`role="status"`).
 */
export function Alert({ className, variant, children, ...props }: AlertProps) {
  const success = variant === 'success'
  const Icon = success ? CheckCircle2 : AlertCircle
  return (
    <div
      data-slot="alert"
      role={success ? 'status' : 'alert'}
      className={cn(alertVariants({ variant }), className)}
      {...props}
    >
      <Icon aria-hidden="true" className={cn('mt-0.5 size-4 shrink-0', success && 'text-primary')} />
      <span className="text-pretty">{children}</span>
    </div>
  )
}
