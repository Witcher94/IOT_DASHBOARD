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

	// Initialize DESFire service
	desfireService, err := services.NewDesfireService(cfg.DesfireMasterKey)
	if err != nil {
		log.Printf("Warning: DESFire service not initialized: %v", err)
		desfireService = nil
	}

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(cfg, db, authService)
	deviceHandler := handlers.NewDeviceHandler(db, deviceService, hub, alertingService)
	dashboardHandler := handlers.NewDashboardHandler(db)
	adminHandler := handlers.NewAdminHandler(db)
	wsHandler := handlers.NewWebSocketHandler(hub)
	gatewayHandler := handlers.NewGatewayHandler(db, hub)
	skudHandler := handlers.NewSKUDHandler(db, hub)
	
	// DESFire handler (only if service is available)
	var desfireHandler *handlers.DesfireHandler
	if desfireService != nil {
		desfireHandler = handlers.NewDesfireHandler(db, hub, desfireService)
	}

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
				devices.DELETE("/:id/commands/:commandId", deviceHandler.CancelCommand)
				devices.PUT("/:id/alerts", deviceHandler.UpdateAlertSettings)
				// Sharing
				devices.POST("/:id/shares", deviceHandler.ShareDevice)
				devices.GET("/:id/shares", deviceHandler.GetDeviceShares)
				devices.DELETE("/:id/shares/:userId", deviceHandler.DeleteDeviceShare)
			}

			// Shared devices
			protected.GET("/shared-devices", deviceHandler.GetSharedDevices)

			// Dashboard routes
			dashboard := protected.Group("/dashboard")
			{
				dashboard.GET("/stats", dashboardHandler.GetStats)
			}

			// Admin routes
			admin := protected.Group("/admin")
			admin.Use(middleware.AdminMiddleware())
			{
				admin.GET("/users", adminHandler.GetAllUsers)
				admin.DELETE("/users/:id", adminHandler.DeleteUser)
				admin.PUT("/users/:id/role", adminHandler.UpdateUserRole)
				admin.GET("/users/:id/devices", adminHandler.GetUserDevices)
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
			esp.POST("/metrics/batch", gatewayHandler.ReceiveBatchMetrics)
			esp.POST("/gateway/metrics", gatewayHandler.ReceiveBatchMetrics) // Alias for gateway
			esp.GET("/devices/commands", deviceHandler.GetDeviceCommands)
			esp.GET("/commands/pending", gatewayHandler.GetPendingCommands)
			esp.POST("/devices/commands/:id/ack", deviceHandler.AcknowledgeCommand)
		}

		// Gateway topology routes (requires user auth)
		gateways := protected.Group("/gateways")
		{
			gateways.GET("/:id/topology", gatewayHandler.GetGatewayTopology)
			gateways.POST("/:id/nodes/:nodeId/commands", gatewayHandler.SendCommandToMeshNode)
		}

		// SKUD (Access Control) routes - protected (requires auth)
		// Note: SKUD devices are now created via regular Devices page
		skud := protected.Group("/skud")
		{
			// Cards management
			skud.GET("/cards", skudHandler.GetCards)
			skud.GET("/cards/:id", skudHandler.GetCard)
			skud.PUT("/cards/:id", skudHandler.UpdateCard)
			skud.PATCH("/cards/:id/status", skudHandler.UpdateCardStatus)
			skud.DELETE("/cards/:id", skudHandler.DeleteCard)

			// Card-Device links
			skud.POST("/cards/:id/devices/:device_id", skudHandler.LinkCardToDevice)
			skud.DELETE("/cards/:id/devices/:device_id", skudHandler.UnlinkCardFromDevice)

			// Access logs
			skud.GET("/logs", skudHandler.GetAccessLogs)
		}
	}

	// SKUD ESP device endpoints (device auth via X-Device-Token)
	// SKUD devices use challenge-response: GET /challenge first, then POST /verify with challenge
	skudApi := v1.Group("/access")
	{
		skudApi.GET("/challenge", skudHandler.GetChallenge)  // Get one-time challenge (SKUD only)
		skudApi.POST("/verify", skudHandler.VerifyAccess)    // Verify card access
		skudApi.POST("/register", skudHandler.RegisterCard)  // Register new card
		
		// DESFire cloud authentication endpoints
		if desfireHandler != nil {
			skudApi.POST("/desfire/start", desfireHandler.DesfireStart)      // Start DESFire auth session
			skudApi.POST("/desfire/step", desfireHandler.DesfireStep)        // Process auth step
			skudApi.POST("/desfire/confirm", desfireHandler.DesfireProvisionConfirm) // Confirm provisioning
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
