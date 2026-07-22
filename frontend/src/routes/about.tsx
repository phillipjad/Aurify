import { createFileRoute } from '@tanstack/react-router'
import { Info } from 'lucide-react'

import { EmptyState } from '@/components/empty-state'

// SCAFFOLD: content page not written yet. Footer links here.
export const Route = createFileRoute('/about')({
  component: () => <EmptyState icon={Info} title="About Aurify" description="This page is coming soon." />,
})
