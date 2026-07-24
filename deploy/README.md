# Deploying Aurify

Aurify runs as a single container on **Cloud Run**, with secrets in **Google
Secret Manager** and PostgreSQL on **Neon**. The container is the one built by
[`backend/Dockerfile`](../backend/Dockerfile): it embeds the compiled SPA, so
the API and the web app are served from one origin.

The application reads all configuration from environment variables and nothing
from a file. In production the secret ones come from Secret Manager and the rest
are set on the service. Local development is unchanged and still uses `.env`
files (see `scripts/dev-setup.sh`). The decision is recorded in
[ADR 0013](../docs/adr/0013-secrets-and-deployment.md).

## What is a secret, and what is not

| Secret (Secret Manager) | Configuration (set on the service) |
|---|---|
| `AURIFY_AUTH_SIGNING_KEY` | origins, base URL, issuer, audience, TTLs |
| `AURIFY_GOOGLE_CLIENT_SECRET` | `AURIFY_GOOGLE_CLIENT_ID` (public) |
| `AURIFY_SMTP_USERNAME` / `AURIFY_SMTP_PASSWORD` | `AURIFY_SMTP_HOST` / `PORT` / `FROM` / `TLS` |
| `AURIFY_DATABASE_URL` (Neon, carries the password) | the OAuth redirect URL |

## One-time setup

**Prerequisites:** `gcloud` (authenticated, project selected), `docker`, and a
Neon account.

1. **Create the database on Neon.** Make a project, copy the connection string.
   It looks like `postgresql://user:pass@ep-xxx.region.aws.neon.tech/aurify?sslmode=require`.
   Keep `sslmode=require`; Neon only accepts TLS. This whole string is the value
   of the `aurify-database-url` secret. The schema is created automatically:
   the app runs its goose migrations on startup, so the first deploy provisions
   an empty database.

2. **Seed the secrets** and create the runtime service account:

   ```bash
   AURIFY_GCP_PROJECT=your-project ./deploy/secrets.sh
   ```

   It prompts for each value (hidden input) and offers to generate the signing
   key. Re-run it to rotate any secret.

3. **Create the Artifact Registry repository** the image is pushed to, once:

   ```bash
   gcloud artifacts repositories create aurify \
     --repository-format=docker --location=us-central1
   gcloud auth configure-docker us-central1-docker.pkg.dev
   ```

## Deploy

```bash
AURIFY_DOMAIN=https://aurify-xxxx.run.app \
AURIFY_GOOGLE_CLIENT_ID=467040299717-...apps.googleusercontent.com \
./deploy/deploy.sh
```

`deploy.sh` builds the image for `linux/amd64` (required, even on Apple
Silicon), pushes it, and deploys the service with the secrets and configuration
wired in.

On the **first** deploy you will not yet know the `*.run.app` URL. Deploy once
with a placeholder domain, read the URL Cloud Run prints, then set `AURIFY_DOMAIN`
to it and deploy again so the origins and the OAuth redirect URL are correct.

## After the domain is known

Register the production callback as an authorized redirect URI on the Google
OAuth client (the same client used locally):

```
https://<your-domain>/api/v1/auth/federated/google/callback
```

Google requires HTTPS for non-localhost redirect URIs; Cloud Run serves HTTPS,
so this is satisfied. While the OAuth consent screen is in Testing, only listed
test users can sign in.

## Rotating a secret

Re-run `./deploy/secrets.sh` (it adds a new version), then `./deploy/deploy.sh`
to restart against `:latest`. Cloud Run resolves secret versions at container
start, so a rotation takes effect on the next deploy, not live.

## Notes

- **Port.** The app listens on `AURIFY_HTTP_ADDR` (`:8080`), and the deploy pins
  Cloud Run's port to 8080. If you change one, change the other.
- **No Cloud SQL.** Postgres is Neon, reached over the public internet with TLS.
  There is no VPC connector or proxy sidecar to configure.
- **Scale to zero** is the Cloud Run default, so an idle service costs nothing.
  The first request after idle pays a cold start, and Neon likewise resumes from
  its autosuspend.
