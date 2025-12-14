package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port            string
	DatabaseURL     string
	JWTSecret       string
	GoogleClientID  string
	GoogleSecret    string
	GoogleCallback  string
	FrontendURL     string
	AdminEmail      string
	
	// DESFire Master Key (32 hex chars = 16 bytes AES-128)
	DesfireMasterKey string
	
	// Alerting (логування для GCP Cloud Monitoring)
	AlertingEnabled       bool
	AlertCheckInterval    time.Duration
	AlertOfflineThreshold time.Duration
	AlertCooldown         time.Duration
	
	// Threshold alerts
	TempMin     float64
	TempMax     float64
	HumidityMin float64
	HumidityMax float64
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8080"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/iot_dashboard?sslmode=disable"),
		JWTSecret:       getEnv("JWT_SECRET", "your-super-secret-jwt-key-change-in-production"),
		GoogleClientID:  getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleSecret:    getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleCallback:  getEnv("GOOGLE_CALLBACK_URL", "http://localhost:8080/api/v1/auth/google/callback"),
		FrontendURL:     getEnv("FRONTEND_URL", "http://localhost:3000"),
		AdminEmail:      getEnv("ADMIN_EMAIL", "admin@example.com"),
		
		// DESFire Master Key - CHANGE IN PRODUCTION!
		// Default is a random key for development only
		DesfireMasterKey: getEnv("DESFIRE_MASTER_KEY", "0123456789ABCDEF0123456789ABCDEF"),
		
		// Alerting
		AlertingEnabled:       getEnvBool("ALERTING_ENABLED", true),
		AlertCheckInterval:    getEnvDuration("ALERT_CHECK_INTERVAL", 1*time.Minute),
		AlertOfflineThreshold: getEnvDuration("ALERT_OFFLINE_THRESHOLD", 5*time.Minute),
		AlertCooldown:         getEnvDuration("ALERT_COOLDOWN", 30*time.Minute),
		
		// Thresholds
		TempMin:     getEnvFloat("TEMP_MIN", 0),
		TempMax:     getEnvFloat("TEMP_MAX", 40),
		HumidityMin: getEnvFloat("HUMIDITY_MIN", 0),
		HumidityMax: getEnvFloat("HUMIDITY_MAX", 90),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}
