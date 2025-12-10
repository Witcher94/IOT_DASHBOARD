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
	db              *database.DB
	deviceService   *services.DeviceService
	hub             *websocket.Hub
	alertingService *services.AlertingService
}

func NewDeviceHandler(db *database.DB, deviceService *services.DeviceService, hub *websocket.Hub, alertingService *services.AlertingService) *DeviceHandler {
	return &DeviceHandler{
		db:              db,
		deviceService:   deviceService,
		hub:             hub,
		alertingService: alertingService,
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

	// Return device with token (only on creation)
	response := models.DeviceWithToken{
		Device: *device,
		Token:  device.Token,
	}
	c.JSON(http.StatusCreated, response)
}

func (h *DeviceHandler) GetDevices(c *gin.Context) {
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")

	var devices []models.Device
	var err error

	// Адмін бачить всі пристрої, звичайний юзер - тільки свої
	if isAdmin.(bool) {
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

func (h *DeviceHandler) CancelCommand(c *gin.Context) {
	commandID, err := uuid.Parse(c.Param("commandId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid command ID"})
		return
	}

	// Get command to check ownership
	cmd, err := h.db.GetCommandByID(c.Request.Context(), commandID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Command not found"})
		return
	}

	// Only pending commands can be cancelled
	if cmd.Status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only pending commands can be cancelled"})
		return
	}

	// Get device to check ownership
	device, err := h.deviceService.GetDevice(c.Request.Context(), cmd.DeviceID)
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

	if err := h.db.DeleteCommand(c.Request.Context(), commandID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Command cancelled"})
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

func (h *DeviceHandler) UpdateAlertSettings(c *gin.Context) {
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

	var req models.UpdateAlertSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.UpdateAlertSettings(c.Request.Context(), deviceID, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch updated device
	updatedDevice, _ := h.deviceService.GetDevice(c.Request.Context(), deviceID)
	c.JSON(http.StatusOK, updatedDevice)
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

	// Update alerting service with device last seen
	if h.alertingService != nil {
		h.alertingService.UpdateDeviceLastSeen(dev.ID.String())

		// Check metric thresholds
		metric := &models.Metric{
			Temperature: payload.Temperature,
			Humidity:    payload.Humidity,
		}
		h.alertingService.CheckMetricThresholds(dev, metric)
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
