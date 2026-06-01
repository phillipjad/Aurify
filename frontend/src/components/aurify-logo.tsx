import type { ComponentProps } from 'react'

import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'

// Tile (badge) color behind the gradient orb, and the punched-out center that
// reads as a ring. A dark tile reads well on light themes; a light tile reads
// well on dark themes. Picked from the active theme so the mark always has
// contrast against the page background.
const TILE: Record<'light' | 'dark', string> = {
  light: '#0b0b0f',
  dark: '#f1eeff',
}

// The Aurify mark: a gradient orb on a rounded tile. The tile auto-switches
// with the active theme (see useTheme / ThemeProvider).
export function AurifyLogo({ className, ...props }: ComponentProps<'svg'>) {
  const { theme } = useTheme()
  const tile = TILE[theme]

  return (
    <svg viewBox="0 0 64 64" role="img" aria-label="Aurify" className={cn('h-8 w-8', className)} {...props}>
      <defs>
        <linearGradient id="aurifyLogoGradient" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#FF5A36" />
          <stop offset="0.5" stopColor="#FFB23E" />
          <stop offset="1" stopColor="#4F86C6" />
        </linearGradient>
      </defs>
      <rect width="64" height="64" rx="16" fill={tile} />
      <circle cx="32" cy="32" r="16" fill="url(#aurifyLogoGradient)" />
      <circle cx="32" cy="32" r="6" fill={tile} />
    </svg>
  )
}
