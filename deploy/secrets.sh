#!/usr/bin/env bash
#
# secrets.sh — create or update Aurify's secrets in Google Secret Manager and
# grant the Cloud Run runtime service account read access to them.
#
# Run this once to seed the secrets, and again whenever one rotates. It is
# idempotent: creating a secret that exists is skipped, adding a value always
# creates a new version, and granting an existing binding is a no-op.
#
# Values are read from a hidden prompt, so they never land in your shell history
# or on disk. The signing key can be generated for you.
#
# See ADR 0013 and deploy/README.md.

set -euo pipefail

# --- configuration: override via the environment or edit here ----------------
PROJECT="${AURIFY_GCP_PROJECT:-aurify-503320}"
# Dedicated runtime identity for the Cloud Run service. It holds only
# secretAccessor on the secrets below, nothing else.
RUN_SA_NAME="${AURIFY_RUN_SA_NAME:-aurify-run}"
RUN_SA="${RUN_SA_NAME}@${PROJECT}.iam.gserviceaccount.com"

# The secrets this project needs. AURIFY_DATABASE_URL is the Neon connection
# string (it carries the password). The DSP secrets are commented out until
# those integrations are real; uncomment as needed.
SECRETS=(
	aurify-auth-signing-key
	aurify-google-client-secret
	aurify-smtp-username
	aurify-smtp-password
	aurify-database-url
	# aurify-spotify-client-secret
	# aurify-apple-client-secret
	# aurify-youtube-client-secret
)
# -----------------------------------------------------------------------------

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }

require() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "secrets.sh: '$1' is required but not on PATH" >&2
		exit 1
	}
}

require gcloud
gcloud config set project "$PROJECT" >/dev/null

log "Enabling the Secret Manager API (no-op if already enabled)"
gcloud services enable secretmanager.googleapis.com >/dev/null

# Dedicated runtime service account, created if it does not exist.
if ! gcloud iam service-accounts describe "$RUN_SA" >/dev/null 2>&1; then
	log "Creating runtime service account $RUN_SA"
	gcloud iam service-accounts create "$RUN_SA_NAME" \
		--display-name "Aurify Cloud Run runtime" >/dev/null
else
	log "Runtime service account $RUN_SA already exists"
fi

# read_secret_value NAME -> echoes the value to stdout.
# The signing key is offered for generation; everything else is prompted.
read_secret_value() {
	local name="$1" value confirm

	if [[ "$name" == "aurify-auth-signing-key" ]]; then
		read -rp "Generate a new signing key for '$name'? [Y/n] " confirm
		if [[ "$confirm" != [nN] ]]; then
			# base64 of 32 random bytes, exactly what AURIFY_AUTH_SIGNING_KEY expects.
			openssl rand -base64 32
			return
		fi
	fi

	# -s hides the input; the prompt goes to stderr so stdout stays clean.
	read -rsp "Value for '$name' (input hidden): " value >&2
	echo >&2
	printf '%s' "$value"
}

for name in "${SECRETS[@]}"; do
	if ! gcloud secrets describe "$name" >/dev/null 2>&1; then
		log "Creating secret $name"
		gcloud secrets create "$name" --replication-policy=automatic >/dev/null
	fi

	value="$(read_secret_value "$name")"
	if [[ -z "$value" ]]; then
		echo "  skipped $name (no value entered)"
		continue
	fi
	printf '%s' "$value" | gcloud secrets versions add "$name" --data-file=- >/dev/null
	log "Added a new version to $name"

	# Least privilege: the runtime account can read this secret and nothing more.
	gcloud secrets add-iam-policy-binding "$name" \
		--member "serviceAccount:${RUN_SA}" \
		--role roles/secretmanager.secretAccessor >/dev/null
done

log "Done. $RUN_SA can read: ${SECRETS[*]}"
echo "Next: deploy/deploy.sh"
