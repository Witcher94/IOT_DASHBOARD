package config

import (
	"os"
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
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

