variable "project" {
  type        = string
  description = "GCP project ID."
}

variable "region" {
  type        = string
  description = "GCP region for Cloud Run, Artifact Registry, and secrets."
  default     = "us-central1"
}

variable "service_name" {
  type        = string
  description = "Cloud Run service name."
  default     = "aurify"
}

variable "runtime_sa_account_id" {
  type        = string
  description = "Account id of the Cloud Run runtime service account. Must match deploy/database."
  default     = "aurify-run"
}

variable "image" {
  type        = string
  description = "Full image reference to deploy, e.g. us-central1-docker.pkg.dev/PROJECT/aurify/aurify:TAG."
}

variable "domain" {
  type        = string
  description = "Public https origin the service is served on. Used for CORS, the app base URL, and the OAuth redirect."
}

variable "google_client_id" {
  type        = string
  description = "Google OAuth client id (public, not a secret)."
}

variable "smtp_host" {
  type        = string
  description = "SMTP relay host."
  default     = "mail.smtp2go.com"
}

variable "smtp_port" {
  type        = string
  description = "SMTP relay port."
  default     = "2525"
}

variable "smtp_from" {
  type        = string
  description = "From address for outbound mail."
}

variable "support_email" {
  type        = string
  description = "Support address shown to locked-out users."
}
