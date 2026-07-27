# 0014 — Hosted APIs for prompt and image generation, called from the API

- Status: Accepted
- Date: 2026-07-24
- Supersedes: [0006](0006-llm-sidecars.md) (its port contracts are kept; its
  hosting and separate-process decisions are replaced)

## Context

The final pipeline step turns a `PlaylistAnalysis` into an image-generation
prompt and then an image. [ADR 0006](0006-llm-sidecars.md) planned small local
models run as separate sidecar processes. We are deploying to Cloud Run
([ADR 0013](0013-secrets-and-deployment.md)) with a scale-to-zero posture where
an idle system costs nothing.

Self-hosting the image model requires a GPU. An always-on L4 VM is roughly
$500/month regardless of how many images are generated, and scale-to-zero GPU on
Cloud Run still costs about $0.012 to $0.018 per cold image once model load is
included. Hosted image APIs are $0.003 to $0.04 per image with no idle cost. The
break-even for owning a GPU is well over ten thousand images per month; launch
volume is orders of magnitude below that.

## Decision

**Prompt and image generation call hosted APIs, not local models.** Prompt
generation calls a hosted text model; image generation calls a hosted image
model.

**The calls are made directly from the Go API. There are no sidecar services.**
The `PromptGenerator` and `ImageGenerator` ports from ADR 0006 stay; their
adapters in `internal/platform/llm/` call the provider directly instead of
POSTing to a sidecar URL. The whole backend is the single Cloud Run service from
ADR 0013. Separate services existed to isolate a heavy local model; a hosted API
call is an ordinary HTTP request and needs no isolation.

**The provider is configuration, swappable behind the port.** Sensible defaults
are a small fast text model for prompts and a Flux model or `gpt-image-1.5`/
`gpt-image-2` for images. Which one is a config value, not an architectural
commitment.

**Generated images are stored in a GCS bucket, and `imageUrl` points there.**
Hosted image APIs return bytes or a URL that expires within about an hour, so the
image adapter persists the result to Cloud Storage and returns a stable URL.
Objects are UUID-named in a public-read bucket: cover art is meant to be
displayed, and unguessable names keep it unlisted without the caching friction of
signed URLs.

**Generation stays synchronous.** Text (~1 to 3s) plus a hosted image (~2 to
15s) fits inside the Cloud Run request timeout. Moving to a job plus SSE model
([issue #34](https://github.com/phillipjad/Aurify/issues/34)) is a later
latency improvement, not a prerequisite, because there is no GPU cold start to
wait out.

## Consequences

- Configuration replaces `AURIFY_PROMPTGEN_URL` and `AURIFY_IMAGEGEN_URL` with,
  per provider, an API key, a model name, and the GCS bucket name.
- Two secrets are added under ADR 0013's scheme: the text-model key and the
  image-API key.
- Cover generation depends on two third-party services; an outage there degrades
  it. The deterministic placeholder already in the adapters stays as the
  fallback when a provider or key is absent.
- Cost scales per image rather than sitting on a fixed monthly floor. Owning a
  GPU is worth revisiting only at sustained high volume, and that is a change to
  one adapter behind the port.
