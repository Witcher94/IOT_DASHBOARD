# ==========================================
# Outputs
# ==========================================

output "load_balancer_ip" {
  description = "Global Load Balancer IP - додай A-запис в DNS!"
  value       = google_compute_global_address.lb_ip.address
}

output "domain" {
  description = "Your domain"
  value       = var.domain
}

output "frontend_url" {
  description = "Frontend URL"
  value       = "https://${var.domain}"
}

output "backend_url" {
  description = "Backend API URL"
  value       = "https://${var.domain}/api/v1"
}

output "esp_backend_url" {
  description = "URL for ESP devices"
  value       = "https://${var.domain}"
}

output "oauth_redirect_uri" {
  description = "Add this to Google OAuth Authorized redirect URIs"
  value       = "https://${var.domain}/api/v1/auth/google/callback"
}

output "cloud_sql_connection" {
  description = "Cloud SQL connection name"
  value       = google_sql_database_instance.postgres.connection_name
}

output "artifact_registry" {
  description = "Docker registry URL"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/iot-dashboard"
}

output "cloudrun_service_account" {
  description = "Cloud Run Service Account email"
  value       = google_service_account.cloudrun_sa.email
}

# ==========================================
# GitHub Actions Secrets (copy these!)
# ==========================================

output "github_workload_identity_provider" {
  description = "GitHub Secret: WIF_PROVIDER"
  value       = google_iam_workload_identity_pool_provider.github_provider.name
}

output "github_service_account" {
  description = "GitHub Secret: WIF_SERVICE_ACCOUNT"
  value       = google_service_account.github_actions_sa.email
}

output "github_secrets_instructions" {
  description = "Add these secrets to GitHub repository"
  value       = <<-EOT
    
    🔐 GITHUB SECRETS (Settings → Secrets → Actions):
    ================================================
    
    GCP_PROJECT_ID:
    ${var.project_id}
    
    WIF_PROVIDER:
    ${google_iam_workload_identity_pool_provider.github_provider.name}
    
    WIF_SERVICE_ACCOUNT:
    ${google_service_account.github_actions_sa.email}
    
    GCP_REGION:
    ${var.region}
    
    DOMAIN:
    ${var.domain}
    
    DB_PASSWORD:
    (use same as in terraform.tfvars)
    
    ADMIN_EMAIL:
    ${var.admin_email}
    
  EOT
}

output "dns_nameservers" {
  description = "DNS Nameservers"
  value       = var.create_dns_zone ? google_dns_managed_zone.main[0].name_servers : try(data.google_dns_managed_zone.existing[0].name_servers, [])
}

output "dns_instructions" {
  description = "DNS Configuration"
  value       = <<-EOT

    📍 DNS CONFIGURATION:
    ================================
    
    ✅ A запис створено автоматично:
    ${var.domain} -> ${google_compute_global_address.lb_ip.address}
    
    ${var.create_dns_zone ? "⚠️ Налаштуй NS записи у реєстратора домену!" : "✅ Cloud Domains керує DNS автоматично!"}
    
    ⏰ SSL certificate: 15-60 хв після DNS propagation
    
  EOT
}

output "estimated_monthly_cost" {
  description = "Estimated monthly cost"
  value       = <<-EOT
    
    💰 ESTIMATED MONTHLY COST:
    ================================
    Cloud SQL db-f1-micro:     ~$7-10
    Load Balancer:             ~$18
    Cloud DNS:                 ~$0.20
    Cloud Run:                 ~$0-5
    SSL Certificate:           FREE
    Secret Manager:            ~$0
    Cloud SQL Auth Proxy:      FREE ✨
    --------------------------------
    TOTAL:                     ~$25-33/month
    
    🔒 SECURITY:
    ✅ Database via Cloud SQL Auth Proxy (no direct access)
    ✅ HTTPS only
    ✅ Secrets in Secret Manager
    ✅ Cloud Run behind Load Balancer
    
    💡 Cloud Run services deployed via GitHub Actions!
  EOT
}
