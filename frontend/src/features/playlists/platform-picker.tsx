import { cn } from '@/lib/utils'
import type { Platform } from '@/lib/api/types'
import { PLATFORMS, platformLabel } from './platforms'

interface PlatformPickerProps {
  value: Platform
  onChange: (platform: Platform) => void
}

/**
 * Segmented control for choosing the source DSP.
 *
 * Built as a radiogroup rather than a row of buttons: the previous version
 * signalled the active platform with fill color alone, so assistive tech
 * announced all three options identically. Selection now carries through
 * `aria-checked`, a weight change, and an inset surface — three redundant cues,
 * none of them color-only.
 *
 * Roving tabindex keeps the group a single tab stop, which is what the WAI-ARIA
 * radiogroup pattern expects; arrow keys move between options.
 */
export function PlatformPicker({ value, onChange }: PlatformPickerProps) {
  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    const delta =
      event.key === 'ArrowRight' || event.key === 'ArrowDown'
        ? 1
        : event.key === 'ArrowLeft' || event.key === 'ArrowUp'
          ? -1
          : 0
    if (delta === 0) return

    event.preventDefault()
    const index = PLATFORMS.indexOf(value)
    const next = PLATFORMS[(index + delta + PLATFORMS.length) % PLATFORMS.length]
    onChange(next)
  }

  return (
    <div
      role="radiogroup"
      aria-label="Music platform"
      onKeyDown={handleKeyDown}
      className="inline-flex rounded-lg border border-border bg-muted/60 p-0.5"
    >
      {PLATFORMS.map((platform) => {
        const selected = platform === value
        return (
          <button
            key={platform}
            type="button"
            role="radio"
            aria-checked={selected}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(platform)}
            className={cn(
              'rounded-md px-3 py-1.5 text-sm transition-colors duration-150',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background',
              selected
                ? 'bg-card font-semibold text-foreground shadow-sm'
                : 'font-medium text-muted-foreground hover:text-foreground',
            )}
          >
            {platformLabel(platform)}
          </button>
        )
      })}
    </div>
  )
}
