package handlers

import (
	"log"
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

	deviceType := req.DeviceType
	if deviceType == "" {
		deviceType = models.DeviceTypeSimple
	}

	log.Printf("[DEVICE] Creating %s device '%s' for user %s", deviceType, req.Name, userID)

	device, err := h.deviceService.CreateDevice(c.Request.Context(), userID.(uuid.UUID), req.Name, deviceType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEVICE] Created device %s (type: %s, token: %s...)", device.ID, device.DeviceType, device.Token[:8])

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

	log.Printf("[CMD TRACE] CreateCommand called for device %s", deviceID)

	device, err := h.deviceService.GetDevice(c.Request.Context(), deviceID)
	if err != nil {
		log.Printf("[CMD TRACE] Device not found: %s", deviceID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	// Check ownership
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")
	if device.UserID != userID.(uuid.UUID) && !isAdmin.(bool) {
		log.Printf("[CMD TRACE] Access denied for user %s on device %s", userID, deviceID)
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var req models.CreateCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[CMD TRACE] Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[CMD TRACE] Creating command '%s' for device %s (%s)", req.Command, deviceID, device.Name)

	cmd, err := h.deviceService.CreateCommand(c.Request.Context(), deviceID, &req)
	if err != nil {
		log.Printf("[CMD TRACE] Failed to create command: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[CMD TRACE] Command created: ID=%s, Status=%s", cmd.ID, cmd.Status)
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

	log.Printf("[CMD TRACE] GetDeviceCommands called by device %s (%s)", dev.ID, dev.Name)

	cmd, err := h.deviceService.GetPendingCommand(c.Request.Context(), dev.ID)
	if err != nil {
		log.Printf("[CMD TRACE] No pending commands for device %s", dev.ID)
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	log.Printf("[CMD TRACE] Returning pending command: ID=%s, Cmd=%s", cmd.ID, cmd.Command)

	// Mark as sent (ID is already a string from GetPendingCommand)
	cmdUUID, err := uuid.Parse(cmd.ID)
	if err == nil {
		if err := h.db.MarkCommandSent(c.Request.Context(), cmdUUID); err != nil {
			log.Printf("[CMD TRACE] Failed to mark command as sent: %v", err)
		} else {
			log.Printf("[CMD TRACE] Command %s marked as 'sent'", cmd.ID)
		}
	}

	c.JSON(http.StatusOK, cmd)
}

func (h *DeviceHandler) AcknowledgeCommand(c *gin.Context) {
	commandID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid command ID"})
		return
	}

	log.Printf("[CMD TRACE] AcknowledgeCommand called for command %s", commandID)

	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[CMD TRACE] Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[CMD TRACE] Acknowledging command %s with status '%s'", commandID, req.Status)

	if err := h.deviceService.AcknowledgeCommand(c.Request.Context(), commandID, req.Status); err != nil {
		log.Printf("[CMD TRACE] Failed to acknowledge: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[CMD TRACE] Command %s acknowledged successfully", commandID)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ShareDevice shares a device with another user
func (h *DeviceHandler) ShareDevice(c *gin.Context) {
	userID, _ := c.Get("user_id")
	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device ID"})
		return
	}

	// Check if user owns the device
	device, err := h.db.GetDeviceByID(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if device.UserID != userID.(uuid.UUID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "you can only share your own devices"})
		return
	}

	var req models.ShareDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user by email
	targetUser, err := h.db.GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found with this email"})
		return
	}

	if targetUser.ID == userID.(uuid.UUID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot share with yourself"})
		return
	}

	permission := req.Permission
	if permission == "" {
		permission = "view"
	}

	share := &models.DeviceShare{
		DeviceID:     deviceID,
		OwnerID:      userID.(uuid.UUID),
		SharedWithID: targetUser.ID,
		Permission:   permission,
	}

	if err := h.db.CreateDeviceShare(c.Request.Context(), share); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "already shared with this user"})
		return
	}

	share.SharedWithName = targetUser.Name
	share.SharedWithEmail = targetUser.Email

	log.Printf("[SHARE] Device %s shared with %s (%s) by %s", deviceID, targetUser.Email, permission, userID)
	c.JSON(http.StatusCreated, share)
}

// GetDeviceShares returns all shares for a device
func (h *DeviceHandler) GetDeviceShares(c *gin.Context) {
	userID, _ := c.Get("user_id")
	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device ID"})
		return
	}

	// Check if user owns the device
	device, err := h.db.GetDeviceByID(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if device.UserID != userID.(uuid.UUID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only owner can view shares"})
		return
	}

	shares, err := h.db.GetDeviceShares(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if shares == nil {
		shares = []models.DeviceShare{}
	}
	c.JSON(http.StatusOK, shares)
}

// DeleteDeviceShare removes sharing access
func (h *DeviceHandler) DeleteDeviceShare(c *gin.Context) {
	userID, _ := c.Get("user_id")
	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device ID"})
		return
	}

	sharedWithID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	// Check if user owns the device
	device, err := h.db.GetDeviceByID(c.Request.Context(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if device.UserID != userID.(uuid.UUID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only owner can remove shares"})
		return
	}

	if err := h.db.DeleteDeviceShare(c.Request.Context(), deviceID, sharedWithID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SHARE] Device %s unshared with %s by %s", deviceID, sharedWithID, userID)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetSharedDevices returns devices shared with current user
func (h *DeviceHandler) GetSharedDevices(c *gin.Context) {
	userID, _ := c.Get("user_id")

	devices, err := h.db.GetSharedDevices(c.Request.Context(), userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if devices == nil {
		devices = []models.Device{}
	}
	c.JSON(http.StatusOK, devices)
}
