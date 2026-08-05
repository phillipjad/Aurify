# Environment variables

Every setting the API reads, and why it is the way it is.
[`backend/.env.default`](../backend/.env.default) holds the values; this holds the
explanations.

## How configuration is layered

docker-compose loads `backend/.env.default`, then `backend/.env.dev`, then
`backend/.env`, with later files winning.

`.env.default` is committed and genuinely loaded, which is why it is named
"default" rather than "example": editing it changes what every developer's stack
runs with. Put personal overrides in `.env`, which is gitignored.

There is no development mode. Development substitutes real infrastructure
instead: docker-compose runs Mailpit, a real SMTP server, so the verification and
password-reset paths that run locally are the same code that runs for real users.
A mail relay and a signing key are therefore required in every environment. An
unsafe configuration is not representable rather than merely gated behind a flag
someone could set by accident.

Run `./scripts/dev-setup.sh` to generate a local config.

## HTTP and storage

| Variable | Notes |
|---|---|
| `AURIFY_HTTP_ADDR` | Listen address. Only the port the process binds; unrelated to any public URL. |
| `AURIFY_CORS_ORIGINS` | Comma-separated allowed origins. |
| `AURIFY_DATABASE_URL` | PostgreSQL DSN. The schema is created and migrated on startup via goose. |
| `AURIFY_LRCLIB_URL` | Lyrics source. See [ADR 0015](adr/0015-lyrics-cache-and-failure-policy.md). |

## Prompt generation

See [ADR 0016](adr/0016-generation-providers.md).

| Variable | Notes |
|---|---|
| `AURIFY_PROMPTGEN_URL` | Base URL of any OpenAI-compatible chat-completions provider. Empty selects the built-in placeholder prompt. |
| `AURIFY_PROMPTGEN_MODEL` | Model name. |
| `AURIFY_PROMPTGEN_API_KEY` | Empty for Ollama, which wants no credential. |

For a local model:

```bash
brew install ollama && ollama serve && ollama pull gemma3:4b
```

Ollama runs on the host rather than as a compose service, because Docker on macOS
cannot reach Metal and a containerized model falls back to CPU. Which hostname
you want therefore depends on where the API itself is running:

| Where the API runs | `AURIFY_PROMPTGEN_URL` |
|---|---|
| The compose stack (`start_container.sh`) | `http://host.docker.internal:11434/v1` |
| Directly on the host | `http://localhost:11434/v1` |

The same three variables reach Groq, OpenRouter, Cerebras or OpenAI: set their
base URL, their model, and a key.

## Image generation

Cloudflare Workers AI. See [ADR 0016](adr/0016-generation-providers.md).

| Variable | Notes |
|---|---|
| `AURIFY_IMAGEGEN_ACCOUNT_ID` | From the Cloudflare dashboard. Empty selects the local placeholder image. |
| `AURIFY_IMAGEGEN_API_KEY` | An API token scoped to Workers AI Read **and** Edit. Empty selects the placeholder. |
| `AURIFY_IMAGEGEN_MODEL` | Workers AI model id. |
| `AURIFY_IMAGEGEN_URL` | Only to aim the client at a test server. Empty means the Workers AI root; the rest of the endpoint is assembled from the account id and model. |

Workers AI is the only image provider found that is free without a card, a
deposit or an expiry. Together AI now requires a $5 deposit, and Gemini's image
models are not free-tier eligible at all.

The model choice is constrained twice over, both measured:

| Constraint | Detail |
|---|---|
| Safety classifier | FLUX models refused 2 of 8 ordinary abstract-art prompts as NSFW, with no way to opt out. Error 3030, FLUX endpoints only. |
| Cost | `lucid-origin` and `phoenix-1.0` cost around $0.006 per 512x512 tile, so one 1024x1024 image is a quarter of the 10,000-neuron daily free allowance. |

The Stability models are priced at $0 per step, which leaves them the only ones
both unfiltered and usable on the free tier.
`@cf/stabilityai/stable-diffusion-xl-base-1.0` is the same price if you want more
steps and less speed.

## Audio features

No settings. Features come from AcousticBrainz via MusicBrainz, both free and
unauthenticated, cached in `track_features`. When nothing in a playlist matches,
the `AURIFY_PROMPTGEN_*` model above estimates them instead. See
[ADR 0018](adr/0018-audio-features.md).

## DSP OAuth

Obtain each from the provider's developer console.

| Variable | Notes |
|---|---|
| `AURIFY_SPOTIFY_CLIENT_ID` / `_SECRET` / `_REDIRECT_URL` | See [ADR 0005](adr/0005-dsp-provider-abstraction.md). |
| `AURIFY_APPLE_CLIENT_ID` / `_SECRET` / `_REDIRECT_URL` | |
| `AURIFY_YOUTUBE_CLIENT_ID` / `_SECRET` / `_REDIRECT_URL` | See [ADR 0009](adr/0009-youtube-music-via-data-api-v3.md). |

YouTube Music uses Google OAuth plus the YouTube Data API v3: create a "Web
application" OAuth client in Google Cloud Console, enable "YouTube Data API v3",
and register the redirect URL on that client.

The redirect URLs must match what is registered with the provider exactly. A
single stray character produces `redirect_uri_mismatch`, which reads as a broken
integration rather than a typo.

## Authentication

See [ADR 0011](adr/0011-authentication-and-sessions.md) and
[ADR 0012](adr/0012-account-lockout-policy.md).

| Variable | Notes |
|---|---|
| `AURIFY_APP_BASE_URL` | Public origin of the web app. Verification and password-reset links are built against it, so it must be an address the user's browser can reach. |
| `AURIFY_AUTH_SIGNING_KEY` | Base64 of a 32-byte Ed25519 seed. **Required in production.** |
| `AURIFY_AUTH_ISSUER` / `_AUDIENCE` | Token claims. |
| `AURIFY_AUTH_ACCESS_TTL` | Access tokens are verified statelessly, so this is also how long a revoked session keeps working. Keep it short. |
| `AURIFY_AUTH_REFRESH_TTL` | Refresh token idle timeout. |
| `AURIFY_AUTH_SESSION_TTL` | Absolute session cap, regardless of refreshes. |
| `AURIFY_AUTH_COOKIE_SECURE` | Leave true everywhere except local HTTP development. |
| `AURIFY_SUPPORT_EMAIL` | Shown to users who hit the permanent lockout, since only an operator can lift one. Point it at a monitored address before launch. |

Generate a signing key with:

```bash
head -c 32 /dev/urandom | base64
```

Left empty, the API generates an ephemeral key at startup and warns. Every
restart then invalidates outstanding access tokens, and two instances reject each
other's.

`AURIFY_AUTH_COOKIE_SECURE=false` exists because a browser silently drops a
Secure cookie sent over plain http: sign-in appears to succeed and then nothing
is authenticated.

### Sign in with Google

| Variable | Notes |
|---|---|
| `AURIFY_GOOGLE_CLIENT_ID` / `_SECRET` | Empty disables the feature: the routes answer 501 and the frontend hides the button. |
| `AURIFY_GOOGLE_REDIRECT_URL` | Points at the API's own port. |

This is a **separate** OAuth client from `AURIFY_YOUTUBE_*` and must stay
separate: that one grants access to a music library, this one asserts who the
user is, and sharing a client would let a data connection imply a login.

The redirect points at the API's own port, matching the DSP callbacks, so the
container stack can complete a sign-in with no frontend dev server running.
Cookies are scoped by host and ignore the port, so a session cookie set on
`localhost:8080` is still sent to the app on `localhost:5173`.

Google requires HTTPS for every non-localhost redirect URI, so a deployment sets
its real public URL, for example
`https://aurify.example/api/v1/auth/federated/google/callback`. That URL is
browser-facing and unrelated to `AURIFY_HTTP_ADDR`.

## Outbound mail

SMTP2GO by default, but any SMTP relay works.

| Variable | Notes |
|---|---|
| `AURIFY_SMTP_HOST` | Required in every environment. Locally the Mailpit container; in production a real relay. |
| `AURIFY_SMTP_PORT` | |
| `AURIFY_SMTP_TLS` | Requires STARTTLS. Only ever false for a relay on the local machine. |
| `AURIFY_SMTP_USERNAME` / `_PASSWORD` | |
| `AURIFY_SMTP_FROM` | |

An invalid configuration is a startup failure rather than a silent downgrade.
Undelivered mail otherwise presents as users simply unable to sign up or recover
an account, with nothing looking unhealthy.
