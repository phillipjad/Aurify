import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'

import { Card } from '@/components/ui/card'

interface EmptyStateProps {
  icon: LucideIcon
  title: string
  description?: string
  /** Optional call-to-action shown beneath the message. */
  action?: ReactNode
}

export function EmptyState({ icon: Icon, title, description, action }: EmptyStateProps) {
  return (
    <Card className="flex flex-col items-center justify-center gap-3 border-dashed bg-transparent px-6 py-14 text-center shadow-none">
      <span
        aria-hidden="true"
        className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground"
      >
        <Icon className="size-6" />
      </span>
      <div className="space-y-1">
        <p className="font-display font-semibold text-balance">{title}</p>
        {description && <p className="mx-auto max-w-sm text-pretty text-sm text-muted-foreground">{description}</p>}
      </div>
      {action}
    </Card>
  )
}
