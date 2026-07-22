import { createFileRoute } from '@tanstack/react-router'

import { PageHeader } from '@/components/page-header'
import { CoverGallery } from '@/features/covers/cover-gallery'

export const Route = createFileRoute('/covers/')({
  component: CoversPage,
})

function CoversPage() {
  return (
    <section className="space-y-8">
      <PageHeader title="Your covers" description="Every cover you generate is saved here." />
      <CoverGallery />
    </section>
  )
}
