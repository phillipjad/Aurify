import { createFileRoute, Outlet } from '@tanstack/react-router'

import { requireSession } from '@/lib/auth-guard'

// Layout for the /covers subtree: the gallery (index) and the detail view
// ($coverId) both render through this Outlet. Guarding here covers both, so a
// new child route cannot be added without one.
export const Route = createFileRoute('/covers')({
  beforeLoad: ({ context, location }) => requireSession(context.queryClient, location.href),
  component: CoversLayout,
})

function CoversLayout() {
  return <Outlet />
}
