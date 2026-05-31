# 0001 — Record architecture decisions

- Status: Accepted
- Date: 2026-05-30

## Context

Aurify is a greenfield, polyglot project (Go API + React PWA) with several
non-obvious choices (CQRS depth, an abstraction over multiple music platforms,
local LLM sidecars). Future contributors — human and AI — need to understand
*why* the structure is the way it is, not just *what* it is.

## Decision

We keep short ADRs in `docs/adr/`, numbered sequentially, in MADR style
(Context / Decision / Consequences). Each significant or hard-to-reverse
decision gets one. Code comments point back to the relevant ADR where useful.

## Consequences

- A small, ongoing documentation cost per decision.
- New contributors can reconstruct intent without archaeology.
- ADRs are immutable once Accepted; we supersede rather than rewrite.
