output "database_host" {
  description = "Neon default database host."
  value       = neon_project.aurify.database_host
}

output "database_url_secret" {
  description = "Secret Manager secret the app reads the connection string from."
  value       = google_secret_manager_secret.database_url.secret_id
}
