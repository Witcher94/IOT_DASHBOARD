package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/pfaka/iot-dashboard/internal/config"
	"github.com/pfaka/iot-dashboard/internal/database"
	"github.com/pfaka/iot-dashboard/internal/handlers"
	"github.com/pfaka/iot-dashboard/internal/middleware"
	"github.com/pfaka/iot-dashboard/internal/services"
	"github.com/pfaka/iot-dashboard/internal/websocket"
)

func main() {
	// Load .env file
	godotenv.Load()

	// Load configuration
	cfg := config.Load()

	// Connect to database
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	ctx := context.Background()
	if err := db.RunMigrations(ctx); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed")

	// Initialize services
	authService := services.NewAuthService(cfg, db)
	deviceService := services.NewDeviceService(db)

	// Initialize alerting service
	alertConfig := services.AlertConfig{
		Enabled:          cfg.AlertingEnabled,
		CheckInterval:    cfg.AlertCheckInterval,
		OfflineThreshold: cfg.AlertOfflineThreshold,
		AlertCooldown:    cfg.AlertCooldown,
		TempMin:          cfg.TempMin,
		TempMax:          cfg.TempMax,
		HumidityMin:      cfg.HumidityMin,
		HumidityMax:      cfg.HumidityMax,
	}
	alertingService := services.NewAlertingService(db, alertConfig)

	// Initialize WebSocket hub
	hub := websocket.NewHub()
	go hub.Run()

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(cfg, db, authService)
	deviceHandler := handlers.NewDeviceHandler(db, deviceService, hub, alertingService)
	dashboardHandler := handlers.NewDashboardHandler(db)
	wsHandler := handlers.NewWebSocketHandler(hub)

	// Setup Gin router
	router := gin.Default()

	// CORS configuration
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{cfg.FrontendURL, "http://localhost:3000", "http://localhost:5173", "http://localhost"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Device-Token"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Auth routes
		auth := v1.Group("/auth")
		{
			auth.GET("/google", authHandler.GoogleLogin)
			auth.GET("/google/callback", authHandler.GoogleCallback)
			auth.POST("/refresh", authHandler.RefreshToken)
		}

		// Protected routes
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware(cfg))
		{
			// User routes
			protected.GET("/me", authHandler.GetCurrentUser)

			// Device routes
			devices := protected.Group("/devices")
			{
				devices.GET("", deviceHandler.GetDevices)
				devices.POST("", deviceHandler.CreateDevice)
				devices.GET("/:id", deviceHandler.GetDevice)
				devices.DELETE("/:id", deviceHandler.DeleteDevice)
				devices.POST("/:id/regenerate-token", deviceHandler.RegenerateToken)
				devices.GET("/:id/metrics", deviceHandler.GetMetrics)
				devices.POST("/:id/commands", deviceHandler.CreateCommand)
				devices.GET("/:id/commands", deviceHandler.GetCommands)
				devices.PUT("/:id/alerts", deviceHandler.UpdateAlertSettings)
			}

			// Dashboard routes
			dashboard := protected.Group("/dashboard")
			{
				dashboard.GET("/stats", dashboardHandler.GetStats)
			}

			// Admin routes
			admin := protected.Group("/admin")
			admin.Use(middleware.AdminMiddleware())
			{
				admin.GET("/users", dashboardHandler.GetAllUsers)
				admin.GET("/devices", deviceHandler.GetDevices)
			}

			// WebSocket ticket (requires JWT auth)
			protected.POST("/ws/ticket", wsHandler.CreateTicket)
		}

		// WebSocket connection (uses one-time ticket)
		v1.GET("/ws", wsHandler.HandleWebSocket)

		// ESP Device routes (token auth)
		esp := v1.Group("")
		esp.Use(middleware.DeviceAuthMiddleware(db))
		{
			esp.POST("/metrics", deviceHandler.ReceiveMetrics)
			esp.GET("/devices/commands", deviceHandler.GetDeviceCommands)
			esp.POST("/devices/commands/:id/ack", deviceHandler.AcknowledgeCommand)
		}
	}

	// Start alerting service
	go alertingService.Start(ctx)

	// Background job: Mark offline devices
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			if err := db.MarkOfflineDevices(context.Background(), 2*time.Minute); err != nil {
				log.Printf("Error marking offline devices: %v", err)
			}
		}
	}()

	// Background job: Clean old metrics
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			if err := db.DeleteOldMetrics(context.Background(), 7*24*time.Hour); err != nil {
				log.Printf("Error cleaning old metrics: %v", err)
			}
		}
	}()

	// Start server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		log.Printf("Server starting on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	alertingService.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
