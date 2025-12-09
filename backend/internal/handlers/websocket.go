package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	ws "github.com/pfaka/iot-dashboard/internal/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Ticket store for one-time WebSocket auth tickets
type TicketStore struct {
	mu      sync.RWMutex
	tickets map[string]ticketData
}

type ticketData struct {
	UserID    uuid.UUID
	ExpiresAt time.Time
}

var ticketStore = &TicketStore{
	tickets: make(map[string]ticketData),
}

// Generate a secure random ticket
func generateTicket() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Store a ticket (expires in 30 seconds)
func (ts *TicketStore) Create(userID uuid.UUID) string {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ticket := generateTicket()
	ts.tickets[ticket] = ticketData{
		UserID:    userID,
		ExpiresAt: time.Now().Add(30 * time.Second),
	}
	return ticket
}

// Validate and consume a ticket (one-time use)
func (ts *TicketStore) Consume(ticket string) (uuid.UUID, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	data, exists := ts.tickets[ticket]
	if !exists {
		return uuid.Nil, false
	}

	// Delete ticket (one-time use)
	delete(ts.tickets, ticket)

	// Check expiration
	if time.Now().After(data.ExpiresAt) {
		return uuid.Nil, false
	}

	return data.UserID, true
}

// Cleanup expired tickets (call periodically)
func (ts *TicketStore) Cleanup() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	now := time.Now()
	for ticket, data := range ts.tickets {
		if now.After(data.ExpiresAt) {
			delete(ts.tickets, ticket)
		}
	}
}

// Start cleanup goroutine
func init() {
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			ticketStore.Cleanup()
		}
	}()
}

type WebSocketHandler struct {
	hub *ws.Hub
}

func NewWebSocketHandler(hub *ws.Hub) *WebSocketHandler {
	return &WebSocketHandler{hub: hub}
}

// CreateTicket - creates a one-time ticket for WebSocket connection
// Requires JWT auth (called from protected route)
func (h *WebSocketHandler) CreateTicket(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	ticket := ticketStore.Create(userID.(uuid.UUID))
	c.JSON(http.StatusOK, gin.H{"ticket": ticket})
}

// HandleWebSocket - upgrades connection using one-time ticket
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	ticket := c.Query("ticket")
	if ticket == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Ticket required"})
		return
	}

	userID, valid := ticketStore.Consume(ticket)
	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired ticket"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &ws.Client{
		Hub:    h.hub,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		UserID: userID,
	}

	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
