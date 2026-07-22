import { Compass, TriangleAlert } from 'lucide-react'
import { Link, type ErrorComponentProps } from '@tanstack/react-router'

import { EmptyState } from './empty-state'
import { Button } from './ui/button'
import { buttonVariants } from './ui/button-variants'

// Rendered by the router when no route matches the URL.
export function NotFound() {
  return (
    <section className="py-10">
      <EmptyState
        icon={Compass}
        title="Page not found"
        description="That page doesn’t exist — it may have moved, or the link was mistyped."
        action={
          <Link to="/" className={buttonVariants({ size: 'sm' })}>
            Back home
          </Link>
        }
      />
    </section>
  )
}

// Rendered by the router when a route throws while rendering. `reset` retries
// the failed boundary; the message is kept generic so a raw exception never
// leaks to the user.
export function RouteError({ reset }: ErrorComponentProps) {
  return (
    <section className="py-10">
      <EmptyState
        icon={TriangleAlert}
        title="Something went wrong"
        description="An unexpected error interrupted this page. You can try again, or head back home."
        action={
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={reset}>
              Try again
            </Button>
            <Link to="/" className={buttonVariants({ variant: 'ghost', size: 'sm' })}>
              Back home
            </Link>
          </div>
        }
      />
    </section>
  )
}
