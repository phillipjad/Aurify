import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// jsdom doesn't implement scrollTo; TanStack Router calls it on navigation.
Object.defineProperty(window, 'scrollTo', { value: () => {}, writable: true })

// jsdom has no ResizeObserver, which the list virtualizer uses to watch rows. It
// never fires here (there is no layout to change), so a no-op is enough: rows
// keep their estimated height, which is what the virtualizer falls back to.
if (!('ResizeObserver' in globalThis)) {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver
}

// Globals are disabled in vitest.config.ts, so unmount React trees between tests
// explicitly (Testing Library's auto-cleanup relies on a global afterEach).
afterEach(() => {
  cleanup()
})
