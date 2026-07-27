# Database module: the Neon PostgreSQL project, and its connection string written
# into Secret Manager for the app to read.
#
# This is a separate root module with its own state on purpose. Routine app
# deploys apply deploy/app only and never touch these resources, so they cannot
# alter or destroy the database. This module is applied on its own, gated by the
# provision_database flag in the deploy workflow.
#
# The Neon connection string is a generated credential, so it lives in this
# module's state. The state backend must stay private (see deploy/README.md).

terraform {
  required_version = ">= 1.6"

  required_providers {
    neon = {
      source  = "kislerdm/neon"
      version = ">= 0.6.0"
    }
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0"
    }
  }

  # Partial backend config: the bucket is supplied at init with
  # -backend-config="bucket=..." so the project is not hard-coded here.
  backend "gcs" {
    prefix = "database"
  }
}

# Reads NEON_API_KEY from the environment (set from a repo secret in CI).
provider "neon" {}

provider "google" {
  project = var.project
  region  = var.region
}

resource "neon_project" "aurify" {
  name       = "aurify"
  region_id  = var.neon_region_id
  pg_version = var.postgres_version
}

# The connection string, stored where the app reads its secrets. Owning the
# secret here (not in deploy/app) keeps the database self-contained: creating the
# database and publishing how to reach it are one operation.
resource "google_secret_manager_secret" "database_url" {
  secret_id = "aurify-database-url"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "database_url" {
  secret      = google_secret_manager_secret.database_url.id
  secret_data = neon_project.aurify.connection_uri
}

# Let the app's runtime service account read it. The member is granted by its
# deterministic email rather than a cross-module reference, so this module does
# not depend on deploy/app. Granting a role to an identity that does not exist
# yet is permitted, so the two modules can be applied in either order.
resource "google_secret_manager_secret_iam_member" "database_url_accessor" {
  secret_id = google_secret_manager_secret.database_url.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${var.runtime_sa_account_id}@${var.project}.iam.gserviceaccount.com"
}
