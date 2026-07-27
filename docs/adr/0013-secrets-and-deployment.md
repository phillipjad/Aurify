# 0013 — Secrets in Secret Manager, injected as env vars on Cloud Run

- Status: Accepted
- Date: 2026-07-24

## Context

`internal/config` reads every setting from an environment variable and nothing
from a file. Locally, docker-compose fills those variables from layered `.env`
files (see [0011](0011-authentication-and-sessions.md) and `scripts/dev-setup.sh`).
Production needs the same variables filled with real secrets, without those
secrets living in the repository or in a committed file.

We are deploying the container to Cloud Run and hosting PostgreSQL on Neon.
Cloud Run and Secret Manager sit in the same GCP project as the Google OAuth
client the sign-in flow already uses.

## Decision

**Secrets live in Google Secret Manager and Cloud Run injects them as
environment variables at container start.** The application is unchanged: it
reads the same variables it always has, and never imports a cloud SDK. The
binding is a per-secret env var on the Cloud Run service, so the same image runs
unmodified in any environment that can set environment variables.

**Infrastructure is defined in OpenTofu and is the only way to deploy.** Two root
modules under `deploy/` with separate state: `database/` (the Neon project,
applied on its own and gated by a flag) and `app/` (the runtime service account,
secret containers, Artifact Registry, and the Cloud Run service, applied on every
release). The split keeps a routine deploy from being able to touch the database.
The image is built and pushed by the `Deploy` GitHub Actions workflow, which then
runs `tofu apply`; there are no deploy scripts.

**Secret values never enter OpenTofu state.** OpenTofu creates the secret
containers; the values are injected out of band with `gcloud secrets versions
add`. The exception is `AURIFY_DATABASE_URL`: Neon generates it, so the
`database/` module reads it from the Neon resource and writes it into Secret
Manager. That connection string therefore lives in state, which is why the state
bucket must be private and versioned.

**These values are secrets** and are stored in Secret Manager:
`AURIFY_AUTH_SIGNING_KEY`, `AURIFY_GOOGLE_CLIENT_SECRET`, `AURIFY_SMTP_USERNAME`,
`AURIFY_SMTP_PASSWORD`, `AURIFY_DATABASE_URL`, and the DSP client secrets once
those are real. `AURIFY_DATABASE_URL` is the Neon connection string, which
carries the database password.

**Everything else is plain configuration** and is set directly on the service:
origins, base URL, issuer/audience, token TTLs, SMTP host/port, the OAuth
redirect URL, and the client IDs. A `client_id` is public, it appears in every
authorization URL, so it is configuration, not a secret.

**PostgreSQL is Neon, reached over the public internet with `sslmode=require`.**
There is no Cloud SQL instance and no VPC connector. The app runs the goose
migrations on startup, so a fresh database is provisioned by the first deploy.

The application does not fetch from Secret Manager at runtime. Deploy-time
injection needs no application code and keeps the binary portable; a runtime
client would buy live rotation we do not need and couple the app to GCP.

## Consequences

- **Rotation** is a new secret version (`gcloud secrets versions add`) plus a
  redeploy; Cloud Run resolves `latest` at container start. No live reload.
- **Local development is untouched.** `.env` files still fill the variables; only
  the production source of the values changes.
- **A dedicated runtime service account** holds `secretAccessor` on each secret,
  rather than the default compute account, so the grant is scoped to what the
  service reads.
- **First-time setup is multi-step** because the containers must exist before the
  values are injected and the service must not start before either the values or
  the image exist. The runbook in `deploy/` orders it; routine deploys are one
  workflow run.
- The OpenTofu modules and the runbook live in `deploy/`; the deploy workflow is
  `.github/workflows/deploy.yml`.
