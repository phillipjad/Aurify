# Deploying Aurify

Aurify runs as a single container on **Cloud Run**, with secrets in **Google
Secret Manager** and PostgreSQL on **Neon**. Infrastructure is managed with
**OpenTofu**; there are no deploy scripts. The image is built and deployed by the
`Deploy` GitHub Actions workflow. The decisions are in
[ADR 0013](../docs/adr/0013-secrets-and-deployment.md) and
[ADR 0014](../docs/adr/0014-hosted-generation-apis.md).

## Two modules

| Module | Manages | Applied |
|---|---|---|
| [`app/`](app) | runtime service account, secret containers, Artifact Registry, Cloud Run | every deploy |
| [`database/`](database) | the Neon project and its connection string (written into Secret Manager) | only when `provision_database` is set |

They have separate state (same bucket, different prefix). `app/` never references
the Neon resources, so a routine deploy cannot alter or destroy the database.

## What is a secret, and what is not

`app/` creates the secret **containers**; the **values** are injected out of band
and never enter OpenTofu state. The one exception is the database URL, which Neon
generates, so the `database/` module writes it into Secret Manager itself.

| Value | Source |
|---|---|
| `aurify-database-url` | Neon generates it, `database/` writes it |
| `aurify-auth-signing-key`, `aurify-google-client-secret`, `aurify-smtp-username`, `aurify-smtp-password` | you inject them (below) |

## One-time bootstrap

**Prerequisites:** `gcloud` and `tofu` locally, a Neon account, and a GCP project.

1. **Create the state bucket.** OpenTofu stores state here; it must exist before
   `tofu init`. Keep it private and versioned, it holds the database credential.

   ```bash
   gcloud storage buckets create gs://YOUR-PROJECT-tfstate \
     --location=us-central1 --uniform-bucket-level-access
   gcloud storage buckets update gs://YOUR-PROJECT-tfstate --versioning
   ```

2. **Create the Neon database.**

   ```bash
   export NEON_API_KEY=...    # from the Neon console
   tofu -chdir=deploy/database init -backend-config="bucket=YOUR-PROJECT-tfstate"
   tofu -chdir=deploy/database apply -var="project=YOUR-PROJECT"
   ```

3. **Create the secret containers and the runtime account**, then inject the
   four values. The targeted apply creates the containers without deploying the
   service, which cannot start before the values exist.

   ```bash
   tofu -chdir=deploy/app init -backend-config="bucket=YOUR-PROJECT-tfstate"
   tofu -chdir=deploy/app apply -var="project=YOUR-PROJECT" -var="image=placeholder" \
     -target=google_secret_manager_secret.app \
     -target=google_service_account.run

   printf '%s' "$(openssl rand -base64 32)" | gcloud secrets versions add aurify-auth-signing-key --data-file=-
   printf '%s' 'GOCSPX-...'                 | gcloud secrets versions add aurify-google-client-secret --data-file=-
   printf '%s' 'smtp-user'                  | gcloud secrets versions add aurify-smtp-username --data-file=-
   printf '%s' 'smtp-pass'                  | gcloud secrets versions add aurify-smtp-password --data-file=-
   ```

4. **Set the repository variables and secrets** the `Deploy` workflow reads:

   - Variables: `GCP_PROJECT`, `GCP_REGION`, `TF_STATE_BUCKET`, `AURIFY_DOMAIN`,
     `AURIFY_GOOGLE_CLIENT_ID`, `AURIFY_SMTP_FROM`, `AURIFY_SUPPORT_EMAIL`
   - Secrets: `GCP_SA_KEY` (a deploy service account key with rights to Cloud
     Run, Artifact Registry, Secret Manager, and IAM), `NEON_API_KEY`

## Deploying

Run the **Deploy** workflow (Actions tab, "Run workflow"). Leave
`provision_database` **off** for routine deploys; tick it only when you
intentionally change the Neon database. It builds and pushes the image, then
applies `deploy/app`.

On the **first** deploy you will not yet know the `*.run.app` URL. Deploy once
with a placeholder `AURIFY_DOMAIN`, read the URL from the workflow's final step,
set `AURIFY_DOMAIN` to it, and deploy again so the origins and OAuth redirect are
correct.

## After the domain is known

Register the production callback on the Google OAuth client:

```
https://<your-domain>/api/v1/auth/federated/google/callback
```

Google requires HTTPS for non-localhost redirect URIs; Cloud Run serves HTTPS.

## Rotating a secret

Add a new version and redeploy so Cloud Run picks up `latest`:

```bash
printf '%s' 'new-value' | gcloud secrets versions add <secret-id> --data-file=-
```

## Notes

- **Prompt and image generation** run on the built-in placeholder for now. When
  the ADR 0014 adapters land, that change adds the covers GCS bucket and the two
  generation API-key secrets here.
- **Public bucket / public service:** granting `allUsers` may be blocked by an
  org policy (domain-restricted sharing or public-access prevention). If apply
  fails on the IAM member, that policy is why.
- **Port.** The service listens on `:8080`, matching Cloud Run's default.
