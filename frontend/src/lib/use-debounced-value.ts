import { useEffect, useState } from 'react'

/**
 * Returns a copy of `value` that only updates after it has stopped changing for
 * `delayMs`. Used to keep a fast-changing input (a search box) from firing a
 * server request on every keystroke.
 */
export function useDebouncedValue<T>(value: T, delayMs = 300): T {
  const [debounced, setDebounced] = useState(value)

  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delayMs)
    return () => clearTimeout(id)
  }, [value, delayMs])

  return debounced
}
