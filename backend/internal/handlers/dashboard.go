package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/database"
	"github.com/pfaka/iot-dashboard/internal/models"
)

type DashboardHandler struct {
	db *database.DB
}

func NewDashboardHandler(db *database.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

func (h *DashboardHandler) GetStats(c *gin.Context) {
	ctx := c.Request.Context()
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")

	var totalDevices, onlineDevices int
	var avgTemp, avgHumidity float64

	if isAdmin.(bool) {
		// Admin sees global stats
		totalDevices, _ = h.db.GetDevicesCount(ctx)
		onlineDevices, _ = h.db.GetOnlineDevicesCount(ctx)
		avgTemp, _ = h.db.GetAvgTemperature(ctx)
		avgHumidity, _ = h.db.GetAvgHumidity(ctx)
	} else {
		// Regular user sees only their devices
		totalDevices, _ = h.db.GetDevicesCountByUser(ctx, userID.(uuid.UUID))
		onlineDevices, _ = h.db.GetOnlineDevicesCountByUser(ctx, userID.(uuid.UUID))
		avgTemp, _ = h.db.GetAvgTemperatureByUser(ctx, userID.(uuid.UUID))
		avgHumidity, _ = h.db.GetAvgHumidityByUser(ctx, userID.(uuid.UUID))
	}

	totalUsers, _ := h.db.GetUsersCount(ctx)

	stats := models.DashboardStats{
		TotalDevices:  totalDevices,
		OnlineDevices: onlineDevices,
		TotalUsers:    totalUsers,
		AvgTemp:       avgTemp,
		AvgHumidity:   avgHumidity,
	}

	c.JSON(http.StatusOK, stats)
}

func (h *DashboardHandler) GetAllUsers(c *gin.Context) {
	users, err := h.db.GetAllUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

