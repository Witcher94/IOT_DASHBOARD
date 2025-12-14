package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DESFire AES Authentication States
const (
	DesfireStateStarted    = "started"
	DesfireStateSelectApp  = "select_app"
	DesfireStateAuth1      = "auth1"
	DesfireStateAuth2      = "auth2"
	DesfireStateComplete   = "complete"
	DesfireStateFailed     = "failed"
	
	// Provisioning states
	DesfireStateProvSelectPicc    = "prov_select_picc"
	DesfireStateProvAuthPicc1     = "prov_auth_picc_1"
	DesfireStateProvAuthPicc2     = "prov_auth_picc_2"
	DesfireStateProvCreateApp     = "prov_create_app"
	DesfireStateProvSelectApp     = "prov_select_app"
	DesfireStateProvAuthApp1      = "prov_auth_app_1"
	DesfireStateProvAuthApp2      = "prov_auth_app_2"
	DesfireStateProvChangeKey     = "prov_change_key"
	DesfireStateProvComplete      = "prov_complete"
)

// DESFire command types
const (
	DesfireCmdSelectApp   = "SELECT_APP"
	DesfireCmdSelectPicc  = "SELECT_PICC"
	DesfireCmdAuth1       = "AUTH_PART1"
	DesfireCmdAuth2       = "AUTH_PART2"
	DesfireCmdCreateApp   = "CREATE_APP"
	DesfireCmdChangeKey   = "CHANGE_KEY"
)

// Default DESFire key (all zeros) for new cards
var defaultDesfireKey = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

// DesfireSession represents an active authentication session
type DesfireSession struct {
	ID           string    `json:"id"`
	CardUID      string    `json:"card_uid"`
	DeviceID     uuid.UUID `json:"device_id"`
	State        string    `json:"state"`
	
	// Crypto state (not exposed in JSON)
	DerivedKey   []byte    `json:"-"`
	CurrentKey   []byte    `json:"-"`  // Key being used for current auth (default or derived)
	RndA         []byte    `json:"-"`  // Our random
	RndB         []byte    `json:"-"`  // Card's random
	IV           []byte    `json:"-"`  // Current IV for CBC
	SessionKey   []byte    `json:"-"`  // Session key after auth
	
	// Provisioning mode
	IsProvisioning bool     `json:"is_provisioning"`
	ProvStep       int      `json:"-"`  // Current provisioning step
	
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// DesfireCommand represents a command to send to the card
type DesfireCommand struct {
	Type string `json:"type"`
	Data string `json:"data"` // Hex encoded
}

// DesfireStartResponse response for /desfire/start
type DesfireStartResponse struct {
	SessionID     string          `json:"session_id"`
	Status        string          `json:"status"` // auth_required, provision, denied
	Reason        string          `json:"reason,omitempty"`
	Command       *DesfireCommand `json:"command,omitempty"`
	DerivedKeyHex string          `json:"derived_key,omitempty"` // Only for provisioning
	TimeoutMs     int             `json:"timeout_ms"`
}

// DesfireStepResponse response for /desfire/step
type DesfireStepResponse struct {
	Status   string          `json:"status"` // continue, granted, denied, error
	Command  *DesfireCommand `json:"command,omitempty"`
	Reason   string          `json:"reason,omitempty"`
	CardName string          `json:"card_name,omitempty"`
	Message  string          `json:"message,omitempty"`
}

// DesfireService handles DESFire cryptographic operations
type DesfireService struct {
	masterKey    []byte
	appID        []byte // Our application ID
	sessions     map[string]*DesfireSession
	sessionMutex sync.RWMutex
}

// NewDesfireService creates a new DESFire service
func NewDesfireService(masterKeyHex string) (*DesfireService, error) {
	// Parse master key (should be 32 hex chars = 16 bytes for AES-128)
	masterKey, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid master key hex: %w", err)
	}
	if len(masterKey) != 16 {
		return nil, fmt.Errorf("master key must be 16 bytes (32 hex chars), got %d", len(masterKey))
	}

	service := &DesfireService{
		masterKey: masterKey,
		appID:     []byte{0x01, 0x00, 0x00}, // AID = 0x000001
		sessions:  make(map[string]*DesfireSession),
	}

	// Start cleanup goroutine
	go service.cleanupExpiredSessions()

	log.Printf("[DESFire] Service initialized with master key")
	return service, nil
}

// DeriveKeyForCard derives a unique AES-128 key for a specific card UID
// Uses HMAC-SHA256 then truncates to 16 bytes
func (s *DesfireService) DeriveKeyForCard(cardUID string) []byte {
	// Key = HMAC-SHA256(MasterKey, "SKUD_DESFIRE_V1" + CardUID)[:16]
	h := hmac.New(sha256.New, s.masterKey)
	h.Write([]byte("SKUD_DESFIRE_V1"))
	h.Write([]byte(cardUID))
	fullHash := h.Sum(nil)
	
	// Take first 16 bytes for AES-128
	derivedKey := make([]byte, 16)
	copy(derivedKey, fullHash[:16])
	
	return derivedKey
}

// CreateSession creates a new authentication session
func (s *DesfireService) CreateSession(cardUID string, deviceID uuid.UUID, isProvisioning bool) *DesfireSession {
	sessionID := uuid.New().String()
	
	session := &DesfireSession{
		ID:             sessionID,
		CardUID:        cardUID,
		DeviceID:       deviceID,
		State:          DesfireStateStarted,
		DerivedKey:     s.DeriveKeyForCard(cardUID),
		IsProvisioning: isProvisioning,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(30 * time.Second), // 30 second timeout
	}
	
	s.sessionMutex.Lock()
	s.sessions[sessionID] = session
	s.sessionMutex.Unlock()
	
	log.Printf("[DESFire] Session created: %s for card %s (provisioning: %v)", sessionID, cardUID, isProvisioning)
	
	return session
}

// GetSession retrieves a session by ID
func (s *DesfireService) GetSession(sessionID string) (*DesfireSession, error) {
	s.sessionMutex.RLock()
	session, exists := s.sessions[sessionID]
	s.sessionMutex.RUnlock()
	
	if !exists {
		return nil, errors.New("session not found")
	}
	
	if time.Now().After(session.ExpiresAt) {
		s.DeleteSession(sessionID)
		return nil, errors.New("session expired")
	}
	
	return session, nil
}

// DeleteSession removes a session
func (s *DesfireService) DeleteSession(sessionID string) {
	s.sessionMutex.Lock()
	delete(s.sessions, sessionID)
	s.sessionMutex.Unlock()
}

// GetAppID returns the application ID as hex
func (s *DesfireService) GetAppID() string {
	return hex.EncodeToString(s.appID)
}

// BuildSelectAppCommand builds the SELECT_APPLICATION command
func (s *DesfireService) BuildSelectAppCommand() *DesfireCommand {
	// SelectApplication: 0x5A + AID (3 bytes, LSB first)
	cmd := append([]byte{0x5A}, s.appID...)
	return &DesfireCommand{
		Type: DesfireCmdSelectApp,
		Data: hex.EncodeToString(cmd),
	}
}

// BuildAuth1Command builds the first authentication command
func (s *DesfireService) BuildAuth1Command(keyNo byte) *DesfireCommand {
	// AuthenticateAES: 0xAA + KeyNo
	cmd := []byte{0xAA, keyNo}
	return &DesfireCommand{
		Type: DesfireCmdAuth1,
		Data: hex.EncodeToString(cmd),
	}
}

// ProcessAuth1Response processes the EncRndB from the card and generates the response
func (s *DesfireService) ProcessAuth1Response(session *DesfireSession, encRndBHex string) (*DesfireCommand, error) {
	encRndB, err := hex.DecodeString(encRndBHex)
	if err != nil {
		return nil, fmt.Errorf("invalid encRndB hex: %w", err)
	}
	
	if len(encRndB) != 16 {
		return nil, fmt.Errorf("encRndB must be 16 bytes, got %d", len(encRndB))
	}
	
	// Decrypt RndB using the derived key (IV = all zeros for first block)
	block, err := aes.NewCipher(session.DerivedKey)
	if err != nil {
		return nil, err
	}
	
	// For DESFire AES, first decryption uses IV = 0
	iv := make([]byte, 16)
	mode := cipher.NewCBCDecrypter(block, iv)
	
	rndB := make([]byte, 16)
	mode.CryptBlocks(rndB, encRndB)
	session.RndB = rndB
	
	// Generate RndA (our random)
	rndA := make([]byte, 16)
	if _, err := rand.Read(rndA); err != nil {
		return nil, err
	}
	session.RndA = rndA
	
	// Rotate RndB left by 1 byte
	rndBrot := make([]byte, 16)
	copy(rndBrot, rndB[1:])
	rndBrot[15] = rndB[0]
	
	// Concatenate RndA + RndB'
	plaintext := make([]byte, 32)
	copy(plaintext[:16], rndA)
	copy(plaintext[16:], rndBrot)
	
	// Encrypt with CBC, IV = encRndB
	mode2 := cipher.NewCBCEncrypter(block, encRndB)
	ciphertext := make([]byte, 32)
	mode2.CryptBlocks(ciphertext, plaintext)
	
	// Store last block as IV for next operation
	session.IV = ciphertext[16:32]
	
	// Response: 0xAF + encrypted data
	response := append([]byte{0xAF}, ciphertext...)
	
	return &DesfireCommand{
		Type: DesfireCmdAuth2,
		Data: hex.EncodeToString(response),
	}, nil
}

// VerifyAuth2Response verifies the final authentication response from the card
func (s *DesfireService) VerifyAuth2Response(session *DesfireSession, responseHex string) (bool, error) {
	response, err := hex.DecodeString(responseHex)
	if err != nil {
		return false, fmt.Errorf("invalid response hex: %w", err)
	}
	
	// Check status byte
	if len(response) < 1 {
		return false, errors.New("empty response")
	}
	
	status := response[0]
	if status == 0x91 && len(response) > 1 && response[1] == 0xAE {
		// Authentication error - likely a clone!
		log.Printf("[DESFire] Authentication FAILED for card %s - POSSIBLE CLONE!", session.CardUID)
		return false, nil
	}
	
	if status != 0x00 {
		return false, fmt.Errorf("card returned error status: 0x%02X", status)
	}
	
	if len(response) < 17 {
		return false, fmt.Errorf("response too short for encRndA: %d bytes", len(response))
	}
	
	encRndA := response[1:17]
	
	// Decrypt EncRndA
	block, err := aes.NewCipher(session.DerivedKey)
	if err != nil {
		return false, err
	}
	
	mode := cipher.NewCBCDecrypter(block, session.IV)
	rndAResponse := make([]byte, 16)
	mode.CryptBlocks(rndAResponse, encRndA)
	
	// Expected: RndA rotated left by 1 byte
	expectedRndA := make([]byte, 16)
	copy(expectedRndA, session.RndA[1:])
	expectedRndA[15] = session.RndA[0]
	
	// Compare
	if !hmac.Equal(rndAResponse, expectedRndA) {
		log.Printf("[DESFire] RndA verification FAILED for card %s - POSSIBLE CLONE!", session.CardUID)
		return false, nil
	}
	
	// Success! Card is authenticated
	log.Printf("[DESFire] Card %s successfully authenticated!", session.CardUID)
	
	// Generate session key (optional, for encrypted communication)
	// SessionKey = RndA[0:4] + RndB[0:4] + RndA[12:16] + RndB[12:16]
	sessionKey := make([]byte, 16)
	copy(sessionKey[0:4], session.RndA[0:4])
	copy(sessionKey[4:8], session.RndB[0:4])
	copy(sessionKey[8:12], session.RndA[12:16])
	copy(sessionKey[12:16], session.RndB[12:16])
	session.SessionKey = sessionKey
	
	return true, nil
}

// BuildProvisioningCommands builds commands for provisioning a new card
func (s *DesfireService) BuildProvisioningCommands(session *DesfireSession) []DesfireCommand {
	commands := []DesfireCommand{}
	
	// 1. Select PICC (root application)
	commands = append(commands, DesfireCommand{
		Type: "SELECT_PICC",
		Data: hex.EncodeToString([]byte{0x5A, 0x00, 0x00, 0x00}),
	})
	
	// 2. Auth with default key (will be handled in steps)
	
	// 3. Create application
	// CreateApp: 0xCA + AID(3) + KeySettings(1) + NumKeys(1)
	// KeySettings: 0x0F = master key changeable, config changeable
	// NumKeys: 0x81 = 1 key, AES
	createApp := []byte{0xCA}
	createApp = append(createApp, s.appID...)
	createApp = append(createApp, 0x0F, 0x81)
	commands = append(commands, DesfireCommand{
		Type: "CREATE_APP",
		Data: hex.EncodeToString(createApp),
	})
	
	// 4. Select our application
	commands = append(commands, *s.BuildSelectAppCommand())
	
	// 5. Auth with default app key (will be handled)
	// 6. Change key (will be handled)
	
	return commands
}

// GetDerivedKeyHex returns the derived key as hex string (for provisioning)
func (s *DesfireService) GetDerivedKeyHex(cardUID string) string {
	key := s.DeriveKeyForCard(cardUID)
	return hex.EncodeToString(key)
}

// cleanupExpiredSessions periodically removes expired sessions
func (s *DesfireService) cleanupExpiredSessions() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		s.sessionMutex.Lock()
		now := time.Now()
		for id, session := range s.sessions {
			if now.After(session.ExpiresAt) {
				delete(s.sessions, id)
				log.Printf("[DESFire] Session expired and cleaned up: %s", id)
			}
		}
		s.sessionMutex.Unlock()
	}
}

// ==================== Provisioning Methods ====================

// StartProvisioning begins the provisioning flow and returns the first command
func (s *DesfireService) StartProvisioning(session *DesfireSession) *DesfireCommand {
	session.State = DesfireStateProvSelectPicc
	session.CurrentKey = defaultDesfireKey // Start with default key
	session.ProvStep = 0
	
	// First command: Select PICC (root application)
	return &DesfireCommand{
		Type: DesfireCmdSelectPicc,
		Data: "5A000000", // Select AID 0x000000 (PICC)
	}
}

// ProcessProvisioningStep processes a provisioning step and returns the next command
func (s *DesfireService) ProcessProvisioningStep(session *DesfireSession, responseHex string) (*DesfireCommand, string, error) {
	response, err := hex.DecodeString(responseHex)
	if err != nil {
		return nil, "error", fmt.Errorf("invalid hex: %w", err)
	}
	
	log.Printf("[DESFire Prov] State: %s, Response: %s", session.State, responseHex)
	
	switch session.State {
	case DesfireStateProvSelectPicc:
		// Check if PICC selected OK
		if len(response) == 0 || response[0] != 0x00 {
			return nil, "error", fmt.Errorf("select PICC failed: %s", responseHex)
		}
		// Start PICC authentication with default key
		session.State = DesfireStateProvAuthPicc1
		session.CurrentKey = defaultDesfireKey
		return &DesfireCommand{
			Type: DesfireCmdAuth1,
			Data: "AA00", // Authenticate AES, key 0
		}, "continue", nil
		
	case DesfireStateProvAuthPicc1:
		// Response should be 0xAF + EncRndB (16 bytes)
		if len(response) < 17 || response[0] != 0xAF {
			// Some cards don't require PICC-level auth - try creating app directly
			log.Printf("[DESFire Prov] PICC auth not required, creating app directly")
			session.State = DesfireStateProvCreateApp
			return s.buildCreateAppCommand(), "continue", nil
		}
		
		// Process Auth1 response
		encRndB := hex.EncodeToString(response[1:17])
		cmd, err := s.processProvAuth1(session, encRndB)
		if err != nil {
			return nil, "error", err
		}
		session.State = DesfireStateProvAuthPicc2
		return cmd, "continue", nil
		
	case DesfireStateProvAuthPicc2:
		// Verify Auth2 response
		verified, err := s.verifyProvAuth2(session, responseHex)
		if err != nil {
			return nil, "error", err
		}
		if !verified {
			return nil, "error", errors.New("PICC authentication failed")
		}
		// PICC authenticated, create application
		session.State = DesfireStateProvCreateApp
		return s.buildCreateAppCommand(), "continue", nil
		
	case DesfireStateProvCreateApp:
		// Check result - OK (0x00) or already exists (0x91DE)
		if len(response) >= 1 && response[0] == 0x00 {
			log.Printf("[DESFire Prov] Application created successfully")
		} else if len(response) >= 2 && response[0] == 0x91 && response[1] == 0xDE {
			log.Printf("[DESFire Prov] Application already exists")
		} else {
			return nil, "error", fmt.Errorf("create app failed: %s", responseHex)
		}
		// Select our application
		session.State = DesfireStateProvSelectApp
		return s.BuildSelectAppCommand(), "continue", nil
		
	case DesfireStateProvSelectApp:
		if len(response) == 0 || response[0] != 0x00 {
			return nil, "error", fmt.Errorf("select app failed: %s", responseHex)
		}
		// Start app authentication with default key
		session.State = DesfireStateProvAuthApp1
		session.CurrentKey = defaultDesfireKey
		// Reset crypto state for new auth
		session.RndA = nil
		session.RndB = nil
		session.IV = nil
		session.SessionKey = nil
		return &DesfireCommand{
			Type: DesfireCmdAuth1,
			Data: "AA00", // Authenticate AES, key 0
		}, "continue", nil
		
	case DesfireStateProvAuthApp1:
		if len(response) < 17 || response[0] != 0xAF {
			return nil, "error", fmt.Errorf("app auth1 failed: %s", responseHex)
		}
		encRndB := hex.EncodeToString(response[1:17])
		cmd, err := s.processProvAuth1(session, encRndB)
		if err != nil {
			return nil, "error", err
		}
		session.State = DesfireStateProvAuthApp2
		return cmd, "continue", nil
		
	case DesfireStateProvAuthApp2:
		verified, err := s.verifyProvAuth2(session, responseHex)
		if err != nil {
			return nil, "error", err
		}
		if !verified {
			return nil, "error", errors.New("app authentication failed")
		}
		// App authenticated with session key - now change the key!
		session.State = DesfireStateProvChangeKey
		cmd, err := s.buildChangeKeyCommand(session)
		if err != nil {
			return nil, "error", err
		}
		return cmd, "continue", nil
		
	case DesfireStateProvChangeKey:
		if len(response) >= 1 && response[0] == 0x00 {
			log.Printf("[DESFire Prov] KEY WRITTEN SUCCESSFULLY for card %s!", session.CardUID)
			session.State = DesfireStateProvComplete
			return nil, "provisioned", nil
		}
		return nil, "error", fmt.Errorf("change key failed: %s", responseHex)
		
	default:
		return nil, "error", fmt.Errorf("unknown provisioning state: %s", session.State)
	}
}

// processProvAuth1 processes Auth1 response during provisioning (using current key)
func (s *DesfireService) processProvAuth1(session *DesfireSession, encRndBHex string) (*DesfireCommand, error) {
	encRndB, err := hex.DecodeString(encRndBHex)
	if err != nil {
		return nil, err
	}
	if len(encRndB) != 16 {
		return nil, fmt.Errorf("encRndB must be 16 bytes")
	}
	
	// Decrypt with current key (default key during provisioning)
	block, err := aes.NewCipher(session.CurrentKey)
	if err != nil {
		return nil, err
	}
	
	iv := make([]byte, 16)
	mode := cipher.NewCBCDecrypter(block, iv)
	rndB := make([]byte, 16)
	mode.CryptBlocks(rndB, encRndB)
	session.RndB = rndB
	
	// Generate RndA
	rndA := make([]byte, 16)
	if _, err := rand.Read(rndA); err != nil {
		return nil, err
	}
	session.RndA = rndA
	
	// Rotate RndB
	rndBrot := make([]byte, 16)
	copy(rndBrot, rndB[1:])
	rndBrot[15] = rndB[0]
	
	// Concatenate and encrypt
	plaintext := make([]byte, 32)
	copy(plaintext[:16], rndA)
	copy(plaintext[16:], rndBrot)
	
	mode2 := cipher.NewCBCEncrypter(block, encRndB)
	ciphertext := make([]byte, 32)
	mode2.CryptBlocks(ciphertext, plaintext)
	
	session.IV = ciphertext[16:32]
	
	response := append([]byte{0xAF}, ciphertext...)
	return &DesfireCommand{
		Type: DesfireCmdAuth2,
		Data: hex.EncodeToString(response),
	}, nil
}

// verifyProvAuth2 verifies Auth2 response during provisioning
func (s *DesfireService) verifyProvAuth2(session *DesfireSession, responseHex string) (bool, error) {
	response, err := hex.DecodeString(responseHex)
	if err != nil {
		return false, err
	}
	
	if len(response) < 1 {
		return false, errors.New("empty response")
	}
	
	if response[0] != 0x00 {
		log.Printf("[DESFire Prov] Auth2 failed with status: 0x%02X", response[0])
		return false, nil
	}
	
	if len(response) < 17 {
		return false, fmt.Errorf("response too short")
	}
	
	encRndA := response[1:17]
	
	block, err := aes.NewCipher(session.CurrentKey)
	if err != nil {
		return false, err
	}
	
	mode := cipher.NewCBCDecrypter(block, session.IV)
	rndAResponse := make([]byte, 16)
	mode.CryptBlocks(rndAResponse, encRndA)
	
	// Expected: RndA rotated left
	expectedRndA := make([]byte, 16)
	copy(expectedRndA, session.RndA[1:])
	expectedRndA[15] = session.RndA[0]
	
	if !hmac.Equal(rndAResponse, expectedRndA) {
		return false, nil
	}
	
	// Generate session key
	sessionKey := make([]byte, 16)
	copy(sessionKey[0:4], session.RndA[0:4])
	copy(sessionKey[4:8], session.RndB[0:4])
	copy(sessionKey[8:12], session.RndA[12:16])
	copy(sessionKey[12:16], session.RndB[12:16])
	session.SessionKey = sessionKey
	
	log.Printf("[DESFire Prov] Authentication successful, session key established")
	return true, nil
}

// buildCreateAppCommand builds the CreateApplication command
func (s *DesfireService) buildCreateAppCommand() *DesfireCommand {
	// CreateApp: 0xCA + AID(3, LSB first) + KeySettings(1) + NumKeys(1)
	// KeySettings: 0x0F = all changes with master key
	// NumKeys: 0x81 = 1 AES key
	cmd := []byte{0xCA}
	cmd = append(cmd, s.appID...)
	cmd = append(cmd, 0x0F, 0x81)
	return &DesfireCommand{
		Type: DesfireCmdCreateApp,
		Data: hex.EncodeToString(cmd),
	}
}

// buildChangeKeyCommand builds the ChangeKey command to write the derived key
func (s *DesfireService) buildChangeKeyCommand(session *DesfireSession) (*DesfireCommand, error) {
	// ChangeKey command for AES:
	// 0xC4 + KeyNo + Encrypted(NewKey XOR OldKey + CRC32 + padding)
	// When changing our own key: NewKey + CRC32(NewKey) + padding
	
	if session.SessionKey == nil {
		return nil, errors.New("no session key")
	}
	
	newKey := session.DerivedKey
	if len(newKey) != 16 {
		return nil, errors.New("invalid derived key length")
	}
	
	// Since we're changing key 0 while authenticated as key 0,
	// the data is: NewKey(16) + CRC32(0xC4 || KeyNo || NewKey) + Padding
	
	// Calculate CRC32 (DESFire uses ISO 3309 CRC32)
	keyNo := byte(0x00)
	crcData := append([]byte{0xC4, keyNo}, newKey...)
	crc := calculateDesfireCRC32(crcData)
	
	// Plaintext: NewKey(16) + CRC(4) + Padding(12) = 32 bytes
	plaintext := make([]byte, 32)
	copy(plaintext[0:16], newKey)
	plaintext[16] = byte(crc & 0xFF)
	plaintext[17] = byte((crc >> 8) & 0xFF)
	plaintext[18] = byte((crc >> 16) & 0xFF)
	plaintext[19] = byte((crc >> 24) & 0xFF)
	// Bytes 20-31 are padding (zeros)
	
	// Encrypt with session key, IV = 0
	block, err := aes.NewCipher(session.SessionKey)
	if err != nil {
		return nil, err
	}
	
	iv := make([]byte, 16)
	mode := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, 32)
	mode.CryptBlocks(encrypted, plaintext)
	
	// Command: 0xC4 + KeyNo + Encrypted
	cmd := []byte{0xC4, keyNo}
	cmd = append(cmd, encrypted...)
	
	log.Printf("[DESFire Prov] ChangeKey command built, writing new key...")
	
	return &DesfireCommand{
		Type: DesfireCmdChangeKey,
		Data: hex.EncodeToString(cmd),
	}, nil
}

// calculateDesfireCRC32 calculates CRC32 as used by DESFire (ISO 3309)
func calculateDesfireCRC32(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		crc ^= uint32(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xEDB88320
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

