import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// jsdom doesn't implement scrollTo; TanStack Router calls it on navigation, and
// so does the root layout when it resets <main> between routes.
Object.defineProperty(window, 'scrollTo', { value: () => {}, writable: true })
Object.defineProperty(Element.prototype, 'scrollTo', { value: () => {}, writable: true })

// jsdom has no layout, so every element measures zero. That is harmless for row
// heights (the virtualizers fall back to their estimate) but fatal for the
// scroll container: a zero-height viewport means no row is ever in view, and the
// lists render empty. Give that one element a browser-sized box.
// offsetWidth/offsetHeight specifically, because that is the pair the
// virtualizer sizes its viewport from (virtual-core's getRect). Row heights are
// unaffected: those still fall back to each list's own estimate.
//
// Keyed on data-scroll-container, which the app marks every scroll container
// with, so a page that scrolls a region of itself (the covers grid) measures
// like the shell's own scroller does.
const VIEWPORT = { offsetWidth: 1024, offsetHeight: 768 }
for (const [prop, value] of Object.entries(VIEWPORT)) {
  Object.defineProperty(HTMLElement.prototype, prop, {
    configurable: true,
    get(this: HTMLElement) {
      return this.hasAttribute('data-scroll-container') ? value : 0
    },
  })
}

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
