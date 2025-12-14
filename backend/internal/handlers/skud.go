package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/database"
	"github.com/pfaka/iot-dashboard/internal/models"
	"github.com/pfaka/iot-dashboard/internal/websocket"
)

type SKUDHandler struct {
	db  *database.DB
	hub *websocket.Hub
}

func NewSKUDHandler(db *database.DB, hub *websocket.Hub) *SKUDHandler {
	return &SKUDHandler{db: db, hub: hub}
}

// ==================== Access Device Endpoints ====================

func (h *SKUDHandler) GetAccessDevices(c *gin.Context) {
	devices, err := h.db.GetAllAccessDevices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if devices == nil {
		devices = []models.AccessDevice{}
	}
	c.JSON(http.StatusOK, devices)
}

func (h *SKUDHandler) CreateAccessDevice(c *gin.Context) {
	var req models.CreateAccessDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "error_code": "INVALID_REQUEST"})
		return
	}

	// Check if device already exists
	existing, _ := h.db.GetAccessDeviceByDeviceID(c.Request.Context(), req.DeviceID)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Device already exists", "error_code": "DEVICE_ALREADY_EXISTS"})
		return
	}

	device := &models.AccessDevice{
		DeviceID:  req.DeviceID,
		SecretKey: req.SecretKey,
		Name:      req.Name,
	}

	if err := h.db.CreateAccessDevice(c.Request.Context(), device); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SKUD] Created access device: %s (%s)", device.DeviceID, device.ID)
	c.JSON(http.StatusCreated, device)
}

func (h *SKUDHandler) DeleteAccessDevice(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	device, err := h.db.GetAccessDeviceByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found", "error_code": "DEVICE_NOT_FOUND"})
		return
	}

	if err := h.db.DeleteAccessDevice(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SKUD] Deleted access device: %s", device.DeviceID)
	c.Status(http.StatusNoContent)
}

// ==================== Card Endpoints ====================

func (h *SKUDHandler) GetCards(c *gin.Context) {
	status := c.Query("status")

	var cards []models.Card
	var err error

	if status != "" {
		// Validate status
		if status != models.CardStatusPending && status != models.CardStatusActive && status != models.CardStatusDisabled {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status", "error_code": "INVALID_STATUS"})
			return
		}
		cards, err = h.db.GetCardsByStatus(c.Request.Context(), status)
	} else {
		cards, err = h.db.GetAllCards(c.Request.Context())
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if cards == nil {
		cards = []models.Card{}
	}
	c.JSON(http.StatusOK, cards)
}

func (h *SKUDHandler) GetCard(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid card ID"})
		return
	}

	card, err := h.db.GetCardByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Card not found", "error_code": "CARD_NOT_FOUND"})
		return
	}

	c.JSON(http.StatusOK, card)
}

func (h *SKUDHandler) UpdateCardStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid card ID"})
		return
	}

	var req models.UpdateCardStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate status
	if req.Status != models.CardStatusPending && req.Status != models.CardStatusActive && req.Status != models.CardStatusDisabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status", "error_code": "INVALID_STATUS"})
		return
	}

	card, err := h.db.GetCardByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Card not found", "error_code": "CARD_NOT_FOUND"})
		return
	}

	if err := h.db.UpdateCardStatus(c.Request.Context(), id, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Log the status change
	deviceID := ""
	if len(card.Devices) > 0 {
		deviceID = card.Devices[0].DeviceID
	}
	h.logAccess(c, deviceID, card.CardUID, "", "card_status", req.Status, req.Status == models.CardStatusActive)

	// Return updated card
	updatedCard, _ := h.db.GetCardByID(c.Request.Context(), id)
	log.Printf("[SKUD] Card status updated: %s -> %s", card.CardUID, req.Status)
	c.JSON(http.StatusOK, updatedCard)
}

func (h *SKUDHandler) DeleteCard(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid card ID"})
		return
	}

	card, err := h.db.GetCardByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Card not found", "error_code": "CARD_NOT_FOUND"})
		return
	}

	if err := h.db.DeleteCard(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Log the deletion
	deviceID := ""
	if len(card.Devices) > 0 {
		deviceID = card.Devices[0].DeviceID
	}
	h.logAccess(c, deviceID, card.CardUID, "", "card_delete", card.Status, false)

	log.Printf("[SKUD] Card deleted: %s", card.CardUID)
	c.Status(http.StatusNoContent)
}

// ==================== ESP Device Access Endpoints ====================

func (h *SKUDHandler) VerifyAccess(c *gin.Context) {
	deviceID := c.GetHeader("X-Device-ID")
	apiKey := c.GetHeader("X-API-Key")

	if deviceID == "" || apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Missing device authentication headers",
			"error_code": "MISSING_DEVICE_HEADERS",
		})
		return
	}

	var req models.AccessVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Missing card_uid or card_type",
			"error_code": "MISSING_CARD_FIELDS",
		})
		return
	}

	// Verify device credentials
	device, err := h.db.GetAccessDeviceByCredentials(c.Request.Context(), deviceID, apiKey)
	if err != nil {
		log.Printf("[SKUD] Unknown device: %s", deviceID)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Unknown device",
			"error_code": "UNKNOWN_DEVICE",
			"access":     false,
		})
		return
	}

	// Get card
	card, err := h.db.GetCardByUID(c.Request.Context(), req.CardUID)
	if err != nil {
		log.Printf("[SKUD] Card not found: %s", req.CardUID)
		h.logAccess(c, deviceID, req.CardUID, req.CardType, "verify", "not_found", false)
		c.JSON(http.StatusOK, models.AccessVerifyResponse{Access: false})
		return
	}

	// Check if card is active and linked to this device
	linkedToDevice, _ := h.db.IsCardLinkedToDevice(c.Request.Context(), card.ID, device.ID)
	allowed := card.Status == models.CardStatusActive && linkedToDevice

	log.Printf("[SKUD] Verify: device=%s card=%s status=%s linked=%v allowed=%v",
		deviceID, req.CardUID, card.Status, linkedToDevice, allowed)

	h.logAccess(c, deviceID, req.CardUID, req.CardType, "verify", card.Status, allowed)

	c.JSON(http.StatusOK, models.AccessVerifyResponse{Access: allowed})
}

func (h *SKUDHandler) RegisterCard(c *gin.Context) {
	deviceID := c.GetHeader("X-Device-ID")
	apiKey := c.GetHeader("X-API-Key")

	if deviceID == "" || apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Missing device authentication headers",
			"error_code": "MISSING_DEVICE_HEADERS",
		})
		return
	}

	var req models.AccessRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Missing card_uid or card_type",
			"error_code": "MISSING_CARD_FIELDS",
		})
		return
	}

	// Verify device credentials
	device, err := h.db.GetAccessDeviceByCredentials(c.Request.Context(), deviceID, apiKey)
	if err != nil {
		log.Printf("[SKUD] Unknown device: %s", deviceID)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Unknown device",
			"error_code": "UNKNOWN_DEVICE",
		})
		return
	}

	// Check if card exists
	card, err := h.db.GetCardByUID(c.Request.Context(), req.CardUID)
	if err != nil {
		// Create new card as pending
		card = &models.Card{
			CardUID: req.CardUID,
			Status:  models.CardStatusPending,
		}
		if err := h.db.CreateCard(c.Request.Context(), card); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[SKUD] New card created as pending: %s", req.CardUID)
	} else {
		// Card exists
		if card.Status == models.CardStatusPending {
			c.JSON(http.StatusConflict, gin.H{
				"error":      "Card is already pending approval",
				"error_code": "CARD_PENDING",
			})
			return
		}
	}

	// Link card to device if not already linked
	linked, _ := h.db.IsCardLinkedToDevice(c.Request.Context(), card.ID, device.ID)
	if !linked {
		if err := h.db.LinkCardToDevice(c.Request.Context(), card.ID, device.ID); err != nil {
			log.Printf("[SKUD] Failed to link card to device: %v", err)
		}
	}

	h.logAccess(c, deviceID, req.CardUID, req.CardType, "register", card.Status, false)

	log.Printf("[SKUD] Register: device=%s card=%s status=%s", deviceID, req.CardUID, card.Status)
	c.JSON(http.StatusAccepted, models.AccessRegisterResponse{Status: card.Status})
}

// ==================== Access Logs ====================

func (h *SKUDHandler) GetAccessLogs(c *gin.Context) {
	// Parse filter parameters
	filter := database.AccessLogFilter{
		Action:   c.Query("action"),   // verify, register, card_status, card_delete
		CardUID:  c.Query("card_uid"), // partial match
		DeviceID: c.Query("device_id"), // partial match
		Limit:    100,
	}

	// Parse allowed filter (true/false/empty)
	if allowedStr := c.Query("allowed"); allowedStr != "" {
		allowed := allowedStr == "true"
		filter.Allowed = &allowed
	}

	// Parse limit
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			filter.Limit = l
		}
	}

	logs, err := h.db.GetAccessLogsFiltered(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if logs == nil {
		logs = []models.AccessLog{}
	}
	c.JSON(http.StatusOK, logs)
}

// ==================== Helpers ====================

func (h *SKUDHandler) logAccess(c *gin.Context, deviceID, cardUID, cardType, action, status string, allowed bool) {
	accessLog := &models.AccessLog{
		DeviceID: deviceID,
		CardUID:  cardUID,
		CardType: cardType,
		Action:   action,
		Status:   status,
		Allowed:  allowed,
	}
	if err := h.db.CreateAccessLog(c.Request.Context(), accessLog); err != nil {
		log.Printf("[SKUD] Failed to create access log: %v", err)
		return
	}

	// Broadcast via WebSocket to all connected clients
	if h.hub != nil {
		h.hub.BroadcastAccessLog(accessLog)
	}
}

// ValidCardStatuses returns list of valid card statuses
func ValidCardStatuses() []string {
	return []string{models.CardStatusPending, models.CardStatusActive, models.CardStatusDisabled}
}

