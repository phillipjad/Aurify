import type { BadgeProps } from '@/components/ui/badge'
import type { CoverStatus } from '@/lib/api/types'

// Shared cover-lifecycle vocabulary, so the gallery and the detail view label
// and color a status identically.

/** Badge variant per lifecycle status. */
export const STATUS_VARIANT: Record<CoverStatus, BadgeProps['variant']> = {
  pending: 'secondary',
  analyzing: 'secondary',
  generating: 'default',
  ready: 'success',
  failed: 'destructive',
}

// Plain-language copy. "analyzing"/"generating" are internal pipeline stages;
// users care what is happening to *their* playlist.
export const STATUS_LABEL: Record<CoverStatus, string> = {
  pending: 'Queued',
  analyzing: 'Listening',
  generating: 'Painting',
  ready: 'Ready',
  failed: 'Failed',
}

const IN_PROGRESS: ReadonlySet<CoverStatus> = new Set<CoverStatus>(['pending', 'analyzing', 'generating'])

/** True while a cover is still moving through the pipeline (non-terminal). */
export function isInProgress(status: CoverStatus): boolean {
  return IN_PROGRESS.has(status)
}
