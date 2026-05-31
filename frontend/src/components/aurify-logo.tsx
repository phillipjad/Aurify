import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

// The Aurify mark: a gradient orb on a rounded dark tile (matches favicon.svg).
export function AurifyLogo({ className, ...props }: ComponentProps<'svg'>) {
  return (
    <svg
      viewBox="0 0 64 64"
      role="img"
      aria-label="Aurify"
      className={cn('h-8 w-8', className)}
      {...props}
    >
      <defs>
        <linearGradient id="aurifyLogoGradient" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#FF5A36" />
          <stop offset="0.5" stopColor="#FFB23E" />
          <stop offset="1" stopColor="#4F86C6" />
        </linearGradient>
      </defs>
      <rect width="64" height="64" rx="16" fill="#0b0b0f" />
      <circle cx="32" cy="32" r="16" fill="url(#aurifyLogoGradient)" />
      <circle cx="32" cy="32" r="6" fill="#0b0b0f" />
    </svg>
  )
}
