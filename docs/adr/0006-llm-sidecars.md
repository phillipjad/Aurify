# 0006 — Local LLM sidecars for prompt + image generation

- Status: Proposed
- Date: 2026-05-30

## Context

The final step of the pipeline turns a `PlaylistAnalysis` into (1) an
image-generation prompt and (2) an abstract image. The product calls for *small,
local* models for both, run as separate processes ("sidecars") rather than
embedded in the Go API or called as third-party cloud APIs.

## Decision

Model each as an HTTP sidecar behind a port:

- `ports.PromptGenerator` → `internal/platform/llm/promptgen` posts the analysis
  to `AURIFY_PROMPTGEN_URL/generate` and expects `{ "prompt": "..." }`.
- `ports.ImageGenerator` → `internal/platform/llm/imagegen` posts the prompt to
  `AURIFY_IMAGEGEN_URL/generate` and expects `{ "imageUrl": "..." }`.

The sidecar **services themselves are intentionally not built in this scaffold.**
When the corresponding URL is unset, each client returns a deterministic
placeholder (a prompt assembled from the dominant palette dimensions; a
stand-in image URL) so the end-to-end pipeline stays runnable during development.

## Consequences

- The Go API stays language-agnostic about the models; sidecars can be Python,
  Rust, etc., and scale/deploy independently.
- Clear, small HTTP contracts make the sidecars easy to stub, swap, and test.
- Open questions for a follow-up ADR: model selection and hosting; how generated
  images are stored (object storage vs. data URLs) and where `imageUrl` points;
  synchronous vs. queued generation (today the pipeline runs synchronously —
  see ADR 0003); timeouts/retries and backpressure.
