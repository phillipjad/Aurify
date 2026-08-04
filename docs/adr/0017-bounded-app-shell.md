# 0017 — The app shell is bounded to the viewport

- Status: Accepted
- Date: 2026-08-04

## Context

The layout set a floor on its own height and no ceiling: the root was
`min-h-dvh` with a flexible `<main>`, so the document grew with whatever the
route rendered and the window did the scrolling. The footer sat below that
growth. Measured on `/covers` signed in with 24 covers, at 1280x800 the document
was 3541px and the footer began at 3313px, four screens down. On a 390x844
phone it was 9433px and eleven screens.

Both list surfaces used `useWindowVirtualizer` for the same reason, recorded in
a comment in `playlist-browser.tsx`: an inner scroll container would have given
the page a second scrollbar and stranded the footer below it. That reasoning
held only while the document itself scrolled.

## Decision

**The root is a fixed-height grid, and `<main>` is the only scroll container.**
`h-dvh` with rows of `auto / minmax(0, 1fr) / auto`. The `0` minimum is the part
that matters: a `1fr` row still has an `auto` minimum size, so without it the
middle row grows to fit its content and the layout is unbounded again.

**The footer is a single row of links.** In a bounded shell the footer is on
screen on every route, so its height is subtracted from every page. The previous
one was 228px of security pitch plus links, which is 28% of an 800px viewport.
It is now about 44px: About, Privacy, Security, Contact, and the copyright. The
security paragraph moved to `/security`, which it already linked to, and that
page is now reachable directly from the footer rather than only through the
paragraph's heading.

**The virtualizers scroll against an element rather than the window.**
`useWindowVirtualizer` becomes `useVirtualizer` with `getScrollElement`. The
playlists list measures against `<main>`, found by id through
`lib/app-scroll.ts` rather than threaded down as a ref. The covers grid owns its
own scroller and passes that ref directly.

**A route may claim the shell and scroll a region of itself.** The covers route
does: its title and status filters stay put while only the grid moves. The route
marks its root with `data-fills-shell`, and `<main>` switches its grid row from
content-sized to `minmax(0, 1fr)` through `:has()`. Declaring it on the route
keeps the root layout free of a list of which routes are which, which is what
made the two-shell option in ADR 0017's alternatives expensive.

Two earlier attempts are worth recording because both look correct and neither
works. `min-h-full` on the wrapper sets a floor, not a definite height, so a
`flex-1` descendant has nothing to divide and the page scrolls as before.
`minmax(min-content, 1fr)` on the row fails for a subtler reason: a scroll
container still reports its full content height when an ancestor asks for its
min-content size, so the row resolved to the whole grid's height. Only a
definite row height bounds the chain.

**Scrollbars are styled once, on the `data-scroll-container` attribute.** Not
per surface, and not through a plugin. `tailwind-scrollbar` would have been the
obvious reach, but it produces per-element utility classes wrapping
`scrollbar-width` / `scrollbar-color` and the WebKit pseudo-elements, and this
app wants one scrollbar everywhere rather than a vocabulary for varying it. The
rule lives in `styles.css`: thin, a thumb mixed from `--muted-foreground` so it
follows the theme already defined, and a transparent track so the bar reads as
part of the surface rather than a channel cut into it.

Browsers that support `scrollbar-color` get the standard properties; Safari
before 18.2 gets `::-webkit-scrollbar-*` instead, under `@supports not`, because
Chrome ignores those pseudo-elements entirely once `scrollbar-color` is set and
specifying both would leave rules that look load-bearing and are not.

That rule also reserves the gutter (`scrollbar-gutter: stable`). Without it the
content width changes by the scrollbar's thickness when moving between a route
that overflows and one that does not, which re-wraps every balanced heading in
the middle of the route transition.

**The layout resets `main.scrollTop` on navigation.** An element keeps its
scroll position across a route change; the window used to be reset for us.

## Consequences

The header is no longer `sticky`, because it is a grid row that never scrolls.
Its `backdrop-blur` went with it: nothing passes behind it any more. The
translucent background stays so the aurora still tints it.

Anything added inside `<main>` inherits the bound. A page that wants its own
pinned chrome says so with `data-fills-shell` and a flex column whose scrolling
region carries `min-h-0 flex-1 overflow-y-auto`. Every scroll container in the
app is marked `data-scroll-container`, which is also how the test environment
knows which elements to give a size to.

The playlists page still scrolls the whole pane, so its title and platform tabs
scroll away where the covers page's do not. Worth reconciling, in whichever
direction; it is left inconsistent rather than changed unasked.

Tests needed the scroll container modelled. jsdom has no layout, so an
element-scoped virtualizer measures a zero-height viewport and renders no rows
at all, where the window one had `window.innerHeight` to work with. `test/setup.ts`
gives the element with that id a browser-sized `offsetWidth`/`offsetHeight`, and
`test/render.tsx` wraps feature components in it. Row heights are untouched and
still fall back to each list's own estimate.

`/` was the only route whose content was written assuming a page that scrolls
away. It fits the bound today; if the landing page grows, it scrolls inside
`<main>` like everything else rather than pushing the footer down.

## Alternatives considered

**Keep the full footer and bound only the shell.** One file changes and nothing
about the footer moves, but 289px of every viewport becomes permanent chrome,
about a row and a half of covers on a laptop.

**Bound the app routes and leave the marketing routes scrolling.** Gives each
surface the shape its job wants and keeps the long footer where it reads well,
at the cost of two layouts and a rule about which route is which. Worth
revisiting if the marketing pages grow past a screen; the split is easy to add
later, because it is a branch in one component.
