import { createFileRoute } from '@tanstack/react-router'
import { Mail } from 'lucide-react'

import { EmptyState } from '@/components/empty-state'

// SCAFFOLD: content page not written yet. Footer links here.
export const Route = createFileRoute('/contact')({
  component: () => <EmptyState icon={Mail} title="Contact" description="This page is coming soon." />,
})
