import { useEffect, useReducer, useRef } from 'react'

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

// What the waiting stages turn into while they wait. Analyzing is a
// rate-limited feature lookup, roughly a second per uncached track, so at a cap
// of 100 it is a minute or two of a single word sitting there looking hung.
// Painting is shorter but still long enough to look stuck.
//
// Order matters only in that the first entry is the plain one, which is what a
// stage shows the instant it starts.
const STAGE_VERBS: Partial<Record<CoverStatus, readonly string[]>> = {
  analyzing: ['Listening', 'Judging', 'Enjoying', 'Grooving', 'Vibing', 'Absorbing', 'Savoring', 'Pondering'],
  generating: ['Painting', 'Sketching', 'Mixing', 'Shading', 'Layering', 'Composing', 'Blending'],
}

const VERB_MS = 4000

// How often the stage re-announces itself to a screen reader. Far rarer than
// the visible cycle: the words are decoration and must not be spoken, but two
// minutes of total silence reads as a dead page.
const ANNOUNCE_MS = 30_000

/**
 * What the stage says, visibly and aloud.
 *
 * `visible` cycles through synonyms, derived from the wall clock rather than a
 * counter so the gallery tile and the playlist row show the same word for the
 * same cover without sharing anything.
 *
 * `announced` is the stable label plus how long the stage has been running,
 * changing only every ANNOUNCE_MS. There is deliberately no progress here,
 * because nothing measures progress; elapsed time is the one honest thing this
 * screen knows. It counts from when this view saw the stage, so opening a page
 * mid-generation starts from zero.
 */
export function useStageLabel(status: CoverStatus): { visible: string; announced: string } {
  const verbs = STAGE_VERBS[status]
  const [, tick] = useReducer((n: number) => n + 1, 0)
  const startedAt = useRef(Date.now())

  useEffect(() => {
    startedAt.current = Date.now()
  }, [status])

  useEffect(() => {
    if (!verbs) return
    const id = setInterval(tick, VERB_MS)
    return () => clearInterval(id)
  }, [verbs])

  const label = STATUS_LABEL[status]
  if (!verbs) return { visible: label, announced: label }

  // Bucketed, so the announced string is stable between announcements and the
  // live region stays quiet. The visible cycle is what drives the re-render.
  const elapsed = Math.floor((Date.now() - startedAt.current) / ANNOUNCE_MS) * (ANNOUNCE_MS / 1000)
  return {
    visible: verbs[Math.floor(Date.now() / VERB_MS) % verbs.length],
    announced: elapsed > 0 ? `${label}, ${spokenDuration(elapsed)}` : label,
  }
}

/** Spelled out, because a screen reader reads "90s" as letters. */
function spokenDuration(seconds: number): string {
  if (seconds < 60) return `${seconds} seconds`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  const spoken = `${minutes} minute${minutes === 1 ? '' : 's'}`
  return rest > 0 ? `${spoken} ${rest} seconds` : spoken
}

/**
 * The lifecycle badge, shared by the gallery and the detail view.
 *
 * aria-live is on the badge so a status flipping is announced rather than
 * silent. Through a cycling stage the visible word is hidden from it, because
 * the cycle is a sign of life rather than progress and fifteen synonyms a
 * minute would be noise; what is announced instead is the stage and how long it
 * has been running. A settled status is one node, since there is nothing to
 * hide.
 */
export function StatusBadge({ status }: { status: CoverStatus }) {
  const { visible, announced } = useStageLabel(status)

  return (
    <Badge
      variant={STATUS_VARIANT[status]}
      aria-live="polite"
      // Reserved for the widest verb ("Composing" measures 83px), so the
      // playlist name beside it does not re-truncate every four seconds as the
      // word changes. Only while cycling: a settled badge stays snug.
      className={STAGE_VERBS[status] ? 'min-w-[5.5rem] justify-center' : undefined}
    >
      {STAGE_VERBS[status] ? (
        <>
          <span aria-hidden="true">{visible}</span>
          <span className="sr-only">{announced}</span>
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
