# ==========================================
# Variables
# ==========================================

variable "project_id" {
  description = "GCP Project ID"
  type        = string
}

variable "region" {
  description = "GCP Region"
  type        = string
  default     = "europe-west1"
}

variable "domain" {
  description = "Domain name for the application (e.g., iot.example.com)"
  type        = string
}

variable "google_client_id" {
  description = "Google OAuth Client ID"
  type        = string
  sensitive   = true
}

variable "google_client_secret" {
  description = "Google OAuth Client Secret"
  type        = string
  sensitive   = true
}

variable "admin_email" {
  description = "Admin email address"
  type        = string
}

variable "jwt_secret" {
  description = "JWT Secret (32+ characters)"
  type        = string
  sensitive   = true
}

variable "db_password" {
  description = "PostgreSQL password"
  type        = string
  sensitive   = true
}

variable "github_repo" {
  description = "GitHub repository (owner/repo format)"
  type        = string
}

variable "dns_zone_name" {
  description = "Cloud DNS zone name (only alphanumeric and dashes)"
  type        = string
  default     = "iot-zone"
}

variable "create_dns_zone" {
  description = "Create DNS zone (set to false if zone already exists)"
  type        = bool
  default     = true
}
