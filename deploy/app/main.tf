# App module: everything the web app needs to run on Cloud Run, except the
# database (deploy/database). This is what the deploy workflow applies on every
# release. It never references the Neon resources, so a routine deploy cannot
# touch the database.
#
# It provisions exactly what the current binary uses. Prompt and image
# generation run on the built-in placeholder until the ADR 0014 adapters land;
# that work adds the covers GCS bucket and the two generation API-key secrets.

terraform {
  required_version = ">= 1.6"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0"
    }
  }

  # Partial backend config: the bucket is supplied at init with
  # -backend-config="bucket=..." (same bucket as the database module, different
  # prefix, so the two states are separate).
  backend "gcs" {
    prefix = "app"
  }
}

provider "google" {
  project = var.project
  region  = var.region
}

locals {
  # Human-provided secrets: env var name => Secret Manager secret id. Terraform
  # owns the containers; the values are injected out of band (see README), never
  # in state.
  secret_env = {
    AURIFY_AUTH_SIGNING_KEY     = "aurify-auth-signing-key"
    AURIFY_GOOGLE_CLIENT_SECRET = "aurify-google-client-secret"
    AURIFY_SMTP_USERNAME        = "aurify-smtp-username"
    AURIFY_SMTP_PASSWORD        = "aurify-smtp-password"
  }

  # The database URL secret is created by the database module. It is referenced
  # here by name only, so this module has no dependency on that one.
  all_secret_env = merge(local.secret_env, {
    AURIFY_DATABASE_URL = "aurify-database-url"
  })

  # Non-secret configuration.
  plain_env = {
    AURIFY_HTTP_ADDR           = ":8080"
    AURIFY_CORS_ORIGINS        = var.domain
    AURIFY_APP_BASE_URL        = var.domain
    AURIFY_AUTH_ISSUER         = "aurify"
    AURIFY_AUTH_AUDIENCE       = "aurify-api"
    AURIFY_AUTH_COOKIE_SECURE  = "true"
    AURIFY_GOOGLE_CLIENT_ID    = var.google_client_id
    AURIFY_GOOGLE_REDIRECT_URL = "${var.domain}/api/v1/auth/federated/google/callback"
    AURIFY_SMTP_HOST           = var.smtp_host
    AURIFY_SMTP_PORT           = var.smtp_port
    AURIFY_SMTP_TLS            = "true"
    AURIFY_SMTP_FROM           = var.smtp_from
    AURIFY_SUPPORT_EMAIL       = var.support_email
  }
}

resource "google_service_account" "run" {
  account_id   = var.runtime_sa_account_id
  display_name = "Aurify Cloud Run runtime"
}

resource "google_artifact_registry_repository" "aurify" {
  location      = var.region
  repository_id = "aurify"
  format        = "DOCKER"
  description   = "Aurify container images"
}

# One container per human-provided secret. Empty until a value is injected.
resource "google_secret_manager_secret" "app" {
  for_each  = toset(values(local.secret_env))
  secret_id = each.value

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_iam_member" "app" {
  for_each = google_secret_manager_secret.app

  secret_id = each.value.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.run.email}"
}

resource "google_cloud_run_v2_service" "aurify" {
  name     = var.service_name
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.run.email

    containers {
      image = var.image

      # Cover generation runs in a goroutine after the response is written
      # (ADR 0019). The Cloud Run default allocates CPU only while a request is
      # in flight, which would throttle that work to a crawl mid-job; cpu_idle
      # = false keeps CPU allocated for the instance's whole lifetime. The
      # service still scales to zero when idle.
      resources {
        cpu_idle = false
      }

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.plain_env
        content {
          name  = env.key
          value = env.value
        }
      }

      dynamic "env" {
        for_each = local.all_secret_env
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = env.value
              version = "latest"
            }
          }
        }
      }
    }
  }

  # The secret env vars are referenced by name, so Terraform cannot infer that
  # the service needs the containers and their IAM to exist first. The database
  # URL secret lives in the other module and is ordered by apply sequence.
  depends_on = [
    google_secret_manager_secret.app,
    google_secret_manager_secret_iam_member.app,
  ]
}

# Public: the app is a website, and it runs its own authentication.
resource "google_cloud_run_v2_service_iam_member" "public" {
  name     = google_cloud_run_v2_service.aurify.name
  location = google_cloud_run_v2_service.aurify.location
  role     = "roles/run.invoker"
  member   = "allUsers"
}
