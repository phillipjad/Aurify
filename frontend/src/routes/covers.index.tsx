import { createFileRoute } from '@tanstack/react-router'

import { PageHeader } from '@/components/page-header'
import { CoverGallery } from '@/features/covers/cover-gallery'

export const Route = createFileRoute('/covers/')({
  component: CoversPage,
})

// data-fills-shell tells the root layout to give this route exactly one screen
// rather than as much height as it wants (see routes/__root.tsx); the gallery
// then scrolls inside itself and the title and filters stay put. min-h-0 is
// what allows a flex child to be shorter than its content, and so to scroll.
function CoversPage() {
  return (
    <section data-fills-shell className="flex min-h-0 flex-1 flex-col gap-8">
      <PageHeader title="Your covers" description="Every cover you generate is saved here." />
      <CoverGallery />
    </section>
  )
}
