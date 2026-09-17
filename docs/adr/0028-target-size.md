# 0028: Interactive targets are at least 44x44 CSS pixels

- Status: Accepted
- Date: 2026-09-16

## Context

WCAG 2.2 sets two target sizes: 24x24 CSS px at AA (2.5.8, Target Size
(Minimum)) and 44x44 at AAA (2.5.5, Target Size (Enhanced)). Aurify's controls
were 36, 40 and 44px tall, which met AA and nothing above it
([#87](https://github.com/phillipjad/Aurify/issues/87)).
[ADR 0027](0027-shared-ui-primitives.md) put every control's height on three
theme tokens, so the size is one decision.

## Decision

**Every pointer target is at least 44x44 CSS px, for every pointer type.**

**The control scale starts at that size and rises 4px per step.** In
`frontend/src/styles.css`:

| token | height | used by |
|---|---|---|
| `control-sm` | 44px | `sm` and `icon-sm` buttons, inputs, selects, segmented controls |
| `control` | 48px | default and `icon` buttons |
| `control-lg` | 52px | `lg` buttons |

A control takes its height from these tokens. A target that cannot grow
visibly grows its hit area instead: Sonner's close button is drawn at 24px, and
its `::after` extends the clickable area to 44px.

**Two exceptions, both defined by 2.5.5:**

- **Inline.** A link inside a sentence or a line of text takes that line's
  height: "Create one" and "Sign in" on the auth forms, "Reconnect" beside the
  connection status.
- **Equivalent.** "Cover ready, view it" on a playlist row leads to `/covers`,
  the same page as the header's Covers link, which meets the size.

## Consequences

- The header, footer and toolbars are taller. Control widths are unchanged, so
  rows that were fitted to a phone width still fit.
- `sm` and default buttons differ in height as well as padding.
- A new target smaller than 44x44 either fits one of the exceptions above, and is
  added to it, or grows its hit area the way the toast close button does.
