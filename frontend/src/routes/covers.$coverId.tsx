import { createFileRoute } from '@tanstack/react-router'

import { CoverDetail } from '@/features/covers/cover-detail'

/** `?rev=` is the revision being viewed. Absent means the current artwork. */
export type CoverDetailSearch = {
  rev?: number
}

export const Route = createFileRoute('/covers/$coverId')({
  // Narrowed rather than trusted: a URL is typed by anyone. An unusable value
  // becomes undefined here, and the page falls back to the current revision
  // rather than erroring, which is also what happens to a number naming a run
  // that has since been deleted.
  validateSearch: (search: Record<string, unknown>): CoverDetailSearch => {
    const rev = Number(search.rev)
    return Number.isInteger(rev) && rev > 0 ? { rev } : {}
  },
  component: CoverDetailPage,
})

function CoverDetailPage() {
  const { coverId } = Route.useParams()
  return (
    <section className="mx-auto max-w-3xl">
      <CoverDetail coverId={coverId} />
    </section>
  )
}
