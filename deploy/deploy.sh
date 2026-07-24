#!/usr/bin/env bash
#
# deploy.sh — build the image, push it to Artifact Registry, and deploy the
# Cloud Run service with its secrets and configuration wired in.
#
# Secrets come from Secret Manager (seed them first with deploy/secrets.sh);
# everything else is plain configuration set here. The application reads all of
# it as ordinary environment variables and is unaware of the difference.
#
# See ADR 0013 and deploy/README.md.

set -euo pipefail

# --- configuration: override via the environment or edit here ----------------
PROJECT="${AURIFY_GCP_PROJECT:-aurify-503320}"
REGION="${AURIFY_GCP_REGION:-us-central1}"
SERVICE="${AURIFY_SERVICE:-aurify}"
REPO="${AURIFY_AR_REPO:-aurify}" # Artifact Registry repository (Docker format)
RUN_SA="${AURIFY_RUN_SA_NAME:-aurify-run}@${PROJECT}.iam.gserviceaccount.com"

# Public origin the service is served on. Cloud Run assigns a *.run.app URL on
# first deploy; set this to that, or to your custom domain, and redeploy.
DOMAIN="${AURIFY_DOMAIN:?set AURIFY_DOMAIN to the https origin, e.g. https://aurify.example}"

# Public, non-secret. The client secret is injected from Secret Manager below.
GOOGLE_CLIENT_ID="${AURIFY_GOOGLE_CLIENT_ID:?set AURIFY_GOOGLE_CLIENT_ID}"

# Non-secret SMTP transport details. Username and password are secrets.
SMTP_HOST="${AURIFY_SMTP_HOST:-mail.smtp2go.com}"
SMTP_PORT="${AURIFY_SMTP_PORT:-2525}"
SMTP_FROM="${AURIFY_SMTP_FROM:-no-reply@aurify.app}"
SUPPORT_EMAIL="${AURIFY_SUPPORT_EMAIL:-support@aurify.app}"
# -----------------------------------------------------------------------------

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
require() { command -v "$1" >/dev/null 2>&1 || { echo "deploy.sh: '$1' is required" >&2; exit 1; }; }

require gcloud
require docker
require git

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
IMAGE="${REGION}-docker.pkg.dev/${PROJECT}/${REPO}/aurify:${VERSION}"

gcloud config set project "$PROJECT" >/dev/null

# Build for the Cloud Run architecture. On an Apple Silicon machine a native
# build produces an arm64 image that Cloud Run cannot start, so pin the platform.
log "Building $IMAGE (linux/amd64)"
docker build --platform linux/amd64 \
	-f backend/Dockerfile \
	--build-arg "VERSION=${VERSION}" \
	-t "$IMAGE" .

log "Pushing $IMAGE"
docker push "$IMAGE"

# Secret env vars: NAME=secret:version. Cloud Run reads these from Secret
# Manager at container start.
SET_SECRETS="\
AURIFY_AUTH_SIGNING_KEY=aurify-auth-signing-key:latest,\
AURIFY_GOOGLE_CLIENT_SECRET=aurify-google-client-secret:latest,\
AURIFY_SMTP_USERNAME=aurify-smtp-username:latest,\
AURIFY_SMTP_PASSWORD=aurify-smtp-password:latest,\
AURIFY_DATABASE_URL=aurify-database-url:latest"

# Plain configuration. The ^|^ prefix makes | the pair delimiter, so a value may
# contain commas (multiple CORS origins). | is used because the values include
# emails and URLs, which contain @, :, and /, but never a pipe.
SET_ENV="^|^\
AURIFY_HTTP_ADDR=:8080|\
AURIFY_CORS_ORIGINS=${DOMAIN}|\
AURIFY_APP_BASE_URL=${DOMAIN}|\
AURIFY_AUTH_ISSUER=aurify|\
AURIFY_AUTH_AUDIENCE=aurify-api|\
AURIFY_AUTH_COOKIE_SECURE=true|\
AURIFY_GOOGLE_CLIENT_ID=${GOOGLE_CLIENT_ID}|\
AURIFY_GOOGLE_REDIRECT_URL=${DOMAIN}/api/v1/auth/federated/google/callback|\
AURIFY_SMTP_HOST=${SMTP_HOST}|\
AURIFY_SMTP_PORT=${SMTP_PORT}|\
AURIFY_SMTP_TLS=true|\
AURIFY_SMTP_FROM=${SMTP_FROM}|\
AURIFY_SUPPORT_EMAIL=${SUPPORT_EMAIL}"

log "Deploying $SERVICE to Cloud Run in $REGION"
gcloud run deploy "$SERVICE" \
	--image "$IMAGE" \
	--region "$REGION" \
	--service-account "$RUN_SA" \
	--set-secrets "$SET_SECRETS" \
	--set-env-vars "$SET_ENV" \
	--port 8080 \
	--allow-unauthenticated

log "Deployed $VERSION."
echo "If DOMAIN changed, register ${DOMAIN}/api/v1/auth/federated/google/callback"
echo "as an authorized redirect URI on the Google OAuth client."
