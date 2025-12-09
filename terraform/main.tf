# ==========================================
# IoT Dashboard - Google Cloud Infrastructure
# 🔒 Secure + 🌐 Custom Domain + 💰 Budget-friendly
# Cloud Run services deployed via GitHub Actions
# ==========================================

terraform {
  required_version = ">= 1.0"
  
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# ==========================================
# Enable APIs
# ==========================================

resource "google_project_service" "apis" {
  for_each = toset([
    "run.googleapis.com",
    "sqladmin.googleapis.com",
    "secretmanager.googleapis.com",
    "artifactregistry.googleapis.com",
    "cloudbuild.googleapis.com",
    "compute.googleapis.com",
    "iamcredentials.googleapis.com",
    "dns.googleapis.com",
    "domains.googleapis.com",
  ])
  
  service            = each.value
  disable_on_destroy = false
}

# ==========================================
# Workload Identity Federation (GitHub Actions)
# ==========================================

resource "google_iam_workload_identity_pool" "github_pool" {
  workload_identity_pool_id = "github-actions-pool"
  display_name              = "GitHub Actions Pool"
  description               = "Identity pool for GitHub Actions"
  
  depends_on = [google_project_service.apis]
}

resource "google_iam_workload_identity_pool_provider" "github_provider" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github_pool.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-provider"
  display_name                       = "GitHub Provider"
  
  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.actor"      = "assertion.actor"
    "attribute.repository" = "assertion.repository"
    "attribute.ref"        = "assertion.ref"
  }
  
  attribute_condition = "assertion.repository == '${var.github_repo}'"
  
  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

# Service Account для GitHub Actions
resource "google_service_account" "github_actions_sa" {
  account_id   = "github-actions-sa"
  display_name = "GitHub Actions Service Account"
}

# Дозволяємо GitHub Actions використовувати SA
resource "google_service_account_iam_member" "github_actions_wif" {
  service_account_id = google_service_account.github_actions_sa.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github_pool.name}/attribute.repository/${var.github_repo}"
}

# Права для GitHub Actions SA
resource "google_project_iam_member" "github_actions_ar_writer" {
  project = var.project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${google_service_account.github_actions_sa.email}"
}

resource "google_project_iam_member" "github_actions_run_admin" {
  project = var.project_id
  role    = "roles/run.admin"
  member  = "serviceAccount:${google_service_account.github_actions_sa.email}"
}

resource "google_project_iam_member" "github_actions_sa_user" {
  project = var.project_id
  role    = "roles/iam.serviceAccountUser"
  member  = "serviceAccount:${google_service_account.github_actions_sa.email}"
}

# Дозволяємо GHA SA використовувати Cloud Run SA
resource "google_service_account_iam_member" "github_actions_use_cloudrun_sa" {
  service_account_id = google_service_account.cloudrun_sa.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.github_actions_sa.email}"
}

# ==========================================
# Artifact Registry
# ==========================================

resource "google_artifact_registry_repository" "iot_repo" {
  location      = var.region
  repository_id = "iot-dashboard"
  format        = "DOCKER"
  description   = "IoT Dashboard Docker images"
  
  depends_on = [google_project_service.apis]
}

# ==========================================
# Secret Manager
# ==========================================

resource "google_secret_manager_secret" "google_client_id" {
  secret_id = "google-client-id"
  replication { 
    auto {} 
    }
  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret_version" "google_client_id" {
  secret      = google_secret_manager_secret.google_client_id.id
  secret_data = var.google_client_id
}

resource "google_secret_manager_secret" "google_client_secret" {
  secret_id = "google-client-secret"
  replication { 
    auto {} 
    }
  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret_version" "google_client_secret" {
  secret      = google_secret_manager_secret.google_client_secret.id
  secret_data = var.google_client_secret
}

resource "google_secret_manager_secret" "jwt_secret" {
  secret_id = "jwt-secret"
  replication { 
    auto {} 
    }
  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret_version" "jwt_secret" {
  secret      = google_secret_manager_secret.jwt_secret.id
  secret_data = var.jwt_secret
}

resource "google_secret_manager_secret" "db_password" {
  secret_id = "db-password"
  replication { 
    auto {} 
    }
  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret_version" "db_password" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = var.db_password
}

# ==========================================
# Cloud SQL PostgreSQL
# Приватний доступ через Cloud SQL Auth Proxy (вбудований в Cloud Run)
# ==========================================

resource "google_sql_database_instance" "postgres" {
  name             = "iot-dashboard-db"
  database_version = "POSTGRES_15"
  region           = var.region
  
  settings {
    tier              = "db-f1-micro"
    edition           = "ENTERPRISE"
    availability_type = "ZONAL"
    
    disk_size       = 10
    disk_type       = "PD_HDD"
    disk_autoresize = false
    
    backup_configuration {
      enabled = false
    }
    
    # Public IP але доступ тільки через Cloud SQL Auth Proxy!
    ip_configuration {
      ipv4_enabled = true
      
      # Забороняємо прямий доступ з інтернету
      # Cloud Run підключається через вбудований Auth Proxy
    }
    
    insights_config {
      query_insights_enabled = false
    }
  }
  
  deletion_protection = false
  
  depends_on = [google_project_service.apis]
}

resource "google_sql_database" "iot_database" {
  name     = "iot_dashboard"
  instance = google_sql_database_instance.postgres.name
}

resource "google_sql_user" "postgres_user" {
  name     = "iot_user"
  instance = google_sql_database_instance.postgres.name
  password = var.db_password
}

# ==========================================
# Service Account для Cloud Run (використовується при деплої з GHA)
# ==========================================

resource "google_service_account" "cloudrun_sa" {
  account_id   = "iot-cloudrun-sa"
  display_name = "IoT Dashboard Cloud Run SA"
}

resource "google_secret_manager_secret_iam_member" "secrets_access" {
  for_each = toset([
    google_secret_manager_secret.google_client_id.secret_id,
    google_secret_manager_secret.google_client_secret.secret_id,
    google_secret_manager_secret.jwt_secret.secret_id,
    google_secret_manager_secret.db_password.secret_id,
  ])
  
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.cloudrun_sa.email}"
}

# Cloud SQL Client role - дозволяє підключатись через Auth Proxy
resource "google_project_iam_member" "cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.cloudrun_sa.email}"
}

# ==========================================
# Serverless NEG (Network Endpoint Groups)
# Посилаються на Cloud Run сервіси по імені
# Сервіси будуть створені через GitHub Actions
# ==========================================

resource "google_compute_region_network_endpoint_group" "backend_neg" {
  name                  = "iot-backend-neg"
  network_endpoint_type = "SERVERLESS"
  region                = var.region
  
  cloud_run {
    service = "iot-backend"  # Ім'я сервісу, який створить GHA
  }
  
  depends_on = [google_project_service.apis]
}

resource "google_compute_region_network_endpoint_group" "frontend_neg" {
  name                  = "iot-frontend-neg"
  network_endpoint_type = "SERVERLESS"
  region                = var.region
  
  cloud_run {
    service = "iot-frontend"  # Ім'я сервісу, який створить GHA
  }
  
  depends_on = [google_project_service.apis]
}

# ==========================================
# Backend Services
# ==========================================

resource "google_compute_backend_service" "backend" {
  name                  = "iot-backend-service"
  protocol              = "HTTP"
  port_name             = "http"
  timeout_sec           = 30
  load_balancing_scheme = "EXTERNAL_MANAGED"
  
  backend {
    group = google_compute_region_network_endpoint_group.backend_neg.id
  }
}

resource "google_compute_backend_service" "frontend" {
  name                  = "iot-frontend-service"
  protocol              = "HTTP"
  port_name             = "http"
  timeout_sec           = 30
  load_balancing_scheme = "EXTERNAL_MANAGED"
  
  backend {
    group = google_compute_region_network_endpoint_group.frontend_neg.id
  }
}

# ==========================================
# URL Map (Path Routing)
# /api/* → Backend
# /*     → Frontend
# ==========================================

resource "google_compute_url_map" "urlmap" {
  name            = "iot-urlmap"
  default_service = google_compute_backend_service.frontend.id
  
  host_rule {
    hosts        = [var.domain]
    path_matcher = "iot-paths"
  }
  
  path_matcher {
    name            = "iot-paths"
    default_service = google_compute_backend_service.frontend.id
    
    path_rule {
      paths   = ["/api/*"]
      service = google_compute_backend_service.backend.id
    }
    
    path_rule {
      paths   = ["/health"]
      service = google_compute_backend_service.backend.id
    }
  }
}

# ==========================================
# SSL Certificate (Managed)
# ==========================================

resource "google_compute_managed_ssl_certificate" "ssl_cert" {
  name = "iot-ssl-cert-v2"
  
  managed {
    domains = [var.domain]
  }
}

# ==========================================
# HTTPS Proxy + HTTP Redirect
# ==========================================

resource "google_compute_target_https_proxy" "https_proxy" {
  name             = "iot-https-proxy"
  url_map          = google_compute_url_map.urlmap.id
  ssl_certificates = [google_compute_managed_ssl_certificate.ssl_cert.id]
}

resource "google_compute_url_map" "http_redirect" {
  name = "iot-http-redirect"
  
  default_url_redirect {
    https_redirect         = true
    redirect_response_code = "MOVED_PERMANENTLY_DEFAULT"
    strip_query            = false
  }
}

resource "google_compute_target_http_proxy" "http_proxy" {
  name    = "iot-http-proxy"
  url_map = google_compute_url_map.http_redirect.id
}

# ==========================================
# Global IP
# ==========================================

resource "google_compute_global_address" "lb_ip" {
  name = "iot-lb-ip"
}

# ==========================================
# Forwarding Rules
# ==========================================

resource "google_compute_global_forwarding_rule" "https" {
  name                  = "iot-https-rule"
  ip_protocol           = "TCP"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  port_range            = "443"
  target                = google_compute_target_https_proxy.https_proxy.id
  ip_address            = google_compute_global_address.lb_ip.id
}

resource "google_compute_global_forwarding_rule" "http" {
  name                  = "iot-http-rule"
  ip_protocol           = "TCP"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  port_range            = "80"
  target                = google_compute_target_http_proxy.http_proxy.id
  ip_address            = google_compute_global_address.lb_ip.id
}

# ==========================================
# Cloud DNS
# ==========================================

# Отримуємо кореневий домен з повного домену
# Наприклад: iot.example.com -> example.com
locals {
  domain_parts = split(".", var.domain)
  # Якщо домен має 2 частини (example.com), використовуємо його
  # Якщо більше (iot.example.com), беремо останні 2
  root_domain = length(local.domain_parts) <= 2 ? var.domain : join(".", slice(local.domain_parts, length(local.domain_parts) - 2, length(local.domain_parts)))
  # Субдомен (якщо є)
  subdomain = length(local.domain_parts) > 2 ? join(".", slice(local.domain_parts, 0, length(local.domain_parts) - 2)) : ""
}

# DNS Zone - створюємо нову або використовуємо існуючу (від Cloud Domains)
resource "google_dns_managed_zone" "main" {
  count = var.create_dns_zone ? 1 : 0
  
  name        = var.dns_zone_name
  dns_name    = "${local.root_domain}."
  description = "DNS zone for ${local.root_domain}"
  
  depends_on = [google_project_service.apis]
}

# Data source для існуючої зони (Cloud Domains)
data "google_dns_managed_zone" "existing" {
  count = var.create_dns_zone ? 0 : 1
  name  = var.dns_zone_name
}

locals {
  # Вибираємо зону: створену нами або існуючу
  dns_zone_name = var.create_dns_zone ? google_dns_managed_zone.main[0].name : data.google_dns_managed_zone.existing[0].name
}

# A Record - вказує домен на Load Balancer IP
resource "google_dns_record_set" "a_record" {
  name         = "${var.domain}."
  type         = "A"
  ttl          = 300
  managed_zone = local.dns_zone_name
  
  rrdatas = [google_compute_global_address.lb_ip.address]
}

# WWW Record (CNAME)
resource "google_dns_record_set" "www_record" {
  count = local.subdomain == "" ? 1 : 0
  
  name         = "www.${var.domain}."
  type         = "CNAME"
  ttl          = 300
  managed_zone = local.dns_zone_name
  
  rrdatas = ["${var.domain}."]
}
