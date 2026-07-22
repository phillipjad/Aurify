import { createFileRoute, Outlet } from '@tanstack/react-router'

// Layout for the /covers subtree: the gallery (index) and the detail view
// ($coverId) both render through this Outlet.
export const Route = createFileRoute('/covers')({
  component: CoversLayout,
})

function CoversLayout() {
  return <Outlet />
}
