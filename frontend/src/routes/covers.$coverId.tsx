import { createFileRoute } from '@tanstack/react-router'

import { CoverDetail } from '@/features/covers/cover-detail'

export const Route = createFileRoute('/covers/$coverId')({
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
