import { createFileRoute } from '@tanstack/react-router'

import { CoverGallery } from '@/features/covers/cover-gallery'

export const Route = createFileRoute('/covers')({
  component: CoversPage,
})

function CoversPage() {
  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-semibold tracking-tight">Your covers</h1>
      <CoverGallery />
    </section>
  )
}
