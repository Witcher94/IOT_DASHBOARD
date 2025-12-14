package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/database"
)

type AdminHandler struct {
	db *database.DB
}

func NewAdminHandler(db *database.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

// GetAllUsers returns all users (admin only)
func (h *AdminHandler) GetAllUsers(c *gin.Context) {
	users, err := h.db.GetAllUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

// DeleteUser deletes a user and all their data (admin only)
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	userIDParam := c.Param("id")
	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Prevent self-deletion
	currentUserID, _ := c.Get("user_id")
	if currentUserID.(uuid.UUID) == userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete yourself"})
		return
	}

	// Check if user exists
	user, err := h.db.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Prevent deleting other admins (optional security measure)
	if user.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete another admin"})
		return
	}

	if err := h.db.DeleteUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

// UpdateUserRole updates user's admin status (admin only)
func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	userIDParam := c.Param("id")
	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Prevent self-demotion
	currentUserID, _ := c.Get("user_id")
	if currentUserID.(uuid.UUID) == userID && !req.IsAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot remove your own admin rights"})
		return
	}

	if err := h.db.SetUserAdmin(c.Request.Context(), userID, req.IsAdmin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated user
	user, _ := h.db.GetUserByID(c.Request.Context(), userID)
	c.JSON(http.StatusOK, user)
}

// GetUserDevices returns all devices for a specific user (admin only)
func (h *AdminHandler) GetUserDevices(c *gin.Context) {
	userIDParam := c.Param("id")
	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	devices, err := h.db.GetDevicesByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, devices)
}



