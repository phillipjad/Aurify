import { clsx, type ClassValue } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

// Taught the theme's own names (styles.css), so `h-auto` passed to a primitive
// replaces its `h-control` instead of both landing in the class list.
const twMerge = extendTailwindMerge({
  extend: {
    theme: {
      spacing: ['control-sm', 'control', 'control-lg'],
      leading: ['display'],
    },
  },
})

/** cn merges class names with Tailwind-aware conflict resolution (shadcn/ui). */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
