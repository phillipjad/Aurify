import { useEffect, useRef, useState } from 'react'
import {
  AlertTriangle,
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Download,
  ImageOff,
  Info,
  ListMusic,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Card } from '@/components/ui/card'
import { EmptyState } from '@/components/empty-state'
import { ImageWithFallback } from '@/components/image-with-fallback'
import { Skeleton } from '@/components/ui/skeleton'
import { isApiError } from '@/lib/api/client'
import { useDeleteCoverRevision, useGenerateCover, useSetPlaylistCover } from '@/lib/api/commands'
import { useCover } from '@/lib/api/queries'
import { cn } from '@/lib/utils'
import type { ColorWeight, Cover, CoverRevision } from '@/lib/api/types'
import { platformLabel } from '@/features/playlists/platforms'
import { CoverGlyph, StatusBadge, isInProgress } from './cover-status'

export function CoverDetail({ coverId }: { coverId: string }) {
  // Runs that produced nothing are off by default; the toggle above the list
  // turns them on. Held here because it is what the query asks the server for,
  // not something the list filters after the fact.
  const [showFailedRuns, setShowFailedRuns] = useState(false)
  const cover = useCover(coverId, showFailedRuns)

  // strict: false so this reads whatever route it is mounted under, which is
  // what keeps it renderable in isolation. Narrowed again here rather than
  // trusted: the route's validateSearch only covers navigation into the route,
  // and at this looseness the value's shape genuinely is not known.
  const search = useSearch({ strict: false }) as Record<string, unknown>
  const rev = Number(search.rev)
  const revisionNumber = Number.isInteger(rev) && rev > 0 ? rev : undefined

  if (cover.isPending) return <CoverDetailSkeleton />

  if (cover.isError) {
    const notFound = isApiError(cover.error) && cover.error.status === 404
    return (
      <EmptyState
        icon={ImageOff}
        title={notFound ? 'This cover doesn’t exist' : 'Couldn’t load this cover'}
        description={
          notFound
            ? 'It may have been deleted. It won’t appear in your gallery anymore.'
            : 'The request failed before it reached your library.'
        }
        action={
          <Link to="/covers" className={buttonVariants({ size: 'sm' })}>
            <ArrowLeft aria-hidden="true" className="size-4" />
            All covers
          </Link>
        }
      />
    )
  }

  return (
    <CoverDetailView
      cover={cover.data}
      revisionNumber={revisionNumber}
      showFailedRuns={showFailedRuns}
      onShowFailedRuns={setShowFailedRuns}
    />
  )
}

function CoverDetailView({
  cover,
  revisionNumber,
  showFailedRuns,
  onShowFailedRuns,
}: {
  cover: Cover
  revisionNumber?: number
  showFailedRuns: boolean
  onShowFailedRuns: (show: boolean) => void
}) {
  const navigate = useNavigate()
  const regenerate = useGenerateCover()
  const push = useSetPlaylistCover()

  const working = isInProgress(cover.status)

  // Only successful runs can be shown, because only they have artwork. Newest
  // first, so index 0 is the current one.
  const runs = (cover.revisions ?? []).filter((r) => r.status === 'ready')
  // A `rev` naming a run that never existed, or one since deleted, falls back to
  // the current artwork rather than erroring. A shared link outliving its
  // revision should still open the cover.
  const index = Math.max(
    0,
    runs.findIndex((r) => r.number === revisionNumber),
  )
  const selected = runs[index]

  const select = (to: number) => {
    const run = runs[to]
    if (!run) return
    void navigate({
      to: '/covers/$coverId',
      params: { coverId: cover.id },
      // The current revision is the page's default, so it leaves the URL clean
      // rather than pinning a number that changes on the next generation.
      search: to === 0 ? {} : { rev: run.number },
      // Replace, so stepping through eight runs does not put eight entries
      // between the user and the page they came from.
      replace: true,
    })
  }

  // While a first generation is still running there are no revisions yet, so the
  // cover's own current-artwork fields are all there is to show.
  const imageUrl = selected?.imageUrl ?? cover.imageUrl
  const palette = selected?.palette ?? cover.palette

  return (
    <div className="space-y-6">
      <Link
        to="/covers"
        className="inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
      >
        <ArrowLeft aria-hidden="true" className="size-4" />
        All covers
      </Link>

      <div className="grid gap-6 md:grid-cols-[minmax(0,22rem)_1fr] md:items-start">
        <div className="space-y-3">
          <CoverImage cover={cover} imageUrl={imageUrl} working={working} />
          <RevisionStepper runs={runs} index={index} onSelect={select} />
        </div>

        <div className="space-y-6">
          <header className="space-y-2">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
              <span>{platformLabel(cover.platform)}</span>
              <span aria-hidden="true">·</span>
              <time dateTime={cover.createdAt}>{formatDate(cover.createdAt)}</time>
              <StatusBadge status={cover.status} live />
            </div>

            <h1 className="font-display text-2xl font-bold tracking-tight text-balance sm:text-3xl">
              {cover.playlistName}
            </h1>
          </header>

          {cover.status === 'failed' && (
            <p role="alert" className="text-sm text-destructive-text">
              {cover.error || 'Generation didn’t finish. Nothing was saved to the cover.'}
            </p>
          )}

          <RevisionList
            cover={cover}
            selectedID={selected?.id}
            showFailedRuns={showFailedRuns}
            onShowFailedRuns={onShowFailedRuns}
            onSelect={(run) => select(runs.findIndex((r) => r.id === run.id))}
          />

          <PaletteBreakdown palette={palette} />

          {/* gap-1 and px-2.5 rather than the default gap-2/px-3: the three
              buttons need 364px at those defaults and this row gets as little as
              350 on a phone, which dropped Set cover onto a second line.
              Tightened here rather than in the sm button variant, which every
              other surface shares. */}
          <div className="flex flex-wrap items-center gap-1 border-t border-border pt-5">
            {imageUrl && (
              // The label stays "Download" on every revision. Widening it to
              // "Download #23" reflowed this row on each step, dropping Delete
              // onto a line of its own; the stepper directly above already says
              // which run this is, and the file it saves carries the number.
              <a
                href={imageUrl}
                download={
                  selected && index > 0
                    ? `aurify-${cover.playlistName}-${selected.number}.png`
                    : `aurify-${cover.playlistName}.png`
                }
                target="_blank"
                rel="noreferrer"
                className={cn(buttonVariants({ variant: 'outline', size: 'sm' }), 'px-2.5')}
              >
                <Download aria-hidden="true" className="size-4" />
                Download
              </a>
            )}

            {/* One run at a time per playlist, which the API enforces with a
                409. Disabling while this one is in flight is what keeps that
                from being something the user discovers by hitting it. */}
            <Button
              variant="outline"
              size="sm"
              className="px-2.5"
              loading={regenerate.isPending}
              disabled={working}
              onClick={() =>
                regenerate.mutate(
                  { platform: cover.platform, playlistId: cover.playlistId },
                  { onSuccess: () => void navigate({ to: '/covers' }) },
                )
              }
            >
              <RefreshCw aria-hidden="true" className="size-4" />
              Regenerate
            </Button>

            {/* Only YouTube Music can take a cover back today: Spotify's API can
                but its provider is still a stub, and Apple Music exposes
                playlist artwork as read-only. Hiding the button beats a 422 the
                user can do nothing about.

                "Set cover" rather than "Set as playlist cover" for width, with
                the rest of the sentence in the accessible name, which keeps the
                visible text as a substring so it satisfies label-in-name. */}
            {cover.platform === 'youtube_music' && (
              <Button
                variant="outline"
                size="sm"
                className="px-2.5"
                aria-label="Set cover on YouTube Music"
                loading={push.isPending}
                disabled={working || !cover.imageUrl}
                onClick={() =>
                  push.mutate(cover.id, {
                    onSuccess: () => toast.success('Set as the playlist cover on YouTube Music'),
                    onError: (err) =>
                      toast.error(isApiError(err) ? err.detail || err.title : 'Could not set the playlist cover'),
                  })
                }
              >
                <ListMusic aria-hidden="true" className="size-4" />
                Set cover
              </Button>
            )}
          </div>

          {regenerate.isError && (
            <p role="alert" className="text-sm text-destructive-text">
              {errorText(regenerate.error, 'That didn’t work. Try again in a moment.')}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

/**
 * Chevron, label, chevron, directly under the artwork it moves.
 *
 * Walks successful runs only: a failed one has no image, so stepping onto it
 * would empty the frame this sits beneath. Failed runs are in the list instead,
 * where an error message is the useful thing to show.
 */
function RevisionStepper({
  runs,
  index,
  onSelect,
}: {
  runs: CoverRevision[]
  index: number
  onSelect: (index: number) => void
}) {
  // Nothing to step through until a second run exists.
  if (runs.length < 2) return null
  const run = runs[index]

  return (
    <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-stretch gap-1 rounded-xl border border-border bg-card p-1">
      <Button
        variant="ghost"
        size="sm"
        className="h-auto px-2.5"
        disabled={index === 0}
        aria-label="Newer revision"
        onClick={() => onSelect(index - 1)}
      >
        <ChevronLeft aria-hidden="true" className="size-4" />
      </Button>

      <span
        // Announced rather than silently swapped: the artwork changing is the
        // visible feedback, and a screen reader gets none of it.
        role="status"
        aria-live="polite"
        className="flex min-w-0 flex-col items-center justify-center px-1 py-1.5 text-center"
      >
        <span className="truncate text-sm font-semibold tabular-nums">
          {index === 0 ? 'Current' : `Revision #${run.number}`}
        </span>
        <time dateTime={run.completedAt} className="text-xs text-muted-foreground tabular-nums">
          {formatDateTime(run.completedAt)}
        </time>
      </span>

      <Button
        variant="ghost"
        size="sm"
        className="h-auto px-2.5"
        disabled={index === runs.length - 1}
        aria-label="Older revision"
        onClick={() => onSelect(index + 1)}
      >
        <ChevronRight aria-hidden="true" className="size-4" />
      </Button>
    </div>
  )
}

/**
 * Every generation this playlist has had, newest first, as a scrollable list of
 * thumbnails: the second way into the history, for jumping rather than stepping.
 *
 * It sits where the prompt used to be. The prompt is internal now, and a row of
 * small renders says more about what changed between runs than its text did.
 */
function RevisionList({
  cover,
  selectedID,
  showFailedRuns,
  onShowFailedRuns,
  onSelect,
}: {
  cover: Cover
  selectedID?: string
  showFailedRuns: boolean
  onShowFailedRuns: (show: boolean) => void
  onSelect: (run: CoverRevision) => void
}) {
  const runs = cover.revisions ?? []
  const del = useDeleteCoverRevision(cover.id)
  const [confirming, setConfirming] = useState<string | null>(null)
  const listRef = useRef<HTMLUListElement>(null)
  const current = runs.find((r) => r.status === 'ready')

  // The chevrons can step past the visible window, so the list follows the
  // selection. Nudges the list's own scrollTop rather than calling
  // scrollIntoView, which walks every ancestor and would jump the page.
  useEffect(() => {
    const list = listRef.current
    const row = list?.querySelector('[aria-current="true"]')
    if (!list || !row) return
    const listBox = list.getBoundingClientRect()
    const rowBox = row.getBoundingClientRect()
    if (rowBox.top < listBox.top) list.scrollTop -= listBox.top - rowBox.top
    else if (rowBox.bottom > listBox.bottom) list.scrollTop += rowBox.bottom - listBox.bottom
  }, [selectedID, showFailedRuns])

  // Nothing has finished yet: the toggle would reveal nothing either, so the
  // whole section stays out of the way until there is a history to have.
  if (runs.length === 0 && !showFailedRuns) return null

  return (
    <section className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          Revisions{cover.runCount > 0 && ` · ${cover.runCount} successful`}
        </h2>
        <Button
          variant="ghost"
          size="sm"
          className="text-muted-foreground"
          aria-pressed={showFailedRuns}
          onClick={() => onShowFailedRuns(!showFailedRuns)}
        >
          {showFailedRuns ? 'Hide failed runs' : 'Show failed runs'}
        </Button>
      </div>

      {runs.length === 0 ? (
        <p className="text-sm text-muted-foreground">No runs to show yet.</p>
      ) : (
        <Card className="p-1">
          <ul ref={listRef} className="max-h-80 space-y-1 overflow-y-auto">
            {runs.map((run) => {
              // A failed run has no artwork, so there is nothing to select and
              // the row is not a control. Rendering it as a disabled button
              // wrapped its error message, the one useful thing on the row, in
              // an element that announces itself as unavailable.
              const selectable = run.status === 'ready'
              const body = (
                <>
                  <ImageWithFallback
                    src={run.imageUrl}
                    // Decorative: the number and status beside it identify the run.
                    alt=""
                    loading="lazy"
                    className="size-11 shrink-0 rounded-md object-cover"
                    fallback={
                      <span
                        aria-hidden="true"
                        className="flex size-11 shrink-0 items-center justify-center rounded-md bg-muted"
                      >
                        <AlertTriangle className="size-4 text-muted-foreground" />
                      </span>
                    }
                  />

                  <span className="min-w-0 flex-1 space-y-0.5">
                    <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                      <span className="text-sm font-semibold tabular-nums">Revision #{run.number}</span>
                      {/* The shared Badge, not a one-off. Its own tint over the
                          selected row's stacked two primary washes behind
                          primary text, which measured 4.09:1 and missed AA. */}
                      {run.id === current?.id && <Badge variant="secondary">Current</Badge>}
                      {run.status === 'failed' && <StatusBadge status={run.status} />}
                    </span>
                    {/* Every run shows the same two facts, whatever its outcome.
                        A failed one's error used to sit here, where its length
                        set the row's shape; it is behind the button instead. */}
                    <time dateTime={run.completedAt} className="block text-xs text-muted-foreground tabular-nums">
                      {formatDateTime(run.completedAt)}
                    </time>
                  </span>
                </>
              )

              return (
                <li
                  key={run.id}
                  className={cn(
                    'flex items-center gap-1 rounded-lg border border-transparent pr-1.5 transition-colors',
                    run.id === selectedID ? 'border-primary/30 bg-primary/10' : selectable && 'hover:bg-muted',
                  )}
                >
                  {selectable ? (
                    <button
                      type="button"
                      aria-current={run.id === selectedID ? 'true' : undefined}
                      onClick={() => onSelect(run)}
                      className="flex min-w-0 flex-1 items-center gap-3 rounded-lg p-1.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      {body}
                    </button>
                  ) : (
                    <span className="flex min-w-0 flex-1 items-center gap-3 p-1.5">{body}</span>
                  )}

                  {/* A sibling of the select button, not inside it: a button
                      cannot contain a button. */}
                  <span className="flex shrink-0 items-center">
                    {run.status === 'failed' && <RunErrorPopover run={run} />}
                    {confirming === run.id ? (
                      <span className="inline-flex items-center gap-1">
                        <Button
                          variant="outline"
                          size="sm"
                          className="border-destructive/40 text-destructive hover:bg-destructive/10"
                          loading={del.isPending}
                          onClick={() => del.mutate(run.id, { onSuccess: () => setConfirming(null) })}
                        >
                          Delete
                        </Button>
                        <Button variant="ghost" size="sm" disabled={del.isPending} onClick={() => setConfirming(null)}>
                          Cancel
                        </Button>
                      </span>
                    ) : (
                      // Visible at rest rather than revealed on hover. Touch has
                      // no hover, so the previous opacity-0 left a control that
                      // could not be seen but could still be tapped.
                      <Button
                        variant="ghost"
                        size="icon"
                        className="text-muted-foreground hover:text-destructive"
                        onClick={() => setConfirming(run.id)}
                      >
                        <Trash2 aria-hidden="true" className="size-4" />
                        <span className="sr-only">Delete revision {run.number}</span>
                      </Button>
                    )}
                  </span>
                </li>
              )
            })}
          </ul>
        </Card>
      )}

      {del.isError && (
        <p role="alert" className="text-sm text-destructive-text">
          {errorText(del.error, 'That run couldn’t be deleted. Try again in a moment.')}
        </p>
      )}
    </section>
  )
}

/**
 * A failed run's error, behind an info button.
 *
 * The native `popover` attribute rather than a portal, because it does the three
 * hard parts itself: the panel goes in the top layer, so the revision list's
 * `overflow-y` cannot clip it; clicking outside dismisses it; and Escape closes
 * it. Only placement is ours, since CSS anchor positioning is not available
 * broadly enough to rely on yet.
 *
 * The error lives here rather than in the row because its length was setting the
 * row's shape, and a run that failed should still read as a revision.
 */
function RunErrorPopover({ run }: { run: CoverRevision }) {
  const panelID = `run-error-${run.id}`
  const panel = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    const el = panel.current
    if (!el) return

    // Anchored with right/bottom rather than left/top, so the panel's own size
    // is never needed. That is what lets this run in `beforetoggle`, while the
    // panel is still display:none and unmeasurable. Placing on `toggle` instead
    // paints it at 0,0 for a frame and then jumps it ~200px into position,
    // because that event is queued rather than dispatched synchronously.
    const place = () => {
      const btn = trigger.current
      if (!btn) return
      const anchor = btn.getBoundingClientRect()
      el.style.right = `${Math.max(8, window.innerWidth - anchor.right)}px`

      // Flip above when the space below is too tight to be worth using. The
      // panel's max-height keeps either direction inside the viewport.
      if (window.innerHeight - anchor.bottom < 180) {
        el.style.top = 'auto'
        el.style.bottom = `${window.innerHeight - anchor.top + 6}px`
      } else {
        el.style.bottom = 'auto'
        el.style.top = `${anchor.bottom + 6}px`
      }
    }

    // A fixed panel does not travel with the list it is anchored to, so any
    // scroll dismisses it rather than leaving it stranded beside nothing.
    const hide = () => {
      if (el.matches(':popover-open')) el.hidePopover()
    }

    const onBeforeToggle = (event: Event) => {
      const open = (event as ToggleEvent).newState === 'open'
      if (open) place()
      const bind = open ? window.addEventListener : window.removeEventListener
      bind('scroll', hide, true)
      bind('resize', hide)
    }

    el.addEventListener('beforetoggle', onBeforeToggle)
    return () => {
      el.removeEventListener('beforetoggle', onBeforeToggle)
      window.removeEventListener('scroll', hide, true)
      window.removeEventListener('resize', hide)
    }
  }, [])

  return (
    <>
      {/* A plain button with the shared variants, as the download link does:
          <Button> takes no ref, and placement needs the trigger's rect. */}
      <button
        ref={trigger}
        type="button"
        popoverTarget={panelID}
        className={buttonVariants({ variant: 'ghost', size: 'icon', className: 'text-muted-foreground' })}
      >
        <Info aria-hidden="true" className="size-4" />
        <span className="sr-only">Why revision {run.number} failed</span>
      </button>

      <div
        ref={panel}
        id={panelID}
        popover="auto"
        // inset-auto and m-0 undo the user-agent styles that would otherwise
        // centre this in the viewport.
        className="inset-auto m-0 max-h-60 w-72 max-w-[calc(100vw-1rem)] overflow-y-auto rounded-lg border border-border bg-card p-3 shadow-lg"
      >
        <p className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          Revision #{run.number} failed
        </p>
        <p className="mt-1 text-xs text-destructive-text [overflow-wrap:anywhere]">
          {run.error || 'This run didn’t finish, and recorded no reason.'}
        </p>
      </div>
    </>
  )
}

function CoverImage({ cover, imageUrl, working }: { cover: Cover; imageUrl?: string; working: boolean }) {
  return (
    <Card className="overflow-hidden">
      <ImageWithFallback
        src={imageUrl}
        alt={`Cover generated for ${cover.playlistName}`}
        className="aspect-square w-full object-cover"
        fallback={
          <div aria-hidden="true" className="flex aspect-square w-full items-center justify-center bg-muted">
            {cover.status === 'failed' ? (
              <AlertTriangle className="size-10 text-muted-foreground" />
            ) : (
              <CoverGlyph working={working} className="size-10" />
            )}
          </div>
        }
      />
    </Card>
  )
}

/** Every dimension in the derived palette, as a labeled weight bar. */
function PaletteBreakdown({ palette }: { palette?: ColorWeight[] }) {
  if (!palette || palette.length === 0) return null
  const sorted = [...palette].sort((a, b) => b.weight - a.weight)

  return (
    <section className="space-y-2">
      <h2 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">Palette</h2>
      <ul className="space-y-2">
        {sorted.map((color) => (
          <li key={color.dimension} className="flex items-center gap-3">
            <span
              aria-hidden="true"
              className="size-4 shrink-0 rounded-full ring-1 ring-border"
              style={{ backgroundColor: color.hexColor }}
            />
            <span className="w-28 shrink-0 text-sm font-medium capitalize">{color.dimension}</span>
            <span className="h-2 flex-1 overflow-hidden rounded-full bg-muted">
              <span
                className="block h-full rounded-full"
                style={{ width: `${Math.round(color.weight * 100)}%`, backgroundColor: color.hexColor }}
              />
            </span>
            <span className="w-10 shrink-0 text-right text-sm tabular-nums text-muted-foreground">
              {Math.round(color.weight * 100)}%
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}

function CoverDetailSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-5 w-24" />
      <div className="grid gap-6 md:grid-cols-[minmax(0,22rem)_1fr]">
        <Skeleton className="aspect-square w-full rounded-xl" />
        <div className="space-y-4">
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-8 w-3/4" />
          <div className="space-y-2 pt-4">
            {Array.from({ length: 4 }, (_, i) => (
              <Skeleton key={i} className="h-4 w-full" />
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

/** Runs of the same playlist often share a day, so the history needs the clock. */
function formatDateTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
}

function errorText(error: unknown, fallback: string): string {
  return isApiError(error) ? (error.detail ?? fallback) : fallback
}
