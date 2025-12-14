package handlers

import (
	"encoding/hex"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pfaka/iot-dashboard/internal/database"
	"github.com/pfaka/iot-dashboard/internal/models"
	"github.com/pfaka/iot-dashboard/internal/services"
	"github.com/pfaka/iot-dashboard/internal/websocket"
)

type DesfireHandler struct {
	db      *database.DB
	hub     *websocket.Hub
	desfire *services.DesfireService
}

func NewDesfireHandler(db *database.DB, hub *websocket.Hub, desfire *services.DesfireService) *DesfireHandler {
	return &DesfireHandler{
		db:      db,
		hub:     hub,
		desfire: desfire,
	}
}

// getDeviceFromToken validates X-Device-Token and returns the device
func (h *DesfireHandler) getDeviceFromToken(c *gin.Context) (*models.Device, error) {
	token := c.GetHeader("X-Device-Token")
	if token == "" {
		return nil, nil
	}
	return h.db.GetDeviceByToken(c.Request.Context(), token)
}

// DesfireStart handles POST /access/desfire/start
// Begins a new DESFire authentication session
func (h *DesfireHandler) DesfireStart(c *gin.Context) {
	// Authenticate device
	device, err := h.getDeviceFromToken(c)
	if device == nil || err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Invalid or missing device token",
			"error_code": "INVALID_DEVICE_TOKEN",
		})
		return
	}

	var req models.DesfireStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid request: card_uid required",
			"error_code": "INVALID_REQUEST",
		})
		return
	}

	// Normalize card UID (remove spaces, uppercase)
	cardUID := strings.ToUpper(strings.ReplaceAll(req.CardUID, " ", ""))

	log.Printf("[DESFire] Start auth for card %s on device %s", cardUID, device.Name)

	// Check if this is a DESFire card
	if req.CardType != "" && !strings.Contains(strings.ToUpper(req.CardType), "DESFIRE") {
		// Not a DESFire card - use regular flow
		c.JSON(http.StatusOK, services.DesfireStartResponse{
			Status:    "not_desfire",
			Reason:    "Card is not DESFire type",
			TimeoutMs: 5000,
		})
		return
	}

	// Look up card in database
	card, err := h.db.GetCardByUID(c.Request.Context(), cardUID)
	
	if err != nil {
		// Card not found - this is a new card, need to provision
		log.Printf("[DESFire] Card %s not found - starting cloud provisioning", cardUID)
		
		session := h.desfire.CreateSession(cardUID, device.ID, true)
		
		// Start provisioning and get first command
		firstCmd := h.desfire.StartProvisioning(session)
		
		c.JSON(http.StatusOK, services.DesfireStartResponse{
			SessionID: session.ID,
			Status:    "provision",
			Command:   firstCmd,
			TimeoutMs: 30000, // 30 seconds for provisioning
		})
		return
	}

	// Card exists - check status and device link
	if card.Status == models.CardStatusDisabled {
		log.Printf("[DESFire] Card %s is disabled", cardUID)
		c.JSON(http.StatusOK, services.DesfireStartResponse{
			Status:    "denied",
			Reason:    "card_disabled",
			TimeoutMs: 5000,
		})
		return
	}

	// Check if card is linked to this device
	isLinked := false
	for _, d := range card.Devices {
		if d.ID == device.ID {
			isLinked = true
			break
		}
	}

	if !isLinked && len(card.Devices) > 0 {
		// Card is linked to other devices, not this one
		log.Printf("[DESFire] Card %s not linked to device %s", cardUID, device.Name)
		c.JSON(http.StatusOK, services.DesfireStartResponse{
			Status:    "denied",
			Reason:    "wrong_device",
			TimeoutMs: 5000,
		})
		return
	}

	if card.Status == models.CardStatusPending {
		log.Printf("[DESFire] Card %s is pending activation", cardUID)
		c.JSON(http.StatusOK, services.DesfireStartResponse{
			Status:    "denied",
			Reason:    "card_pending",
			TimeoutMs: 5000,
		})
		return
	}

	// Card is active and (linked to this device OR not linked anywhere) - start authentication
	session := h.desfire.CreateSession(cardUID, device.ID, false)
	session.State = services.DesfireStateSelectApp

	c.JSON(http.StatusOK, services.DesfireStartResponse{
		SessionID: session.ID,
		Status:    "auth_required",
		Command:   h.desfire.BuildSelectAppCommand(),
		TimeoutMs: 10000, // 10 seconds for auth
	})
}

// DesfireStep handles POST /access/desfire/step
// Processes the next step in DESFire authentication
func (h *DesfireHandler) DesfireStep(c *gin.Context) {
	// Authenticate device
	device, err := h.getDeviceFromToken(c)
	if device == nil || err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Invalid or missing device token",
			"error_code": "INVALID_DEVICE_TOKEN",
		})
		return
	}

	var req models.DesfireStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid request",
			"error_code": "INVALID_REQUEST",
		})
		return
	}

	// Get session
	session, err := h.desfire.GetSession(req.SessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      err.Error(),
			"error_code": "SESSION_ERROR",
		})
		return
	}

	// Verify device owns this session
	if session.DeviceID != device.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "Session belongs to different device",
			"error_code": "SESSION_MISMATCH",
		})
		return
	}

	// Parse response (remove spaces, handle status byte)
	responseHex := strings.ToUpper(strings.ReplaceAll(req.Response, " ", ""))
	response, err := hex.DecodeString(responseHex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid hex response",
			"error_code": "INVALID_HEX",
		})
		return
	}

	log.Printf("[DESFire] Step for session %s, state: %s, response: %s", session.ID, session.State, responseHex)

	// Handle provisioning states first
	if session.IsProvisioning {
		cmd, status, err := h.desfire.ProcessProvisioningStep(session, responseHex)
		
		if err != nil {
			log.Printf("[DESFire] Provisioning error: %v", err)
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "error",
				Reason:  "provisioning_failed",
				Message: err.Error(),
			})
			return
		}
		
		if status == "provisioned" {
			// Key written successfully! Register card in database
			log.Printf("[DESFire] Card %s provisioned successfully, registering...", session.CardUID)
			
			card := &models.Card{
				CardUID:  session.CardUID,
				CardType: "MIFARE_DESFIRE",
				Status:   models.CardStatusPending,
			}
			
			if err := h.db.CreateCard(c.Request.Context(), card); err != nil {
				log.Printf("[DESFire] Error creating card: %v", err)
			}
			
			// Broadcast new card
			createdCard, _ := h.db.GetCardByUID(c.Request.Context(), session.CardUID)
			if createdCard != nil {
				h.hub.BroadcastCardUpdate("created", createdCard)
			}
			
			// Log the registration
			h.db.CreateAccessLog(c.Request.Context(), &models.AccessLog{
				DeviceID: device.Name,
				CardUID:  session.CardUID,
				CardType: "MIFARE_DESFIRE",
				Action:   "provision",
				Status:   "key_written",
				Allowed:  false,
			})
			
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "provisioned",
				Message: "Card provisioned successfully, awaiting activation",
			})
			return
		}
		
		if status == "error" {
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "error",
				Reason:  "provisioning_error",
				Message: "Provisioning failed",
			})
			return
		}
		
		// Continue provisioning
		c.JSON(http.StatusOK, services.DesfireStepResponse{
			Status:  "continue",
			Command: cmd,
		})
		return
	}

	// Handle authentication states
	switch session.State {
	case services.DesfireStateSelectApp:
		// Response should be 0x00 (OK) or error
		if len(response) == 0 || response[0] != 0x00 {
			// App might not exist yet
			if len(response) >= 2 && response[0] == 0x91 && response[1] == 0xA0 {
				// Application not found - card not provisioned correctly
				log.Printf("[DESFire] App not found on card %s - needs re-provisioning", session.CardUID)
				h.desfire.DeleteSession(session.ID)
				c.JSON(http.StatusOK, services.DesfireStepResponse{
					Status:  "error",
					Reason:  "app_not_found",
					Message: "Card needs provisioning",
				})
				return
			}
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "error",
				Reason:  "select_app_failed",
				Message: "Failed to select application",
			})
			return
		}

		// App selected successfully, start authentication
		session.State = services.DesfireStateAuth1
		c.JSON(http.StatusOK, services.DesfireStepResponse{
			Status:  "continue",
			Command: h.desfire.BuildAuth1Command(0x00), // Key 0
		})
		return

	case services.DesfireStateAuth1:
		// Response should be 0xAF + EncRndB (16 bytes)
		if len(response) < 17 || response[0] != 0xAF {
			log.Printf("[DESFire] Unexpected auth1 response: %s", responseHex)
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "error",
				Reason:  "auth1_failed",
				Message: "Authentication step 1 failed",
			})
			return
		}

		// Extract EncRndB
		encRndB := hex.EncodeToString(response[1:17])
		
		// Process and generate response
		cmd, err := h.desfire.ProcessAuth1Response(session, encRndB)
		if err != nil {
			log.Printf("[DESFire] ProcessAuth1Response error: %v", err)
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "error",
				Reason:  "crypto_error",
				Message: err.Error(),
			})
			return
		}

		session.State = services.DesfireStateAuth2
		c.JSON(http.StatusOK, services.DesfireStepResponse{
			Status:  "continue",
			Command: cmd,
		})
		return

	case services.DesfireStateAuth2:
		// Verify the final response
		verified, err := h.desfire.VerifyAuth2Response(session, responseHex)
		if err != nil {
			log.Printf("[DESFire] VerifyAuth2Response error: %v", err)
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "error",
				Reason:  "verification_error",
				Message: err.Error(),
			})
			return
		}

		h.desfire.DeleteSession(session.ID)

		if !verified {
			// CLONE DETECTED!
			log.Printf("[DESFire] CLONE DETECTED for card %s!", session.CardUID)
			
			// Log the clone attempt
			h.db.CreateAccessLog(c.Request.Context(), &models.AccessLog{
				DeviceID: device.Name,
				CardUID:  session.CardUID,
				CardType: "MIFARE_DESFIRE",
				Action:   "desfire_auth",
				Status:   "clone_detected",
				Allowed:  false,
			})

			// Broadcast alert
			h.hub.BroadcastAccessLog(map[string]interface{}{
				"type":     "clone_alert",
				"card_uid": session.CardUID,
				"device":   device.Name,
			})

			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "denied",
				Reason:  "clone_detected",
				Message: "Card authentication failed - possible clone",
			})
			return
		}

		// SUCCESS! Card authenticated
		log.Printf("[DESFire] Card %s authenticated successfully!", session.CardUID)

		// Get card info for response
		card, _ := h.db.GetCardByUID(c.Request.Context(), session.CardUID)
		cardName := ""
		if card != nil {
			cardName = card.Name
			if cardName == "" {
				cardName = card.CardUID
			}
		}

		// Log success
		h.db.CreateAccessLog(c.Request.Context(), &models.AccessLog{
			DeviceID: device.Name,
			CardUID:  session.CardUID,
			CardType: "MIFARE_DESFIRE",
			Action:   "desfire_auth",
			Status:   "authenticated",
			Allowed:  true,
		})

		// Broadcast to websocket
		h.hub.BroadcastAccessLog(map[string]interface{}{
			"type":     "access",
			"card_uid": session.CardUID,
			"device":   device.Name,
			"allowed":  true,
			"action":   "desfire_auth",
		})

		c.JSON(http.StatusOK, services.DesfireStepResponse{
			Status:   "granted",
			CardName: cardName,
			Message:  "Card authenticated successfully",
		})
		return

	default:
		h.desfire.DeleteSession(session.ID)
		c.JSON(http.StatusOK, services.DesfireStepResponse{
			Status:  "error",
			Reason:  "invalid_state",
			Message: "Invalid session state",
		})
	}
}

// DesfireProvisionConfirm handles POST /access/desfire/confirm
// Confirms successful provisioning of a card
func (h *DesfireHandler) DesfireProvisionConfirm(c *gin.Context) {
	// Authenticate device
	device, err := h.getDeviceFromToken(c)
	if device == nil || err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Invalid or missing device token",
			"error_code": "INVALID_DEVICE_TOKEN",
		})
		return
	}

	var req models.DesfireProvisionConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid request",
			"error_code": "INVALID_REQUEST",
		})
		return
	}

	// Get session
	session, err := h.desfire.GetSession(req.SessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      err.Error(),
			"error_code": "SESSION_ERROR",
		})
		return
	}

	defer h.desfire.DeleteSession(session.ID)

	if !session.IsProvisioning {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Session is not a provisioning session",
			"error_code": "NOT_PROVISIONING",
		})
		return
	}

	if !req.Success {
		log.Printf("[DESFire] Provisioning failed for card %s", session.CardUID)
		c.JSON(http.StatusOK, gin.H{
			"status":  "failed",
			"message": "Provisioning was not successful",
		})
		return
	}

	// Create the card in database
	card := &models.Card{
		CardUID:  session.CardUID,
		CardType: "MIFARE_DESFIRE",
		Status:   models.CardStatusPending,
	}

	if err := h.db.CreateCard(c.Request.Context(), card); err != nil {
		// Card might already exist
		log.Printf("[DESFire] Error creating card: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"status":  "exists",
			"message": "Card already registered",
		})
		return
	}

	log.Printf("[DESFire] Card %s provisioned and registered!", session.CardUID)

	// Broadcast new card
	createdCard, _ := h.db.GetCardByUID(c.Request.Context(), session.CardUID)
	if createdCard != nil {
		h.hub.BroadcastCardUpdate("created", createdCard)
	}

	// Log the registration
	h.db.CreateAccessLog(c.Request.Context(), &models.AccessLog{
		DeviceID: device.Name,
		CardUID:  session.CardUID,
		CardType: "MIFARE_DESFIRE",
		Action:   "register",
		Status:   "provisioned",
		Allowed:  false, // Card is pending, not yet allowed
	})

	c.JSON(http.StatusOK, gin.H{
		"status":  "registered",
		"message": "Card provisioned and registered successfully",
	})
}

