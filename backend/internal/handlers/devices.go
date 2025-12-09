package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/database"
	"github.com/pfaka/iot-dashboard/internal/models"
	"github.com/pfaka/iot-dashboard/internal/services"
	"github.com/pfaka/iot-dashboard/internal/websocket"
)

type DeviceHandler struct {
	db            *database.DB
	deviceService *services.DeviceService
	hub           *websocket.Hub
}

func NewDeviceHandler(db *database.DB, deviceService *services.DeviceService, hub *websocket.Hub) *DeviceHandler {
	return &DeviceHandler{
		db:            db,
		deviceService: deviceService,
		hub:           hub,
	}
}

func (h *DeviceHandler) CreateDevice(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req models.CreateDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, err := h.deviceService.CreateDevice(c.Request.Context(), userID.(uuid.UUID), req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, device)
}

func (h *DeviceHandler) GetDevices(c *gin.Context) {
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")

	var devices []models.Device
	var err error

	if isAdmin.(bool) && c.Query("all") == "true" {
		devices, err = h.db.GetAllDevices(c.Request.Context())
	} else {
		devices, err = h.deviceService.GetUserDevices(c.Request.Context(), userID.(uuid.UUID))
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, devices)
}

func (h *DeviceHandler) GetDevice(c *gin.Context) {
	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	device, err := h.deviceService.GetDevice(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	// Check ownership
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")
	if device.UserID != userID.(uuid.UUID) && !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, device)
}

func (h *DeviceHandler) DeleteDevice(c *gin.Context) {
	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	device, err := h.deviceService.GetDevice(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	// Check ownership
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")
	if device.UserID != userID.(uuid.UUID) && !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	if err := h.deviceService.DeleteDevice(c.Request.Context(), deviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Device deleted"})
}

func (h *DeviceHandler) RegenerateToken(c *gin.Context) {
	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	device, err := h.deviceService.GetDevice(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	// Check ownership
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")
	if device.UserID != userID.(uuid.UUID) && !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	newToken, err := h.deviceService.RegenerateToken(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": newToken})
}

func (h *DeviceHandler) GetMetrics(c *gin.Context) {
	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	device, err := h.deviceService.GetDevice(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	// Check ownership
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")
	if device.UserID != userID.(uuid.UUID) && !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Check for period
	periodStr := c.Query("period")
	if periodStr != "" {
		period, err := time.ParseDuration(periodStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid period format"})
			return
		}
		end := time.Now()
		start := end.Add(-period)
		metrics, err := h.deviceService.GetMetricsForPeriod(c.Request.Context(), deviceID, start, end)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, metrics)
		return
	}

	metrics, err := h.deviceService.GetMetrics(c.Request.Context(), deviceID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

func (h *DeviceHandler) CreateCommand(c *gin.Context) {
	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	device, err := h.deviceService.GetDevice(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	// Check ownership
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")
	if device.UserID != userID.(uuid.UUID) && !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var req models.CreateCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cmd, err := h.deviceService.CreateCommand(c.Request.Context(), deviceID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, cmd)
}

func (h *DeviceHandler) GetCommands(c *gin.Context) {
	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	device, err := h.deviceService.GetDevice(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	// Check ownership
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")
	if device.UserID != userID.(uuid.UUID) && !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	commands, err := h.db.GetCommandsByDeviceID(c.Request.Context(), deviceID, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, commands)
}

// ESP Device Endpoints

func (h *DeviceHandler) ReceiveMetrics(c *gin.Context) {
	device, _ := c.Get("device")
	dev := device.(*models.Device)

	var payload models.DeviceMetricsPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.deviceService.ProcessMetrics(c.Request.Context(), dev, &payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Broadcast via WebSocket
	h.hub.BroadcastMetrics(dev.UserID, dev.ID, payload)
	h.hub.BroadcastDeviceStatus(dev.UserID, dev.ID, true)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *DeviceHandler) GetDeviceCommands(c *gin.Context) {
	device, _ := c.Get("device")
	dev := device.(*models.Device)

	cmd, err := h.deviceService.GetPendingCommand(c.Request.Context(), dev.ID)
	if err != nil {
		// No pending commands
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	c.JSON(http.StatusOK, cmd)
}

func (h *DeviceHandler) AcknowledgeCommand(c *gin.Context) {
	commandID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid command ID"})
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.deviceService.AcknowledgeCommand(c.Request.Context(), commandID, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

