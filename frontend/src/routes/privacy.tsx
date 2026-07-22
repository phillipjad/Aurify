import { createFileRoute } from '@tanstack/react-router'
import { ShieldCheck } from 'lucide-react'

import { EmptyState } from '@/components/empty-state'

// SCAFFOLD: content page not written yet. Footer links here.
export const Route = createFileRoute('/privacy')({
  component: () => <EmptyState icon={ShieldCheck} title="Privacy Policy" description="This page is coming soon." />,
})
