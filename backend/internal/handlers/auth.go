package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/config"
	"github.com/pfaka/iot-dashboard/internal/database"
	"github.com/pfaka/iot-dashboard/internal/services"
)

type AuthHandler struct {
	cfg         *config.Config
	db          *database.DB
	authService *services.AuthService
}

func NewAuthHandler(cfg *config.Config, db *database.DB, authService *services.AuthService) *AuthHandler {
	return &AuthHandler{
		cfg:         cfg,
		db:          db,
		authService: authService,
	}
}

func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	b := make([]byte, 16)
	rand.Read(b)
	state := base64.URLEncoding.EncodeToString(b)

	c.SetCookie("oauth_state", state, 600, "/", "", false, true)

	url := h.authService.GetGoogleAuthURL(state)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	// Verify state
	state := c.Query("state")
	savedState, err := c.Cookie("oauth_state")
	if err != nil || state != savedState {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state"})
		return
	}

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Code not found"})
		return
	}

	user, token, err := h.authService.HandleGoogleCallback(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Redirect to frontend with token
	redirectURL := h.cfg.FrontendURL + "/auth/callback?token=" + token
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)

	_ = user // Used for logging if needed
}

func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID, _ := c.Get("user_id")

	user, err := h.db.GetUserByID(c.Request.Context(), userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newToken, err := h.authService.RefreshToken(c.Request.Context(), req.Token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": newToken})
}

