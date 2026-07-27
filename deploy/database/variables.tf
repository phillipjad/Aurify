variable "project" {
  type        = string
  description = "GCP project ID that holds the database-url secret."
}

variable "region" {
  type        = string
  description = "GCP region for the google provider."
  default     = "us-central1"
}

variable "neon_region_id" {
  type        = string
  description = "Neon region, e.g. aws-us-east-1. Pick one close to the Cloud Run region."
  default     = "aws-us-east-1"
}

variable "postgres_version" {
  type        = number
  description = "Postgres major version for the Neon project."
  default     = 16
}

variable "runtime_sa_account_id" {
  type        = string
  description = "Account id (not full email) of the Cloud Run runtime service account created in deploy/app."
  default     = "aurify-run"
}
