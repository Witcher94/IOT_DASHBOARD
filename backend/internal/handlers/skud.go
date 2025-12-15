package handlers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

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
	handler := &SKUDHandler{db: db, hub: hub}
	// Start background challenge cleanup
	go handler.startChallengeCleanup()
	return handler
}

// startChallengeCleanup periodically cleans up expired challenges
func (h *SKUDHandler) startChallengeCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if err := h.db.CleanupExpiredChallenges(context.Background()); err != nil {
			log.Printf("[SKUD] Failed to cleanup challenges: %v", err)
		}
	}
}

// getDeviceFromToken validates X-Device-Token and returns the device
func (h *SKUDHandler) getDeviceFromToken(c *gin.Context) (*models.Device, error) {
	token := c.GetHeader("X-Device-Token")
	if token == "" {
		return nil, nil
	}
	return h.db.GetDeviceByToken(c.Request.Context(), token)
}

// validateAndLockChipID validates the X-Chip-ID header against the stored chip_id
// For SKUD devices:
//   - If chip_id is confirmed: must match exactly or CLONE_DETECTED
//   - If chip_id is not set: save as pending_chip_id for user to confirm in UI
//
// Returns: (valid bool, error string) - valid=true if chip_id matches or is pending
func (h *SKUDHandler) validateAndLockChipID(c *gin.Context, device *models.Device) (bool, string) {
	// Only validate chip_id for SKUD devices
	if device.DeviceType != models.DeviceTypeSKUD {
		return true, ""
	}

	chipID := c.GetHeader("X-Chip-ID")
	
	// Log current chip_id state for debugging
	confirmedChip := ""
	pendingChip := ""
	if device.ChipID != nil {
		confirmedChip = *device.ChipID
	}
	if device.PendingChipID != nil {
		pendingChip = *device.PendingChipID
	}
	log.Printf("[SKUD CHIP] Device: %s | Received: %s | Confirmed: %s | Pending: %s", 
		device.Name, chipID, confirmedChip, pendingChip)
	
	if chipID == "" {
		// No chip_id provided - for backwards compatibility, allow but log warning
		log.Printf("[SKUD CHIP] ⚠ WARNING: Device %s sent request WITHOUT X-Chip-ID header!", device.Name)
		return true, ""
	}

	// Case 1: Device has confirmed chip_id - must match exactly
	if device.ChipID != nil && *device.ChipID != "" {
		if *device.ChipID != chipID {
			// CLONE DETECTED!
			log.Printf("[SKUD] ⚠️ CLONE DETECTED! Device %s expected chip_id=%s, got chip_id=%s",
				device.Name, *device.ChipID, chipID)

			// Log the clone attempt
			h.logAccess(c, device.Name, "", "", "clone_attempt", "clone_detected", false)

			// Broadcast alert via WebSocket
			if h.hub != nil {
				h.hub.BroadcastAccessLog(map[string]interface{}{
					"type":          "clone_alert",
					"device_name":   device.Name,
					"device_id":     device.ID,
					"expected_chip": *device.ChipID,
					"received_chip": chipID,
					"message":       "Possible device clone detected!",
				})
			}

			return false, "CLONE_DETECTED"
		}
		// Chip ID matches - all good
		return true, ""
	}

	// Case 2: No confirmed chip_id - check/update pending
	if device.PendingChipID == nil || *device.PendingChipID == "" || *device.PendingChipID != chipID {
		// New or different chip_id - save as pending
		if err := h.db.SetPendingChipID(c.Request.Context(), device.ID, chipID); err != nil {
			log.Printf("[SKUD] Failed to set pending chip_id for device %s: %v", device.Name, err)
			return false, "Failed to register device hardware"
		}
		log.Printf("[SKUD] Device %s: pending chip_id set to %s (awaiting confirmation)", device.Name, chipID)
		device.PendingChipID = &chipID

		// Broadcast device update for user to confirm chip_id in dashboard
		if h.hub != nil {
			h.hub.BroadcastDeviceUpdate("chip_id_pending", map[string]interface{}{
				"id":              device.ID,
				"name":            device.Name,
				"device_type":     device.DeviceType,
				"pending_chip_id": chipID,
				"message":         "New device hardware detected - please confirm in dashboard",
			})
		}
	}

	// Allow request while pending (device can work, but user should confirm)
	return true, ""
}

// ==================== Challenge Endpoint (for SKUD devices) ====================

// GetChallenge generates a one-time challenge for SKUD device authentication
// GET /access/challenge
func (h *SKUDHandler) GetChallenge(c *gin.Context) {
	log.Printf("[SKUD AUTH] ════════ GET CHALLENGE REQUEST ════════")
	
	// Authenticate device via X-Device-Token
	device, err := h.getDeviceFromToken(c)
	if device == nil || err != nil {
		log.Printf("[SKUD AUTH] ❌ Challenge rejected: invalid or missing device token")
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Invalid or missing device token",
			"error_code": "INVALID_DEVICE_TOKEN",
		})
		return
	}

	log.Printf("[SKUD AUTH] Step 1: Device authenticated: %s (type: %s)", device.Name, device.DeviceType)

	// Only SKUD devices need challenges
	if device.DeviceType != models.DeviceTypeSKUD {
		log.Printf("[SKUD AUTH] ❌ Challenge rejected: device %s is not SKUD type", device.Name)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Challenge not required for this device type",
			"error_code": "NOT_SKUD_DEVICE",
		})
		return
	}

	// Validate and lock chip_id (clone protection)
	chipID := c.GetHeader("X-Chip-ID")
	log.Printf("[SKUD AUTH] Step 2: Validating chip_id: %s", chipID)
	if valid, errCode := h.validateAndLockChipID(c, device); !valid {
		log.Printf("[SKUD AUTH] ❌ Challenge rejected: chip_id mismatch - %s", errCode)
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "Device hardware mismatch - possible clone detected",
			"error_code": errCode,
		})
		return
	}

	// Update device online status
	if err := h.db.UpdateDeviceOnline(c.Request.Context(), device.ID, true); err != nil {
		log.Printf("[SKUD AUTH] Warning: Failed to update device online status: %v", err)
	}

	// Generate and store challenge
	challenge, err := h.db.CreateChallenge(c.Request.Context(), device.ID)
	if err != nil {
		log.Printf("[SKUD AUTH] ❌ Failed to create challenge for device %s: %v", device.Name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate challenge"})
		return
	}

	log.Printf("[SKUD AUTH] ════════ ESP32 DEVICE CHALLENGE (NOT card!) ════════")
	log.Printf("[SKUD AUTH] Device: %s", device.Name)
	log.Printf("[SKUD AUTH] Challenge (32-char hex): %s", challenge)
	log.Printf("[SKUD AUTH] Purpose: Verify ESP32 device authenticity")
	log.Printf("[SKUD AUTH] NOTE: This is DIFFERENT from DESFire card crypto challenge!")
	log.Printf("[SKUD AUTH] Expires in: 30 seconds")
	log.Printf("[SKUD AUTH] ════════ Waiting for verify request ════════")

	c.JSON(http.StatusOK, models.ChallengeResponse{
		Challenge: challenge,
		ExpiresIn: 30, // seconds
	})
}

// ==================== Card Endpoints ====================
// Note: Access devices are now created through the regular Devices page.
// SKUD uses X-Device-Token (same as IoT devices) for authentication.

func (h *SKUDHandler) GetCards(c *gin.Context) {
	status := c.Query("status")
	deviceID := c.Query("device_id")

	var cards []models.Card
	var err error

	// Parse device UUID if provided
	var deviceUUID *uuid.UUID
	if deviceID != "" {
		parsed, parseErr := uuid.Parse(deviceID)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device_id", "error_code": "INVALID_DEVICE_ID"})
			return
		}
		deviceUUID = &parsed
	}

	// Validate status if provided
	if status != "" && status != models.CardStatusPending && status != models.CardStatusActive && status != models.CardStatusDisabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status", "error_code": "INVALID_STATUS"})
		return
	}

	// Get cards with filters
	cards, err = h.db.GetCardsFiltered(c.Request.Context(), status, deviceUUID)

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

	// Include current token
	token, err := h.db.GetCurrentCardToken(c.Request.Context(), id)
	if err == nil {
		card.Token = token
	}

	c.JSON(http.StatusOK, card)
}

// RegenerateCardToken generates a new token for a card
// Old token remains valid for 24 hours for smooth transition
// POST /skud/cards/:id/token
func (h *SKUDHandler) RegenerateCardToken(c *gin.Context) {
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

	// Generate new token (rotates old one with 24h expiry)
	token, err := h.db.CreateCardToken(c.Request.Context(), id, true)
	if err != nil {
		log.Printf("[SKUD] Failed to regenerate token for card %s: %v", card.CardUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to regenerate token"})
		return
	}

	log.Printf("[SKUD] Token regenerated for card %s", card.CardUID)
	
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"message": "Token regenerated. Old token valid for 24 hours.",
	})
}

// RegenerateDesfireKey schedules a DESFire key rotation for a card
// The new key will be written to the card on next authentication
// POST /skud/cards/:id/desfire-key
func (h *SKUDHandler) RegenerateDesfireKey(c *gin.Context) {
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

	// Check if card is DESFire type
	if card.CardType != "MIFARE_DESFIRE" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Key rotation only available for DESFire cards",
			"error_code": "NOT_DESFIRE",
		})
		return
	}

	// Check if already pending
	if card.PendingKeyUpdate {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Key update already pending. Present card to reader to complete.",
			"error_code": "ALREADY_PENDING",
		})
		return
	}

	// Set pending key update (increments key version)
	newVersion, err := h.db.SetPendingKeyUpdate(c.Request.Context(), id)
	if err != nil {
		log.Printf("[SKUD] Failed to set pending key update for card %s: %v", card.CardUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to schedule key rotation"})
		return
	}

	log.Printf("[SKUD] DESFire key rotation scheduled for card %s (v%d -> v%d)", card.CardUID, card.KeyVersion, newVersion)
	
	// Broadcast update
	updatedCard, _ := h.db.GetCardByID(c.Request.Context(), id)
	if updatedCard != nil {
		h.hub.BroadcastCardUpdate("updated", updatedCard)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"key_version": newVersion,
		"message":     "Key rotation scheduled. Present card to reader to apply new key.",
	})
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
	
	// Broadcast card update to WebSocket clients
	if updatedCard != nil {
		h.hub.BroadcastCardUpdate("updated", updatedCard)
	}
	
	c.JSON(http.StatusOK, updatedCard)
}

// UpdateCard updates card details (name, status)
func (h *SKUDHandler) UpdateCard(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid card ID"})
		return
	}

	var req models.UpdateCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate status if provided
	if req.Status != nil {
		if *req.Status != models.CardStatusPending && *req.Status != models.CardStatusActive && *req.Status != models.CardStatusDisabled {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status", "error_code": "INVALID_STATUS"})
			return
		}
	}

	card, err := h.db.GetCardByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Card not found", "error_code": "CARD_NOT_FOUND"})
		return
	}

	if err := h.db.UpdateCard(c.Request.Context(), id, req.Name, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated card
	updatedCard, _ := h.db.GetCardByID(c.Request.Context(), id)
	log.Printf("[SKUD] Card updated: %s (name=%v)", card.CardUID, req.Name)
	
	// Broadcast card update to WebSocket clients
	if updatedCard != nil {
		h.hub.BroadcastCardUpdate("updated", updatedCard)
	}
	
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

// ==================== Card-Device Links ====================

// LinkCardToDevice links a card to a device
// POST /skud/cards/:id/devices/:device_id
func (h *SKUDHandler) LinkCardToDevice(c *gin.Context) {
	cardID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid card ID"})
		return
	}

	deviceID, err := uuid.Parse(c.Param("device_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	// Check if card exists
	card, err := h.db.GetCardByID(c.Request.Context(), cardID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Card not found", "error_code": "CARD_NOT_FOUND"})
		return
	}

	// Check if device exists and is SKUD type
	device, err := h.db.GetDeviceByID(c.Request.Context(), deviceID)
	if err != nil || device == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found", "error_code": "DEVICE_NOT_FOUND"})
		return
	}

	if device.DeviceType != models.DeviceTypeSKUD {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Device is not a SKUD device", "error_code": "NOT_SKUD_DEVICE"})
		return
	}

	// Link card to device
	if err := h.db.LinkCardToDevice(c.Request.Context(), cardID, deviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated card
	updatedCard, _ := h.db.GetCardByID(c.Request.Context(), cardID)
	log.Printf("[SKUD] Card %s linked to device %s", card.CardUID, device.Name)
	
	// Broadcast card update to WebSocket clients
	if updatedCard != nil {
		h.hub.BroadcastCardUpdate("updated", updatedCard)
	}
	
	c.JSON(http.StatusOK, updatedCard)
}

// UnlinkCardFromDevice unlinks a card from a device
// DELETE /skud/cards/:id/devices/:device_id
func (h *SKUDHandler) UnlinkCardFromDevice(c *gin.Context) {
	cardID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid card ID"})
		return
	}

	deviceID, err := uuid.Parse(c.Param("device_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	// Check if card exists
	card, err := h.db.GetCardByID(c.Request.Context(), cardID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Card not found", "error_code": "CARD_NOT_FOUND"})
		return
	}

	// Unlink card from device
	if err := h.db.UnlinkCardFromDevice(c.Request.Context(), cardID, deviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated card
	updatedCard, _ := h.db.GetCardByID(c.Request.Context(), cardID)
	log.Printf("[SKUD] Card %s unlinked from device %s", card.CardUID, deviceID)
	
	// Broadcast card update to WebSocket clients
	if updatedCard != nil {
		h.hub.BroadcastCardUpdate("updated", updatedCard)
	}
	
	c.JSON(http.StatusOK, updatedCard)
}

// ==================== ESP Device Access Endpoints ====================
// These endpoints use X-Device-Token (same token as IoT devices)
// SKUD devices require challenge-response authentication

func (h *SKUDHandler) VerifyAccess(c *gin.Context) {
	log.Printf("[SKUD VERIFY] ════════ VERIFY ACCESS REQUEST ════════")
	
	// Authenticate device via X-Device-Token
	device, err := h.getDeviceFromToken(c)
	if device == nil || err != nil {
		log.Printf("[SKUD VERIFY] ❌ Step 1 FAILED: Invalid or missing device token")
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Invalid or missing device token",
			"error_code": "INVALID_DEVICE_TOKEN",
			"access":     false,
		})
		return
	}
	log.Printf("[SKUD VERIFY] ✓ Step 1: Device authenticated: %s (type: %s)", device.Name, device.DeviceType)

	// Validate chip_id (clone protection) - must match before processing any request
	chipID := c.GetHeader("X-Chip-ID")
	log.Printf("[SKUD VERIFY] Step 2: Validating chip_id: %s", chipID)
	if valid, errCode := h.validateAndLockChipID(c, device); !valid {
		log.Printf("[SKUD VERIFY] ❌ Step 2 FAILED: chip_id mismatch - %s", errCode)
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "Device hardware mismatch - possible clone detected",
			"error_code": errCode,
			"access":     false,
		})
		return
	}
	log.Printf("[SKUD VERIFY] ✓ Step 2: Chip ID validated")

	var req models.AccessVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[SKUD VERIFY] ❌ Step 3 FAILED: Invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid request: card_uid required",
			"error_code": "INVALID_REQUEST",
		})
		return
	}
	
	// Log the full request for debugging
	hasToken := req.CardToken != ""
	hasChallenge := req.Challenge != ""
	log.Printf("[SKUD VERIFY] ✓ Step 3: Request parsed - card_uid=%s card_type=%s has_token=%v has_challenge=%v", 
		req.CardUID, req.CardType, hasToken, hasChallenge)

	// SKUD devices require challenge-response authentication
	if device.DeviceType == models.DeviceTypeSKUD {
		log.Printf("[SKUD VERIFY] Step 4: Challenge-Response validation (SKUD device)")
		if req.Challenge == "" {
			log.Printf("[SKUD VERIFY] ❌ Step 4 FAILED: No challenge provided for SKUD device")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "Challenge required for SKUD devices. Call GET /access/challenge first.",
				"error_code": "CHALLENGE_REQUIRED",
				"access":     false,
			})
			return
		}

		// Validate and consume challenge (one-time use)
		log.Printf("[SKUD VERIFY] ════════ DEVICE CHALLENGE VALIDATION ════════")
		log.Printf("[SKUD VERIFY] Device: %s", device.Name)
		log.Printf("[SKUD VERIFY] Challenge received: %s", req.Challenge)
		if err := h.db.ValidateAndConsumeChallenge(c.Request.Context(), device.ID, req.Challenge); err != nil {
			log.Printf("[SKUD VERIFY] ❌ CHALLENGE INVALID: %v", err)
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "Invalid or expired challenge",
				"error_code": "INVALID_CHALLENGE",
				"access":     false,
			})
			return
		}
		log.Printf("[SKUD VERIFY] ✓ DEVICE CHALLENGE VERIFIED AND CONSUMED (one-time use)")
		log.Printf("[SKUD VERIFY] ════════════════════════════════════════════")
	} else {
		log.Printf("[SKUD VERIFY] Step 4: Skipping challenge validation (device type: %s)", device.DeviceType)
	}

	deviceName := device.Name

	// Get card
	log.Printf("[SKUD VERIFY] Step 5: Looking up card in database: %s", req.CardUID)
	card, err := h.db.GetCardByUID(c.Request.Context(), req.CardUID)
	if err != nil {
		log.Printf("[SKUD VERIFY] ❌ Step 5 FAILED: Card not found in database: %s", req.CardUID)
		h.logAccess(c, deviceName, req.CardUID, req.CardType, "verify", "not_found", false)
		c.JSON(http.StatusOK, models.AccessVerifyResponse{Access: false})
		return
	}
	log.Printf("[SKUD VERIFY] ✓ Step 5: Card found - id=%s status=%s type=%s name=%s", 
		card.ID, card.Status, card.CardType, card.Name)

	// Check if card is active and linked to this device
	linkedToDevice, _ := h.db.IsCardLinkedToDevice(c.Request.Context(), card.ID, device.ID)
	allowed := card.Status == models.CardStatusActive && linkedToDevice
	tokenUpdated := false
	
	log.Printf("[SKUD VERIFY] Step 6: Access check - status=%s linked=%v → allowed=%v", 
		card.Status, linkedToDevice, allowed)

	// Token verification ONLY for SKUD devices
	// Gateway and other devices use simple UID-based verification
	if device.DeviceType == models.DeviceTypeSKUD && allowed {
		// Check if this is a DESFire card
		isDESFire := req.CardType == "MIFARE_DESFIRE" || req.CardType == "DESFIRE" ||
			card.CardType == "MIFARE_DESFIRE" || card.CardType == "DESFIRE"

		log.Printf("[SKUD VERIFY] Step 7: Token verification - isDESFire=%v has_token=%v", isDESFire, hasToken)

		if isDESFire {
			// DESFire cards REQUIRE token authentication on SKUD devices
			if req.CardToken == "" {
				log.Printf("[SKUD VERIFY] ❌ Step 7 FAILED: DESFire card requires token but none provided - device=%s card=%s", deviceName, req.CardUID)
				h.logAccess(c, deviceName, req.CardUID, req.CardType, "verify", "token_required", false)
				c.JSON(http.StatusOK, models.AccessVerifyResponse{Access: false})
				return
			}

			log.Printf("[SKUD VERIFY] Step 7a: Verifying DESFire token: %s...", req.CardToken[:min(8, len(req.CardToken))])
			tokenCard, isCurrent, err := h.db.GetCardByToken(c.Request.Context(), req.CardToken)
			if err != nil || tokenCard == nil || tokenCard.ID != card.ID {
				log.Printf("[SKUD VERIFY] ❌ Step 7 FAILED: DESFire token verification failed - device=%s card=%s token_err=%v token_match=%v", 
					deviceName, req.CardUID, err, tokenCard != nil && tokenCard.ID == card.ID)
				h.logAccess(c, deviceName, req.CardUID, req.CardType, "verify", "invalid_token", false)
				c.JSON(http.StatusOK, models.AccessVerifyResponse{Access: false})
				return
			}
			log.Printf("[SKUD VERIFY] ✓ Step 7: DESFire token verified - is_current=%v", isCurrent)

			// If old token was used successfully, promote it (rotate completed)
			if !isCurrent {
				log.Printf("[SKUD VERIFY] Step 7b: Promoting old token to current (rotation complete)")
				if err := h.db.PromoteCardToken(c.Request.Context(), req.CardToken); err != nil {
					log.Printf("[SKUD VERIFY] Warning: Failed to promote token: %v", err)
				} else {
					log.Printf("[SKUD VERIFY] ✓ Token promoted to current for card %s", req.CardUID)
					tokenUpdated = true
				}
			}
		} else if req.CardToken != "" {
			// Optional token verification for non-DESFire cards on SKUD devices
			log.Printf("[SKUD VERIFY] Step 7: Verifying non-DESFire card token")
			tokenCard, isCurrent, err := h.db.GetCardByToken(c.Request.Context(), req.CardToken)
			if err != nil || tokenCard == nil || tokenCard.ID != card.ID {
				log.Printf("[SKUD VERIFY] ❌ Step 7 FAILED: Card token verification failed - device=%s card=%s", deviceName, req.CardUID)
				h.logAccess(c, deviceName, req.CardUID, req.CardType, "verify", "invalid_token", false)
				c.JSON(http.StatusOK, models.AccessVerifyResponse{Access: false})
				return
			}

			if !isCurrent {
				if err := h.db.PromoteCardToken(c.Request.Context(), req.CardToken); err != nil {
					log.Printf("[SKUD VERIFY] Warning: Failed to promote token: %v", err)
				} else {
					log.Printf("[SKUD VERIFY] ✓ Token promoted for non-DESFire card %s", req.CardUID)
					tokenUpdated = true
				}
			}
		} else {
			log.Printf("[SKUD VERIFY] Step 7: Skipping token verification (not DESFire and no token provided)")
		}
	} else if device.DeviceType != models.DeviceTypeSKUD {
		log.Printf("[SKUD VERIFY] Step 7: Skipping token verification (non-SKUD device: %s)", device.DeviceType)
	}

	log.Printf("[SKUD VERIFY] ════════ RESULT: access=%v tokenUpdated=%v ════════", allowed, tokenUpdated)
	log.Printf("[SKUD VERIFY] Summary: device=%s card=%s (%s) status=%s linked=%v → ACCESS %s",
		deviceName, req.CardUID, card.CardType, card.Status, linkedToDevice, map[bool]string{true: "✓ GRANTED", false: "✗ DENIED"}[allowed])

	h.logAccess(c, deviceName, req.CardUID, req.CardType, "verify", card.Status, allowed)

	// Return card name for ESP display (use name if set, otherwise use card_uid)
	cardName := card.Name
	if cardName == "" {
		cardName = card.CardUID
	}

	c.JSON(http.StatusOK, models.AccessVerifyResponse{
		Access:       allowed,
		CardName:     cardName,
		TokenUpdated: tokenUpdated,
	})
}

func (h *SKUDHandler) RegisterCard(c *gin.Context) {
	log.Printf("[SKUD REGISTER] ════════ REGISTER CARD REQUEST ════════")
	
	// Authenticate device via X-Device-Token
	device, err := h.getDeviceFromToken(c)
	if device == nil || err != nil {
		log.Printf("[SKUD REGISTER] ❌ Device auth failed: invalid or missing token")
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Invalid or missing device token",
			"error_code": "INVALID_DEVICE_TOKEN",
		})
		return
	}
	log.Printf("[SKUD REGISTER] ✓ Device authenticated: %s", device.Name)

	// Validate chip_id (clone protection) - must match before processing any request
	if valid, errCode := h.validateAndLockChipID(c, device); !valid {
		log.Printf("[SKUD REGISTER] ❌ Chip ID validation failed: %s", errCode)
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "Device hardware mismatch - possible clone detected",
			"error_code": errCode,
		})
		return
	}

	var req models.AccessRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[SKUD REGISTER] ❌ Invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid request: card_uid required",
			"error_code": "INVALID_REQUEST",
		})
		return
	}
	log.Printf("[SKUD REGISTER] ✓ Request parsed: card_uid=%s card_type=%s", req.CardUID, req.CardType)

	// SKUD devices require challenge-response authentication
	if device.DeviceType == models.DeviceTypeSKUD {
		log.Printf("[SKUD REGISTER] Validating challenge for SKUD device...")
		if req.Challenge == "" {
			log.Printf("[SKUD REGISTER] ❌ No challenge provided for SKUD device")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "Challenge required for SKUD devices. Call GET /access/challenge first.",
				"error_code": "CHALLENGE_REQUIRED",
			})
			return
		}

		// Validate and consume challenge (one-time use)
		if err := h.db.ValidateAndConsumeChallenge(c.Request.Context(), device.ID, req.Challenge); err != nil {
			log.Printf("[SKUD REGISTER] ❌ Invalid/expired challenge: device=%s error=%v", device.Name, err)
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "Invalid or expired challenge",
				"error_code": "INVALID_CHALLENGE",
			})
			return
		}
		log.Printf("[SKUD REGISTER] ✓ Challenge validated and consumed")
	}

	deviceName := device.Name

	// Check if card exists
	log.Printf("[SKUD REGISTER] Looking up card in database: %s", req.CardUID)
	card, err := h.db.GetCardByUID(c.Request.Context(), req.CardUID)
	if err != nil {
		// Create new card as pending with card type
		log.Printf("[SKUD REGISTER] Card not found - creating new card as PENDING")
		card = &models.Card{
			CardUID:  req.CardUID,
			CardType: req.CardType,
			Status:   models.CardStatusPending,
		}
		if err := h.db.CreateCard(c.Request.Context(), card); err != nil {
			log.Printf("[SKUD REGISTER] ❌ Failed to create card: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[SKUD REGISTER] ✓ NEW CARD created as pending: %s (type: %s)", req.CardUID, req.CardType)
		
		// Broadcast new card to WebSocket clients
		createdCard, _ := h.db.GetCardByUID(c.Request.Context(), req.CardUID)
		if createdCard != nil {
			h.hub.BroadcastCardUpdate("created", createdCard)
			log.Printf("[SKUD REGISTER] ✓ Card update broadcast to WebSocket clients")
		}
	} else {
		// Card exists
		log.Printf("[SKUD REGISTER] Card already exists in database: status=%s", card.Status)
		if card.Status == models.CardStatusPending {
			log.Printf("[SKUD REGISTER] Card is already pending - returning conflict")
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
		log.Printf("[SKUD REGISTER] Linking card to device %s...", device.Name)
		if err := h.db.LinkCardToDevice(c.Request.Context(), card.ID, device.ID); err != nil {
			log.Printf("[SKUD REGISTER] ⚠ Failed to link card to device: %v", err)
		} else {
			log.Printf("[SKUD REGISTER] ✓ Card linked to device")
		}
	} else {
		log.Printf("[SKUD REGISTER] Card already linked to device")
	}

	h.logAccess(c, deviceName, req.CardUID, req.CardType, "register", card.Status, false)

	log.Printf("[SKUD REGISTER] ════════ REGISTRATION COMPLETE: card=%s status=%s device=%s ════════", 
		req.CardUID, card.Status, deviceName)
	c.JSON(http.StatusAccepted, models.AccessRegisterResponse{Status: card.Status})
}

// ==================== Access Logs ====================

func (h *SKUDHandler) GetAccessLogs(c *gin.Context) {
	// Parse filter parameters
	filter := database.AccessLogFilter{
		Action:   c.Query("action"),    // verify, register, card_status, card_delete, desfire_auth, provision, key_rotation, clone_attempt
		CardUID:  c.Query("card_uid"),  // partial match
		DeviceID: c.Query("device_id"), // partial match
		CardType: c.Query("card_type"), // MIFARE_CLASSIC_1K, MIFARE_DESFIRE, etc.
		FromDate: c.Query("from_date"), // ISO date string (YYYY-MM-DD)
		ToDate:   c.Query("to_date"),   // ISO date string (YYYY-MM-DD)
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

