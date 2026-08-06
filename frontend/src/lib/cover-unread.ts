// Covers that finished while the user was somewhere else and has not looked at
// since. The count rides the Covers nav link, which is what lets a toast be
// brief without being the only notice: the toast is the interruption, the badge
// is the record.
//
// Deliberately in memory only. A reload lands on a covers list that already
// shows every cover and its status, so persisting this would duplicate what the
// gallery itself says.
const listeners = new Set<() => void>()
let unread: readonly string[] = []

function emit() {
  for (const listener of listeners) listener()
}

/** Subscribe to changes (useSyncExternalStore shape). */
export function subscribeUnreadCovers(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** Current unread cover ids; the reference changes only when the set does. */
export function getUnreadCovers(): readonly string[] {
  return unread
}

export function markCoverUnread(id: string): void {
  if (unread.includes(id)) return
  unread = [...unread, id]
  emit()
}

/** Called when the user reaches the covers section, where they can see them. */
export function clearUnreadCovers(): void {
  if (unread.length === 0) return
  unread = []
  emit()
}
