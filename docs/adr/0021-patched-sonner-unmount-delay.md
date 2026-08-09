# 0021 — Sonner is patched so toast exits can finish

- Status: Accepted
- Date: 2026-08-09

## Context

Cover toasts vanished on a hard cut ([#76](https://github.com/phillipjad/Aurify/issues/76)).
Two separate causes, and only the first is ours:

The reduced-motion reset in `styles.css` forces `transition-duration: 0.01ms`
on everything, so for anyone who asks for reduced motion the toast disappears
between two frames.

The second is sonner's. Its removal rule transitions transform and opacity over
400ms, but `deleteToast` unmounts the node after `TIME_BEFORE_UNMOUNT`, which
ships as 200. Measured in the running app with motion enabled:

```
110ms  opacity 0.68, fallen 23px
182ms  opacity 0.32, fallen 49px
219ms  node removed from the DOM
```

The card was taken away a third lit and two thirds of the way down. Sonner is on
2.0.7, which is the latest release; the constant is module scoped, has no prop,
and no newer version changes it.

## Decision

**Patch the constant to 500ms** (`frontend/patches/sonner@2.0.7.patch`, recorded
in `pnpm-workspace.yaml`), and give the toast a 480ms sink: accelerating
downward with a slight shrink, opacity reaching zero at 360ms so the card is
already invisible for the last of the travel.

Sonner's own comment on the constant reads "Equal to exit animation duration",
so it is meant to move with the exit rather than to cap it. Raising it is using
the knob as intended, through the only mechanism the package exposes.

The alternative was to keep every exit under 200ms. That works, and three of the
four sampled exits did exactly that, but it makes the ceiling the designer
rather than the design.

Rejected: owning all three dismissal paths ourselves (our own timer instead of
`duration`, our own close button instead of `closeButton`) to delay sonner's
dismissal. It reaches the same place having reimplemented pause-on-hover,
close-button placement and focus handling, and swipe-to-dismiss would still take
sonner's own path and disagree.

## Consequences

- `vp install` runs pnpm underneath, so local and CI apply the patch alike. An
  upgrade that moves sonner off 2.0.7 fails the install rather than silently
  dropping the patch.
- A dismissed toast now sits in the DOM, invisible, for half a second, where it
  used to sit for a fifth of one. That window is retired with `pointer-events:
  none` so it cannot swallow a click, plus a `visibility: hidden` whose zero
  length transition is delayed to the end of the fade, which takes it out of the
  tab order and the accessibility tree the moment it stops being visible. Without
  the second one a keyboard user could tab onto the close button of a card that
  was no longer on screen.
- Entering the covers section retires the stack 250ms apart rather than at once,
  oldest first. On a bottom-anchored stack that is top down, and a toast's offset
  is measured from the cards in front of it, so nothing has to move over while
  its neighbour is still leaving. A full stack of five takes about 1.5s to clear.
- Reduced motion gets its own treatment rather than the sink: opacity only, no
  travel and no shrink, over 320ms. A dismissal is not a busy indicator, so the
  exemption that covers spinners ([ADR 0019](0019-async-cover-generation.md) and
  the `data-motion` rules) does not extend to it on the same reasoning. It is
  exempt because a notification that disappears between frames reads as a
  glitch, and it is slower than the motion version because a fade with nothing
  else moving needs the time to register as a departure.
- The exit styling lives in `styles.css` rather than the `classNames` in
  `components/ui/sonner.tsx`: it works by extending sonner's `--y` custom
  property, which carries the stacking offset of every toast behind the front
  one, and that is not expressible as a utility class.
