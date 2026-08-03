# 0016 — Generation providers: real models, and images in PostgreSQL

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

**Image generation calls Cloudflare Workers AI, on a non-FLUX model.** Unlike
promptgen this adapter is provider-shaped, because Workers AI serves OpenAI
compatibility for text only. That is a real cost, accepted for a specific
reason: Workers AI is the only image provider found that is free without a card,
a deposit or an expiry, and being able to run the pipeline at all outranks the
adapter being portable.

The constraint that decided it was money, and the search was narrower than
expected:

| Provider | Free without a card | Why not |
|---|---|---|
| Workers AI, FLUX | Yes, 10,000 neurons/day | Safety classifier, see below |
| Workers AI, non-FLUX | Yes, same allowance | **Chosen** |
| Together AI, FLUX.1 [schnell] | No longer | $5 deposit |
| Gemini 2.5 Flash Image | No | Paid only, $0.039/image |

**The model is deliberately not FLUX.** Cloudflare's FLUX endpoints refused 2 of
8 measured generations with "Input prompt contains NSFW content", on text like
"Deep indigo nebula swirls engulfing bursts of radiant gold, creating an
unsettling beauty". The refusal tracks wording rather than content, so the same
playlist passes on a retry. There is no parameter to disable or tune it, the
request to add one has been open since October 2024, and public reports include
the single word "hamburger" being refused. That error, code 3030, is documented
only against the FLUX endpoints, so the same account and allowance work through
`@cf/leonardo/lucid-origin` instead. Measured on the two playlists FLUX refused:
**8 generations, 0 refusals**, against FLUX's 2 in 8.

The account id is configured on its own and the endpoint is assembled in code.
Everything but the account and model is fixed, so asking for the whole URL would
only create somewhere to typo it, which is how an earlier OAuth redirect URI lost
an afternoon to a trailing character.

Workers AI answers in two shapes depending on the model: newer ones wrap base64
in Cloudflare's JSON envelope, the Stable Diffusion ones stream the image. The
adapter switches on the response content type rather than the model name, so
trying another model stays a configuration change. That matters more than usual
here, since model-shopping is how this provider was made usable.

**`ImageGenerator` returns bytes, not a URL.** Image APIs return the image
itself, or a link that expires within the hour. A new `ports.ImageStore` decides
where it lives and returns the URL that serves it.

**Generated images are stored in PostgreSQL, not GCS.** ADR 0014 chose a
public-read bucket; this defers that until there is a deployment to put it in,
because a bucket requires a cloud account before the pipeline can be run at all.
`cover_images` is a table of its own so the gallery's list query never reads
image bytes, and `bytes` is `STORAGE EXTERNAL` because JPEG will not compress
further.

The ceiling is real and closer than expected. Over 18 generations these models
return 1024x1024 JPEGs averaging **829KB** (585KB to 1094KB), not the ~150KB
assumed when this was drafted. That puts Neon's free 0.5GB at roughly 600 covers.
Moving to a bucket is a second `ports.ImageStore`, which is why the port returns
the URL rather than having callers build one.

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
- Both adapters still take the same three settings, a base URL, a model and a
  key. promptgen is portable across providers by API shape; imagegen is portable
  only across Workers AI models, and another vendor means a sibling adapter.
- A refused prompt still fails the generation. There is no retry: with a
  provider that does not refuse ordinary prompts there is nothing to retry, and
  adding one would have hidden exactly the signal that made Cloudflare's
  behaviour measurable.
- The prompt is recorded on the cover before the image is requested, so a
  refusal keeps the text that caused it. Assigning it afterwards discarded
  exactly the evidence needed to diagnose one.
- The image route must not set `Content-Length`. The router compresses, so a
  hand-set length describes the uncompressed body and no browser can load the
  result: Chrome aborts with `ERR_CONTENT_LENGTH_MISMATCH` while curl fetches it
  happily, because curl does not ask for gzip by default. A router test asserts
  the declared length against the compressed body.
- Covers still look alike, because the analysis feeding the prompt is
  near-constant for YouTube Music
  ([issue #68](https://github.com/phillipjad/Aurify/issues/68)). Real models do
  not fix a constant input.
- Reasoning models emit a `<think>` block inline when the provider does not
  split it into its own field, so the adapter strips one. `gemma3:4b` does not,
  but the base URL exists to be repointed.
