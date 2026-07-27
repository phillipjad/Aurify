output "service_url" {
  description = "The public URL Cloud Run assigned to the service."
  value       = google_cloud_run_v2_service.aurify.uri
}

output "runtime_service_account" {
  description = "Email of the runtime service account."
  value       = google_service_account.run.email
}

output "image_repository" {
  description = "Artifact Registry repository the image is pushed to."
  value       = "${var.region}-docker.pkg.dev/${var.project}/${google_artifact_registry_repository.aurify.repository_id}"
}
