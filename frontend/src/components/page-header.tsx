import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: string
  description?: string
  /** Optional trailing control (e.g. a button) shown on the right. */
  action?: ReactNode
}

export function PageHeader({ title, description, action }: PageHeaderProps) {
  return (
    <header className="flex flex-wrap items-end justify-between gap-4">
      <div className="space-y-1.5">
        <h1 className="font-display text-3xl font-bold tracking-tight">
          {title}
        </h1>
        {description && (
          <p className="max-w-prose text-muted-foreground">{description}</p>
        )}
      </div>
      {action}
    </header>
  )
}
