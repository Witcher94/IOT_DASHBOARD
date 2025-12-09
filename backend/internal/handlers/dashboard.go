package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
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

	totalDevices, _ := h.db.GetDevicesCount(ctx)
	onlineDevices, _ := h.db.GetOnlineDevicesCount(ctx)
	totalUsers, _ := h.db.GetUsersCount(ctx)
	avgTemp, _ := h.db.GetAvgTemperature(ctx)
	avgHumidity, _ := h.db.GetAvgHumidity(ctx)

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

