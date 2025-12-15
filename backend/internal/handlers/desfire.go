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

// validateAndLockChipID validates the X-Chip-ID header against the stored chip_id
// For SKUD devices:
//   - If chip_id is confirmed: must match exactly or CLONE_DETECTED
//   - If chip_id is not set: save as pending_chip_id for user to confirm in UI
//
// Returns: (valid bool, error string) - valid=true if chip_id matches or is pending
func (h *DesfireHandler) validateAndLockChipID(c *gin.Context, device *models.Device) (bool, string) {
	// Only validate chip_id for SKUD devices
	if device.DeviceType != models.DeviceTypeSKUD {
		return true, ""
	}

	chipID := c.GetHeader("X-Chip-ID")
	if chipID == "" {
		// No chip_id provided - for backwards compatibility, allow but log warning
		log.Printf("[DESFire] WARNING: Device %s (%s) sent request without X-Chip-ID header", device.Name, device.ID)
		return true, ""
	}

	// Case 1: Device has confirmed chip_id - must match exactly
	if device.ChipID != nil && *device.ChipID != "" {
		if *device.ChipID != chipID {
			// CLONE DETECTED!
			log.Printf("[DESFire] ⚠️ CLONE DETECTED! Device %s expected chip_id=%s, got chip_id=%s",
				device.Name, *device.ChipID, chipID)

			// Log the clone attempt
			h.db.CreateAccessLog(c.Request.Context(), &models.AccessLog{
				DeviceID: device.Name,
				CardUID:  "",
				CardType: "DEVICE_CLONE",
				Action:   "clone_attempt",
				Status:   "clone_detected",
				Allowed:  false,
			})

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
			log.Printf("[DESFire] Failed to set pending chip_id for device %s: %v", device.Name, err)
			return false, "Failed to register device hardware"
		}
		log.Printf("[DESFire] Device %s: pending chip_id set to %s (awaiting confirmation)", device.Name, chipID)
		device.PendingChipID = &chipID

		// Broadcast notification for user to confirm
		if h.hub != nil {
			h.hub.BroadcastAccessLog(map[string]interface{}{
				"type":        "chip_id_pending",
				"device_name": device.Name,
				"device_id":   device.ID,
				"chip_id":     chipID,
				"message":     "New device hardware detected - please confirm in dashboard",
			})
		}
	}

	// Allow request while pending (device can work, but user should confirm)
	return true, ""
}

// DesfireStart handles POST /access/desfire/start
// Begins a new DESFire authentication session
func (h *DesfireHandler) DesfireStart(c *gin.Context) {
	log.Printf("[DESFire START] ════════ DESFIRE AUTH START ════════")
	
	// Authenticate device
	device, err := h.getDeviceFromToken(c)
	if device == nil || err != nil {
		log.Printf("[DESFire START] ❌ Device auth failed: invalid or missing token")
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Invalid or missing device token",
			"error_code": "INVALID_DEVICE_TOKEN",
		})
		return
	}
	log.Printf("[DESFire START] ✓ Device authenticated: %s (type: %s)", device.Name, device.DeviceType)

	// Validate chip_id (clone protection) - must match before processing any request
	chipID := c.GetHeader("X-Chip-ID")
	log.Printf("[DESFire START] Validating chip_id: %s", chipID)
	if valid, errCode := h.validateAndLockChipID(c, device); !valid {
		log.Printf("[DESFire START] ❌ Chip ID validation failed: %s", errCode)
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "Device hardware mismatch - possible clone detected",
			"error_code": errCode,
		})
		return
	}
	log.Printf("[DESFire START] ✓ Chip ID validated")

	var req models.DesfireStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DESFire START] ❌ Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid request: card_uid required",
			"error_code": "INVALID_REQUEST",
		})
		return
	}

	// Normalize card UID (remove spaces, uppercase)
	cardUID := strings.ToUpper(strings.ReplaceAll(req.CardUID, " ", ""))

	log.Printf("[DESFire START] ✓ Request: card_uid=%s card_type=%s device=%s", cardUID, req.CardType, device.Name)

	// Check if this is a DESFire card
	if req.CardType != "" && !strings.Contains(strings.ToUpper(req.CardType), "DESFIRE") {
		// Not a DESFire card - use regular flow
		log.Printf("[DESFire START] Card type %s is not DESFire - skipping DESFire flow", req.CardType)
		c.JSON(http.StatusOK, services.DesfireStartResponse{
			Status:    "not_desfire",
			Reason:    "Card is not DESFire type",
			TimeoutMs: 5000,
		})
		return
	}

	// Look up card in database
	log.Printf("[DESFire START] Looking up card in database...")
	card, err := h.db.GetCardByUID(c.Request.Context(), cardUID)
	
	if err != nil {
		// Card not found - this is a new card, need to provision
		log.Printf("[DESFire START] ⚡ NEW CARD: %s not found - starting cloud provisioning", cardUID)
		
		session := h.desfire.CreateSession(cardUID, device.ID, true)
		log.Printf("[DESFire START] Created provisioning session: %s", session.ID)
		
		// Start provisioning and get first command
		firstCmd := h.desfire.StartProvisioning(session)
		log.Printf("[DESFire START] Provisioning started - first command: %s", firstCmd)
		log.Printf("[DESFire START] ════════ PROVISIONING SESSION STARTED ════════")
		
		c.JSON(http.StatusOK, services.DesfireStartResponse{
			SessionID: session.ID,
			Status:    "provision",
			Command:   firstCmd,
			TimeoutMs: 30000, // 30 seconds for provisioning
		})
		return
	}
	
	log.Printf("[DESFire START] ✓ Card found: id=%s status=%s type=%s key_v=%d pending_update=%v", 
		card.ID, card.Status, card.CardType, card.KeyVersion, card.PendingKeyUpdate)

	// Card exists - check status and device link
	if card.Status == models.CardStatusDisabled {
		log.Printf("[DESFire START] ❌ Card %s is DISABLED - access denied", cardUID)
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
	log.Printf("[DESFire START] Device link check: linked=%v (linked to %d devices)", isLinked, len(card.Devices))

	if !isLinked && len(card.Devices) > 0 {
		// Card is linked to other devices, not this one
		log.Printf("[DESFire START] ❌ Card %s not linked to device %s - access denied", cardUID, device.Name)
		c.JSON(http.StatusOK, services.DesfireStartResponse{
			Status:    "denied",
			Reason:    "wrong_device",
			TimeoutMs: 5000,
		})
		return
	}

	if card.Status == models.CardStatusPending {
		log.Printf("[DESFire START] ⏳ Card %s is PENDING activation - access denied", cardUID)
		c.JSON(http.StatusOK, services.DesfireStartResponse{
			Status:    "denied",
			Reason:    "card_pending",
			TimeoutMs: 5000,
		})
		return
	}

	// Card is active and (linked to this device OR not linked anywhere) - start authentication
	// Check if key update is pending
	session := h.desfire.CreateSessionWithKeyInfo(cardUID, device.ID, false, card.KeyVersion, card.PendingKeyUpdate)
	log.Printf("[DESFire START] Created auth session: %s (key_version=%d)", session.ID, card.KeyVersion)
	
	if card.PendingKeyUpdate {
		log.Printf("[DESFire START] 🔑 KEY UPDATE PENDING: v%d -> v%d", card.KeyVersion-1, card.KeyVersion)
		session.State = services.DesfireStateSelectApp
		// Will auth with old key first, then update to new key
		log.Printf("[DESFire START] ════════ KEY UPDATE SESSION STARTED ════════")
		c.JSON(http.StatusOK, services.DesfireStartResponse{
			SessionID: session.ID,
			Status:    "key_update",
			Command:   h.desfire.BuildSelectAppCommand(),
			TimeoutMs: 30000, // 30 seconds for key update
		})
		return
	}
	
	session.State = services.DesfireStateSelectApp
	log.Printf("[DESFire START] ════════ AUTH SESSION STARTED - Waiting for steps ════════")

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
		log.Printf("[DESFire STEP] ❌ Device auth failed")
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Invalid or missing device token",
			"error_code": "INVALID_DEVICE_TOKEN",
		})
		return
	}

	// Validate chip_id (clone protection) - must match before processing any request
	if valid, errCode := h.validateAndLockChipID(c, device); !valid {
		log.Printf("[DESFire STEP] ❌ Chip ID validation failed: %s", errCode)
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "Device hardware mismatch - possible clone detected",
			"error_code": errCode,
		})
		return
	}

	var req models.DesfireStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DESFire STEP] ❌ Invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid request",
			"error_code": "INVALID_REQUEST",
		})
		return
	}

	// Get session
	session, err := h.desfire.GetSession(req.SessionID)
	if err != nil {
		log.Printf("[DESFire STEP] ❌ Session not found: %s", req.SessionID)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      err.Error(),
			"error_code": "SESSION_ERROR",
		})
		return
	}

	// Verify device owns this session
	if session.DeviceID != device.ID {
		log.Printf("[DESFire STEP] ❌ Session mismatch: session belongs to different device")
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
		log.Printf("[DESFire STEP] ❌ Invalid hex response: %s", responseHex)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid hex response",
			"error_code": "INVALID_HEX",
		})
		return
	}

	log.Printf("[DESFire STEP] ════ Session: %s | State: %s | Card: %s | Device: %s ════", 
		session.ID[:8], session.State, session.CardUID, device.Name)
	log.Printf("[DESFire STEP] Card response: %s (len=%d)", responseHex, len(response))

	// Handle provisioning states first
	if session.IsProvisioning {
		log.Printf("[DESFire STEP] Processing PROVISIONING step...")
		cmd, status, err := h.desfire.ProcessProvisioningStep(session, responseHex)
		
		if err != nil {
			log.Printf("[DESFire STEP] ❌ PROVISIONING ERROR: %v", err)
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
			log.Printf("[DESFire STEP] ✓✓✓ PROVISIONING COMPLETE: Card %s provisioned successfully!", session.CardUID)
			log.Printf("[DESFire STEP] Registering card in database...")
			
			card := &models.Card{
				CardUID:  session.CardUID,
				CardType: "MIFARE_DESFIRE",
				Status:   models.CardStatusPending,
			}
			
			if err := h.db.CreateCard(c.Request.Context(), card); err != nil {
				log.Printf("[DESFire STEP] ⚠ Error creating card in DB: %v", err)
			} else {
				log.Printf("[DESFire STEP] ✓ Card registered in database with PENDING status")
			}
			
			// Get created card to link to device
			createdCard, _ := h.db.GetCardByUID(c.Request.Context(), session.CardUID)
			if createdCard != nil {
				// Link card to device - IMPORTANT! Without this, card won't have access
				linked, _ := h.db.IsCardLinkedToDevice(c.Request.Context(), createdCard.ID, device.ID)
				if !linked {
					log.Printf("[DESFire STEP] Linking card to device %s...", device.Name)
					if err := h.db.LinkCardToDevice(c.Request.Context(), createdCard.ID, device.ID); err != nil {
						log.Printf("[DESFire STEP] ⚠ Failed to link card to device: %v", err)
					} else {
						log.Printf("[DESFire STEP] ✓ Card linked to device")
					}
				} else {
					log.Printf("[DESFire STEP] Card already linked to device")
				}
				
				// Broadcast new card with device link
				updatedCard, _ := h.db.GetCardByUID(c.Request.Context(), session.CardUID)
				if updatedCard != nil {
					h.hub.BroadcastCardUpdate("created", updatedCard)
					log.Printf("[DESFire STEP] ✓ Card update broadcast to WebSocket clients")
				}
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
			log.Printf("[DESFire STEP] ✓ Access log created: action=provision status=key_written")
			
			h.desfire.DeleteSession(session.ID)
			log.Printf("[DESFire STEP] ════════ PROVISIONING SESSION COMPLETE ════════")
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "provisioned",
				Message: "Card provisioned successfully, awaiting activation",
			})
			return
		}
		
		if status == "error" {
			log.Printf("[DESFire STEP] ❌ PROVISIONING FAILED")
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "error",
				Reason:  "provisioning_error",
				Message: "Provisioning failed",
			})
			return
		}
		
		// Continue provisioning
		log.Printf("[DESFire STEP] → Next provisioning command: %s", cmd)
		c.JSON(http.StatusOK, services.DesfireStepResponse{
			Status:  "continue",
			Command: cmd,
		})
		return
	}

	// Handle authentication states
	log.Printf("[DESFire STEP] AUTH STATE: %s", session.State)
	switch session.State {
	case services.DesfireStateSelectApp:
		log.Printf("[DESFire STEP] [1/3] SELECT APP - Processing response...")
		// Response should be 0x00 (OK) or error
		if len(response) == 0 || response[0] != 0x00 {
			// App might not exist yet
			if len(response) >= 2 && response[0] == 0x91 && response[1] == 0xA0 {
				// Application not found - card not provisioned correctly
				log.Printf("[DESFire STEP] ❌ [1/3] SELECT APP FAILED: App not found on card %s - needs re-provisioning", session.CardUID)
				h.desfire.DeleteSession(session.ID)
				c.JSON(http.StatusOK, services.DesfireStepResponse{
					Status:  "error",
					Reason:  "app_not_found",
					Message: "Card needs provisioning",
				})
				return
			}
			log.Printf("[DESFire STEP] ❌ [1/3] SELECT APP FAILED: Unexpected response 0x%X", response[0])
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "error",
				Reason:  "select_app_failed",
				Message: "Failed to select application",
			})
			return
		}

		// App selected successfully, start authentication
		log.Printf("[DESFire STEP] ✓ [1/3] SELECT APP OK - Starting 3DES authentication...")
		session.State = services.DesfireStateAuth1
		authCmd := h.desfire.BuildAuth1Command(0x00)
		log.Printf("[DESFire STEP] → Sending AUTH1 command: %s", authCmd)
		c.JSON(http.StatusOK, services.DesfireStepResponse{
			Status:  "continue",
			Command: authCmd, // Key 0
		})
		return

	case services.DesfireStateAuth1:
		log.Printf("[DESFire STEP] [2/3] AUTH1 - Processing card RndB...")
		// Response should be 0xAF + EncRndB (16 bytes)
		if len(response) < 17 || response[0] != 0xAF {
			log.Printf("[DESFire STEP] ❌ [2/3] AUTH1 FAILED: Unexpected response: %s", responseHex)
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
		log.Printf("[DESFire STEP] ✓ [2/3] Received EncRndB: %s...", encRndB[:16])
		
		// Process and generate response (decrypt RndB, generate RndA, create response)
		log.Printf("[DESFire STEP] → Decrypting RndB, generating RndA, computing response...")
		cmd, err := h.desfire.ProcessAuth1Response(session, encRndB)
		if err != nil {
			log.Printf("[DESFire STEP] ❌ [2/3] Crypto error: %v", err)
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "error",
				Reason:  "crypto_error",
				Message: err.Error(),
			})
			return
		}

		log.Printf("[DESFire STEP] ✓ [2/3] AUTH1 OK - Sending EncRndA|EncRndB'...")
		session.State = services.DesfireStateAuth2
		c.JSON(http.StatusOK, services.DesfireStepResponse{
			Status:  "continue",
			Command: cmd,
		})
		return

	case services.DesfireStateAuth2:
		log.Printf("[DESFire STEP] [3/3] AUTH2 - Verifying card's RndA response...")
		// Verify the final response
		verified, err := h.desfire.VerifyAuth2Response(session, responseHex)
		if err != nil {
			log.Printf("[DESFire STEP] ❌ [3/3] Verification error: %v", err)
			h.desfire.DeleteSession(session.ID)
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "error",
				Reason:  "verification_error",
				Message: err.Error(),
			})
			return
		}

		if !verified {
			h.desfire.DeleteSession(session.ID)
			// CLONE DETECTED!
			log.Printf("[DESFire STEP] ⚠️⚠️⚠️ CLONE DETECTED for card %s! ⚠️⚠️⚠️", session.CardUID)
			log.Printf("[DESFire STEP] Card's RndA response did not match expected value - possible clone/tampering!")
			
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

		log.Printf("[DESFire STEP] ✓✓✓ [3/3] CRYPTO VERIFICATION PASSED - Card is AUTHENTIC!")

		// Crypto verified - check if we need to update key on card
		if session.PendingKeyUpdate {
			log.Printf("[DESFire STEP] 🔑 Starting key update: v%d -> v%d", session.NewKeyVersion-1, session.NewKeyVersion)
			
			// Calculate new key and build ChangeKey command
			newKey := h.desfire.DeriveKeyForCardVersion(session.CardUID, session.NewKeyVersion)
			session.DerivedKey = newKey // Set new key for change key command
			
			cmd, err := h.desfire.BuildChangeKeyCommandForSession(session)
			if err != nil {
				log.Printf("[DESFire STEP] ❌ Failed to build ChangeKey command: %v", err)
				h.desfire.DeleteSession(session.ID)
				c.JSON(http.StatusOK, services.DesfireStepResponse{
					Status:  "error",
					Reason:  "key_update_error",
					Message: err.Error(),
				})
				return
			}
			
			log.Printf("[DESFire STEP] → Sending ChangeKey command...")
			session.State = services.DesfireStateKeyUpdate
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "continue",
				Command: cmd,
				Message: "Updating key on card...",
			})
			return
		}
		
		h.desfire.DeleteSession(session.ID)
		
		// Crypto verified - now check card status and device linking
		log.Printf("[DESFire STEP] Card %s authenticated - checking database status...", session.CardUID)

		// Get card info
		card, err := h.db.GetCardByUID(c.Request.Context(), session.CardUID)
		if err != nil || card == nil {
			log.Printf("[DESFire] Card %s not found in database", session.CardUID)
			h.db.CreateAccessLog(c.Request.Context(), &models.AccessLog{
				DeviceID: device.Name,
				CardUID:  session.CardUID,
				CardType: "MIFARE_DESFIRE",
				Action:   "desfire_auth",
				Status:   "not_found",
				Allowed:  false,
			})
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:  "denied",
				Reason:  "card_not_found",
				Message: "Card not registered",
			})
			return
		}

		cardName := card.Name
		if cardName == "" {
			cardName = card.CardUID
		}

		// Check card status
		if card.Status != models.CardStatusActive {
			log.Printf("[DESFire] Card %s is not active (status: %s)", session.CardUID, card.Status)
			h.db.CreateAccessLog(c.Request.Context(), &models.AccessLog{
				DeviceID: device.Name,
				CardUID:  session.CardUID,
				CardType: "MIFARE_DESFIRE",
				Action:   "desfire_auth",
				Status:   card.Status,
				Allowed:  false,
			})
			h.hub.BroadcastAccessLog(map[string]interface{}{
				"type":     "access",
				"card_uid": session.CardUID,
				"device":   device.Name,
				"allowed":  false,
				"action":   "desfire_auth",
				"status":   card.Status,
			})
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:   "denied",
				Reason:   "card_" + card.Status,
				Message:  "Card is " + card.Status,
				CardName: cardName,
			})
			return
		}

		// Check if card is linked to this device
		linkedToDevice, _ := h.db.IsCardLinkedToDevice(c.Request.Context(), card.ID, device.ID)
		if !linkedToDevice {
			log.Printf("[DESFire] Card %s not linked to device %s", session.CardUID, device.Name)
			h.db.CreateAccessLog(c.Request.Context(), &models.AccessLog{
				DeviceID: device.Name,
				CardUID:  session.CardUID,
				CardType: "MIFARE_DESFIRE",
				Action:   "desfire_auth",
				Status:   "not_linked",
				Allowed:  false,
			})
			h.hub.BroadcastAccessLog(map[string]interface{}{
				"type":     "access",
				"card_uid": session.CardUID,
				"device":   device.Name,
				"allowed":  false,
				"action":   "desfire_auth",
				"status":   "not_linked",
			})
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:   "denied",
				Reason:   "not_linked",
				Message:  "Card not linked to this device",
				CardName: cardName,
			})
			return
		}

		// SUCCESS! Card authenticated, active, and linked
		log.Printf("[DESFire] Card %s GRANTED access on device %s", session.CardUID, device.Name)

		h.db.CreateAccessLog(c.Request.Context(), &models.AccessLog{
			DeviceID: device.Name,
			CardUID:  session.CardUID,
			CardType: "MIFARE_DESFIRE",
			Action:   "desfire_auth",
			Status:   "authenticated",
			Allowed:  true,
		})

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
			Message:  "Access granted",
		})
		return

	case services.DesfireStateKeyUpdate:
		// Response from ChangeKey command
		if len(response) >= 1 && response[0] == 0x00 {
			// Key updated successfully!
			log.Printf("[DESFire] KEY ROTATION COMPLETE for card %s (now v%d)", session.CardUID, session.NewKeyVersion)
			
			// Clear pending flag in database
			card, _ := h.db.GetCardByUID(c.Request.Context(), session.CardUID)
			if card != nil {
				h.db.ClearPendingKeyUpdate(c.Request.Context(), card.ID)
			}
			
			h.desfire.DeleteSession(session.ID)
			
			// Log the key update
			h.db.CreateAccessLog(c.Request.Context(), &models.AccessLog{
				DeviceID: device.Name,
				CardUID:  session.CardUID,
				CardType: "MIFARE_DESFIRE",
				Action:   "key_rotation",
				Status:   "success",
				Allowed:  true,
			})
			
			cardName := ""
			if card != nil {
				cardName = card.Name
				if cardName == "" {
					cardName = card.CardUID
				}
			}
			
			c.JSON(http.StatusOK, services.DesfireStepResponse{
				Status:   "granted",
				CardName: cardName,
				Message:  "Key updated, access granted",
			})
			return
		}
		
		// Key update failed
		log.Printf("[DESFire] KEY ROTATION FAILED for card %s: %s", session.CardUID, responseHex)
		h.desfire.DeleteSession(session.ID)
		c.JSON(http.StatusOK, services.DesfireStepResponse{
			Status:  "error",
			Reason:  "key_update_failed",
			Message: "Failed to update key on card",
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

	// Get created card and link to device
	createdCard, _ := h.db.GetCardByUID(c.Request.Context(), session.CardUID)
	if createdCard != nil {
		// Link card to device - IMPORTANT! Without this, card won't have access
		linked, _ := h.db.IsCardLinkedToDevice(c.Request.Context(), createdCard.ID, device.ID)
		if !linked {
			log.Printf("[DESFire CONFIRM] Linking card to device %s...", device.Name)
			if err := h.db.LinkCardToDevice(c.Request.Context(), createdCard.ID, device.ID); err != nil {
				log.Printf("[DESFire CONFIRM] ⚠ Failed to link card to device: %v", err)
			} else {
				log.Printf("[DESFire CONFIRM] ✓ Card linked to device")
			}
		}
		
		// Broadcast new card with device link
		updatedCard, _ := h.db.GetCardByUID(c.Request.Context(), session.CardUID)
		if updatedCard != nil {
			h.hub.BroadcastCardUpdate("created", updatedCard)
		}
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

// DesfireCancel handles POST /access/desfire/cancel
// Cancels a DESFire session (when card is removed prematurely)
func (h *DesfireHandler) DesfireCancel(c *gin.Context) {
	// Authenticate device
	device, err := h.getDeviceFromToken(c)
	if device == nil || err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "Invalid or missing device token",
			"error_code": "INVALID_DEVICE_TOKEN",
		})
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "session_id required",
			"error_code": "INVALID_REQUEST",
		})
		return
	}

	// Get and delete session
	session, err := h.desfire.GetSession(req.SessionID)
	if err != nil {
		// Session might already be expired/deleted - that's OK
		log.Printf("[DESFire CANCEL] Session not found (may have expired): %s", req.SessionID)
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "Session not found or already expired",
		})
		return
	}

	// Verify device owns this session
	if session.DeviceID != device.ID {
		log.Printf("[DESFire CANCEL] Session belongs to different device: %s vs %s", session.DeviceID, device.ID)
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "Session belongs to different device",
			"error_code": "SESSION_MISMATCH",
		})
		return
	}

	// Delete the session
	h.desfire.DeleteSession(req.SessionID)
	log.Printf("[DESFire CANCEL] ✓ Session cancelled: %s (card: %s, device: %s)", 
		req.SessionID[:8], session.CardUID, device.Name)

	c.JSON(http.StatusOK, gin.H{
		"status":  "cancelled",
		"message": "Session cancelled successfully",
	})
}

