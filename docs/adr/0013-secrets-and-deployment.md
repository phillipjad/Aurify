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
binding is a line per secret on the Cloud Run service (`--set-secrets`), so the
same image runs unmodified in any environment that can set environment
variables.

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

- **Rotation** is a new secret version plus a redeploy (`--set-secrets` resolves
  `:latest` at start). No live reload.
- **Local development is untouched.** `.env` files still fill the variables; only
  the production source of the values changes.
- **A dedicated runtime service account** holds `secretAccessor` on each secret,
  rather than the default compute account, so the grant is scoped to what the
  service reads.
- The two scripts and the runbook live in `deploy/`.
