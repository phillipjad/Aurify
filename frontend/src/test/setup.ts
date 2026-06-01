import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// jsdom doesn't implement scrollTo; TanStack Router calls it on navigation.
Object.defineProperty(window, 'scrollTo', { value: () => {}, writable: true })

// Globals are disabled in vitest.config.ts, so unmount React trees between tests
// explicitly (Testing Library's auto-cleanup relies on a global afterEach).
afterEach(() => {
  cleanup()
})
