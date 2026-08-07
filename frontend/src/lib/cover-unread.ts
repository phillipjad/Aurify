// Covers that finished while the user was elsewhere, counted on the Covers nav
// link. The toast is the interruption; this is the record that outlives it.
// In memory only: a reload lands on a gallery that already shows every status.
const listeners = new Set<() => void>()
let unread = 0

function emit() {
  for (const listener of listeners) listener()
}

/** useSyncExternalStore shape. */
export function subscribeUnreadCovers(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** Covers announced but not yet looked at. */
export function getUnreadCoverCount(): number {
  return unread
}

export function markCoverUnread(): void {
  unread += 1
  emit()
}

/** Called on reaching the covers section, where they can see them. */
export function clearUnreadCovers(): void {
  if (unread === 0) return
  unread = 0
  emit()
}
