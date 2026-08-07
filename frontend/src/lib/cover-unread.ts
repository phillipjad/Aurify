// How many covers finished while the user was somewhere else and has not
// looked at since. The count rides the Covers nav link, which is what lets a
// toast be brief without being the only notice: the toast is the interruption,
// the badge is the record.
//
// A count rather than the ids, because the ids were never read: the caller
// already refuses to announce the same cover twice, so there is nothing here to
// deduplicate and nothing that needs to know which covers they were.
//
// Deliberately in memory only. A reload lands on a covers list that already
// shows every cover and its status, so persisting this would duplicate what the
// gallery itself says.
const listeners = new Set<() => void>()
let unread = 0

function emit() {
  for (const listener of listeners) listener()
}

/** Subscribe to changes (useSyncExternalStore shape). */
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

/** Called when the user reaches the covers section, where they can see them. */
export function clearUnreadCovers(): void {
  if (unread === 0) return
  unread = 0
  emit()
}
