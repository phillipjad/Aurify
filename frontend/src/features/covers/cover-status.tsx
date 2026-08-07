import { useEffect, useReducer } from 'react'

import { Badge, type BadgeProps } from '@/components/ui/badge'
import type { CoverStatus } from '@/lib/api/types'

// Shared cover-lifecycle vocabulary, so the gallery and the detail view label
// and color a status identically.

/** Badge variant per lifecycle status. */
const STATUS_VARIANT: Record<CoverStatus, BadgeProps['variant']> = {
  pending: 'secondary',
  analyzing: 'secondary',
  generating: 'default',
  ready: 'success',
  failed: 'destructive',
}

// Plain-language copy. "analyzing"/"generating" are internal pipeline stages;
// users care what is happening to *their* playlist.
const STATUS_LABEL: Record<CoverStatus, string> = {
  pending: 'Queued',
  analyzing: 'Listening',
  generating: 'Painting',
  ready: 'Ready',
  failed: 'Failed',
}

// What "Listening" turns into while it waits. The analyzing stage is a
// rate-limited feature lookup, roughly a second per uncached track, so at a cap
// of 100 it is a minute or two of a single word sitting there looking hung.
const ANALYZING_VERBS = ['Listening', 'Judging', 'Enjoying', 'Grooving']

const VERB_MS = 4000

/**
 * The stage label, cycling through synonyms while the playlist is being
 * analyzed.
 *
 * Derived from the wall clock rather than a counter, so the gallery tile and the
 * playlist row show the same word for the same cover without sharing anything.
 */
export function useStageLabel(status: CoverStatus): string {
  const [, tick] = useReducer((n: number) => n + 1, 0)

  useEffect(() => {
    if (status !== 'analyzing') return
    const id = setInterval(tick, VERB_MS)
    return () => clearInterval(id)
  }, [status])

  if (status !== 'analyzing') return STATUS_LABEL[status]
  return ANALYZING_VERBS[Math.floor(Date.now() / VERB_MS) % ANALYZING_VERBS.length]
}

/**
 * The lifecycle badge, shared by the gallery and the detail view.
 *
 * aria-live is on the badge so a status flipping is announced rather than
 * silent. While analyzing, the cycling word is hidden from it and the stable
 * "Listening" is announced instead: the cycle is a sign of life, not progress,
 * and four synonyms a minute would be noise. Every other status is one node,
 * since there is nothing to hide.
 */
export function StatusBadge({ status }: { status: CoverStatus }) {
  const stage = useStageLabel(status)

  return (
    <Badge variant={STATUS_VARIANT[status]} aria-live="polite">
      {status === 'analyzing' ? (
        <>
          <span aria-hidden="true">{stage}</span>
          <span className="sr-only">{STATUS_LABEL.analyzing}</span>
        </>
      ) : (
        STATUS_LABEL[status]
      )}
    </Badge>
  )
}

const IN_PROGRESS: ReadonlySet<CoverStatus> = new Set<CoverStatus>(['pending', 'analyzing', 'generating'])

/** True while a cover is still moving through the pipeline (non-terminal). */
export function isInProgress(status: CoverStatus): boolean {
  return IN_PROGRESS.has(status)
}
