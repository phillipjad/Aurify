# 0016 — Generation providers: an OpenAI-compatible base URL, and images in PostgreSQL

- Status: Accepted
- Date: 2026-08-03
- Amends: [0014](0014-hosted-generation-apis.md) (its provider-is-configuration
  decision is implemented here; its GCS storage decision is deferred)

## Context

[ADR 0014](0014-hosted-generation-apis.md) replaced the local sidecars of
[ADR 0006](0006-llm-sidecars.md) with hosted APIs called from the Go API, but
the adapters were never changed: they still posted to `{baseURL}/generate` and
expected `{"prompt": ...}` / `{"imageUrl": ...}`. Since neither URL was ever
set, both fell through to their placeholders and every cover in the gallery was
the same `placehold.co` image.

The pipeline needed to run for real against something free, before any decision
about a paid provider.

## Decision

**Prompt generation speaks the OpenAI chat-completions API.** The adapter posts
to `{baseURL}/chat/completions`, so the same code reaches Ollama on localhost,
Groq, OpenRouter, Cerebras and OpenAI. Base URL, model and API key are
configuration. This is what makes "the provider is configuration" from ADR 0014
true rather than aspirational: moving from a local model to a hosted one is a
change of environment variables.

The default is Ollama on the host with `gemma3:4b`. It runs natively rather than
as a compose service because Docker on macOS has no access to Metal, so a
containerized model would fall back to CPU. `docker-compose.yml` gains
`extra_hosts: host.docker.internal:host-gateway` so the containerized API can
reach it.

**Image generation calls Cloudflare Workers AI.** Chosen for its free allowance,
not its API: 10,000 neurons a day against `4 tiles x 4.8 + 4 steps x 9.6 = 57.6`
neurons for one FLUX.1 [schnell] image, so roughly 170 images a day at no cost.
Its request shape is Cloudflare's own, so unlike promptgen this adapter is
provider-specific and another provider means a sibling adapter behind the port.

**`ImageGenerator` returns bytes, not a URL.** Image APIs return the image
itself, or a link that expires within the hour. A new `ports.ImageStore` decides
where it lives and returns the URL that serves it.

**Generated images are stored in PostgreSQL, not GCS.** ADR 0014 chose a
public-read bucket. That is still the right answer at volume, but it requires a
cloud account before the pipeline can be run at all, and at roughly 150KB per
JPEG the volume does not justify one yet. `cover_images` is a table of its own so
the gallery's list query never reads image bytes, and `bytes` is
`STORAGE EXTERNAL` because JPEG and PNG will not compress further.

**`GET /api/v1/covers/{id}/image` is anonymous.** An `<img>` tag loading
cross-origin, the app on `:5173` and the API on `:8080`, does not send
credentials, so an authenticated route would render nothing. Cover ids are
UUIDv4, which is the same unguessable-name trade ADR 0014 already accepted for
its public bucket. Because `AllowAnonymous` makes mux skip the authentication
middleware entirely, the handler must not read the principal; a router test
covers the route serving without a session.

## Consequences

- Configuration replaces `AURIFY_PROMPTGEN_URL` and `AURIFY_IMAGEGEN_URL` with
  a base URL, model and key per provider. Both keep the "absent configuration
  means placeholder" path, so a fresh checkout still completes a generation.
- The image placeholder is now rendered locally as a PNG gradient seeded by the
  prompt, rather than a link to `placehold.co`. The bytes have to reach the
  store either way, and a remote URL meant development covers stopped rendering
  without a network.
- Storage grows with covers rather than sitting on a bucket. The upgrade path is
  a second `ports.ImageStore` implementation, which is why the port returns the
  URL rather than having callers build one.
- Covers still look alike, because the analysis feeding the prompt is
  near-constant for YouTube Music
  ([issue #68](https://github.com/phillipjad/Aurify/issues/68)). Real models do
  not fix a constant input.
- Reasoning models emit a `<think>` block inline when the provider does not
  split it into its own field, so the adapter strips one. `gemma3:4b` does not,
  but the base URL exists to be repointed.
