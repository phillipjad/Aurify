import { useLayoutEffect, useState, type RefObject } from 'react'

/**
 * The app shell bounds itself to the viewport, so the document never scrolls:
 * <main> is the one scroll container (see routes/__root.tsx). Anything that
 * needs to measure against "the scroll area" means that element, not the window.
 */
export const APP_SCROLL_ID = 'main'

export function getAppScrollElement(): HTMLElement | null {
  return document.getElementById(APP_SCROLL_ID)
}

/**
 * Distance from the top of the scroll container's content down to `ref`, which
 * is what a virtualizer wants as its `scrollMargin` when the list starts partway
 * down a longer page.
 *
 * Re-read after every render rather than on a dependency list: the toolbars
 * above these lists change height as filters and connection state resolve, and
 * a stale margin puts every row in the wrong place. Written only when it
 * actually moved, so this settles instead of looping.
 */
export function useScrollMargin(ref: RefObject<HTMLElement | null>): number {
  const [margin, setMargin] = useState(0)

  useLayoutEffect(() => {
    const el = ref.current
    const scroller = getAppScrollElement()
    if (!el || !scroller) return

    // Rect difference plus scrollTop, not offsetTop: offsetTop is measured from
    // the nearest positioned ancestor, which is not the scroller.
    const next = el.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop
    if (next !== margin) setMargin(next)
  })

  return margin
}
