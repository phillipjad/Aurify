# 0027: Controls come from shared primitives, and lint enforces it

- Status: Accepted
- Date: 2026-09-16

## Context

A design decision should land in one place. Deciding on a 44px target size
([#87](https://github.com/phillipjad/Aurify/issues/87)) showed it could not:
feature code hand-built its own controls and states, so the app's size, focus,
hover and elevation lived at each call site
([#130](https://github.com/phillipjad/Aurify/issues/130)).

## Decision

**Every control is a primitive in `frontend/src/components/ui`.** They are
shadcn components on Radix, restyled on the app's own tokens: `Button` (with
`asChild` for links), `Input`, `Label`, `NativeSelect`, `SegmentedControl`,
`Popover`, `Switch`, `Separator`, `Alert`, `Card`, `Swatch` and `SkipLink`.
Radix owns keyboard, focus and dismissal behavior. Feature code composes
primitives and lays them out; it does not style states.

**The theme holds the values primitives share.** In `styles.css`:

- One `:focus-visible` outline for the whole app.
- Control heights as spacing tokens (`h-control-sm`, `h-control`, `h-control-lg`).
- Every radius derived from `--radius`.
- Registered utilities for the repeated text role (`eyebrow`) and the app's
  motion classes.

**`vp check` enforces the boundary.** Outside `components/ui`:

- `react/forbid-elements` rejects raw `button`, `a`, `input`, `select` and
  `textarea`.
- `better-tailwindcss/no-restricted-classes`, run through Oxlint `jsPlugins`,
  rejects state variants, ring and outline, shadow, and arbitrary values other
  than grid templates.
- `no-restricted-imports` rejects `button-variants`, so a button look cannot be
  applied without the component.

`better-tailwindcss/no-unknown-classes` applies everywhere, so a class the theme
does not define fails the build. An exception takes an
`oxlint-disable-next-line` comment on that line with its reason.

## Consequences

- Changing a token or a primitive changes every screen that shows it. #87 is a
  change to the three control height tokens.
- A new kind of control is added to `components/ui` first, then used.
- Radix is a runtime dependency (`radix-ui`), and Oxlint's JS plugins are alpha.
  A plugin failure shows up as a `vp check` failure, not as silent drift.
- Enabling Oxlint's `react` plugin, which `forbid-elements` needs, also turned on
  its React Compiler rules. Their warnings predate this change.
