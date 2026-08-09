import { useEffect, useReducer, useRef } from 'react'

import { Badge, type BadgeProps } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
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
  // One verb, because a queued cover is not doing anything yet. It still earns
  // the ellipsis, so every waiting state animates and only settled ones sit
  // still.
  pending: ['Queued'],
  analyzing: ['Listening', 'Judging', 'Enjoying', 'Grooving', 'Vibing', 'Absorbing', 'Savoring', 'Pondering'],
  generating: ['Painting', 'Sketching', 'Mixing', 'Shading', 'Layering', 'Composing', 'Blending'],
}

// One period per second, and a verb holds for a full ellipsis before handing
// over: "Listening" · "Listening." · "Listening.." · "Listening..." · "Judging".
const DOT_MS = 1000
const STEPS_PER_VERB = 4

// How often the stage re-announces itself to a screen reader. Far rarer than
// the visible cycle: the words are decoration and must not be spoken, but two
// minutes of total silence reads as a dead page.
const ANNOUNCE_MS = 30_000

/**
 * What the stage says: `visible` cycles, `announced` does not.
 *
 * Both are anchored to the moment the stage began, not to the wall clock; the
 * reasoning and what that trades away are in
 * docs/adr/0020-coverage-weighted-features.md.
 */
export function useStageLabel(status: CoverStatus): { visible: string; dots: string; announced: string } {
  const verbs = STAGE_VERBS[status]
  const [, tick] = useReducer((n: number) => n + 1, 0)

  // Set during render, not in an effect. An effect lands a frame late, and the
  // frame it misses is the one still showing the previous stage's word — the
  // flicker this anchor exists to remove.
  const cycle = useRef({ status, at: Date.now() })
  if (cycle.current.status !== status) cycle.current = { status, at: Date.now() }

  useEffect(() => {
    if (!verbs) return
    const id = setInterval(tick, DOT_MS)
    return () => clearInterval(id)
  }, [verbs])

  const label = STATUS_LABEL[status]
  if (!verbs) return { visible: label, dots: '', announced: label }

  const since = Date.now() - cycle.current.at
  const step = Math.floor(since / DOT_MS)
  // Bucketed, so the announced string is stable between announcements and the
  // live region stays quiet while the visible half ticks every second.
  const elapsed = Math.floor(since / ANNOUNCE_MS) * (ANNOUNCE_MS / 1000)

  return {
    visible: verbs[Math.floor(step / STEPS_PER_VERB) % verbs.length],
    dots: '.'.repeat(step % STEPS_PER_VERB),
    announced: elapsed > 0 ? `${label}, ${spokenDuration(elapsed)}` : label,
  }
}

/**
 * The verb and its ellipsis, in two reserves so nothing under them moves.
 *
 * Measured: "Composing" is 5.11em and three dots 0.9em, so 6.25em clears the
 * pair. In em because the badge sets 12px and the button 14px, and one rem
 * value cannot be right for both. Purely decorative — every caller hides it
 * from assistive tech and announces a stable label instead.
 */
export function StageLabel({ visible, dots }: { visible: string; dots: string }) {
  return (
    <span className="inline-block min-w-[6.25em] text-left">
      {visible}
      <span className="inline-block w-[0.9em] text-left">{dots}</span>
    </span>
  )
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
 * The cycling word is always hidden from assistive tech and replaced by the
 * stable stage, so the badge reads as "Listening" however it currently looks.
 *
 * `live` is off by default because the gallery renders one of these per tile:
 * with several generations running that was several live regions announcing
 * unattributed stage changes at each other. Completion is already announced
 * once, and by name, by the ready toast (lib/cover-toasts.tsx). Turn it on
 * where the page really is about one cover.
 */
export function StatusBadge({ status, live = false }: { status: CoverStatus; live?: boolean }) {
  const { visible, dots, announced } = useStageLabel(status)

  return (
    // No width reserve here: StageLabel carries its own, so a cycling badge is
    // already a constant width and the playlist name beside it never
    // re-truncates. A settled badge has no StageLabel and stays snug.
    <Badge variant={STATUS_VARIANT[status]} aria-live={live ? 'polite' : undefined}>
      {STAGE_VERBS[status] ? (
        <>
          <span aria-hidden="true">
            <StageLabel visible={visible} dots={dots} />
          </span>
          <span className="sr-only">{announced}</span>
        </>
      ) : (
        STATUS_LABEL[status]
      )}
    </Badge>
  )
}

/**
 * Lucide's `sparkles`, copied from lucide-react 1.25.0 because the package
 * exports the component and not its geometry. A mask rather than `<Sparkles>`
 * because an SVG stroked with `currentColor` cannot carry a gradient.
 */
const SPARKLE_MASK = `url("data:image/svg+xml,${encodeURIComponent(
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
    `<path d="M11.017 2.814a1 1 0 0 1 1.966 0l1.051 5.558a2 2 0 0 0 1.594 1.594l5.558 1.051a1 1 0 0 1 0 1.966l-5.558 1.051a2 2 0 0 0-1.594 1.594l-1.051 5.558a1 1 0 0 1-1.966 0l-1.051-5.558a2 2 0 0 0-1.594-1.594l-5.558-1.051a1 1 0 0 1 0-1.966l5.558-1.051a2 2 0 0 0 1.594-1.594z"/>` +
    `<path d="M20 2v4"/><path d="M22 4h-4"/><circle cx="4" cy="20" r="2"/></svg>`,
)}")`

/**
 * The mark that stands in for artwork a cover does not have yet.
 *
 * At rest it is the plain muted glyph. While the cover is still being made, an
 * aurora drifts through it (see .aurora-glyph in styles.css), which reads as
 * being worked on rather than merely waiting.
 */
export function CoverGlyph({ working, className }: { working: boolean; className?: string }) {
  return (
    <span
      aria-hidden="true"
      data-motion={working ? 'aurora' : undefined}
      className={cn('block bg-muted-foreground', working && 'aurora-glyph', className)}
      style={{
        maskImage: SPARKLE_MASK,
        WebkitMaskImage: SPARKLE_MASK,
        maskSize: 'contain',
        maskRepeat: 'no-repeat',
        maskPosition: 'center',
      }}
    />
  )
}

const IN_PROGRESS: ReadonlySet<CoverStatus> = new Set<CoverStatus>(['pending', 'analyzing', 'generating'])

/** True while a cover is still moving through the pipeline (non-terminal). */
export function isInProgress(status: CoverStatus): boolean {
  return IN_PROGRESS.has(status)
}
