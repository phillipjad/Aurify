import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

export type CardProps = ComponentProps<'div'> & {
  /**
   * Translucent + blurred, for the rare surface that should read as glass over
   * the aurora background.
   *
   * Off by default on purpose: applying it to every card made blur the app's
   * baseline cost — paid even by skeletons and empty states, which have nothing
   * to show through — and left text contrast dependent on whatever gradient
   * happened to sit behind it.
   */
  glass?: boolean
}

// shadcn/ui-style Card.
export function Card({ className, glass = false, ...props }: CardProps) {
  return (
    <div
      data-slot="card"
      className={cn(
        'rounded-xl border border-border text-card-foreground shadow-sm',
        glass ? 'bg-card/70 backdrop-blur-md' : 'bg-card',
        className,
      )}
      {...props}
    />
  )
}

export function CardHeader({ className, ...props }: ComponentProps<'div'>) {
  return <div data-slot="card-header" className={cn('flex flex-col gap-1.5 p-5', className)} {...props} />
}

export function CardTitle({ className, ...props }: ComponentProps<'h3'>) {
  return (
    <h3
      data-slot="card-title"
      className={cn('font-display text-lg leading-tight font-semibold tracking-tight', className)}
      {...props}
    />
  )
}

export function CardDescription({ className, ...props }: ComponentProps<'p'>) {
  return <p data-slot="card-description" className={cn('text-sm text-muted-foreground', className)} {...props} />
}

export function CardContent({ className, ...props }: ComponentProps<'div'>) {
  return <div data-slot="card-content" className={cn('p-5 pt-0', className)} {...props} />
}

export function CardFooter({ className, ...props }: ComponentProps<'div'>) {
  return <div data-slot="card-footer" className={cn('flex items-center p-5 pt-0', className)} {...props} />
}
