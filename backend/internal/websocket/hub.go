package websocket

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte
	UserID   uuid.UUID
	DeviceID *uuid.UUID // nil if subscribed to all user devices
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan *Message
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

type Message struct {
	Type     string      `json:"type"`
	DeviceID string      `json:"device_id,omitempty"`
	Data     interface{} `json:"data"`
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan *Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("WebSocket client connected: %s", client.UserID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
			}
			h.mu.Unlock()
			log.Printf("WebSocket client disconnected: %s", client.UserID)

		case message := <-h.broadcast:
			data, err := json.Marshal(message)
			if err != nil {
				log.Printf("Error marshaling message: %v", err)
				continue
			}

			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- data:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

func (h *Hub) BroadcastToUser(userID uuid.UUID, msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.UserID == userID {
			select {
			case client.Send <- data:
			default:
				close(client.Send)
				delete(h.clients, client)
			}
		}
	}
}

func (h *Hub) BroadcastMetrics(userID uuid.UUID, deviceID uuid.UUID, data interface{}) {
	h.BroadcastToUser(userID, &Message{
		Type:     "metrics",
		DeviceID: deviceID.String(),
		Data:     data,
	})
}

func (h *Hub) BroadcastDeviceStatus(userID uuid.UUID, deviceID uuid.UUID, isOnline bool) {
	h.BroadcastToUser(userID, &Message{
		Type:     "device_status",
		DeviceID: deviceID.String(),
		Data: map[string]interface{}{
			"is_online": isOnline,
		},
	})
}

func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

func (c *Client) WritePump() {
	defer func() {
		c.Conn.Close()
	}()

	for {
		message, ok := <-c.Send
		if !ok {
			c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}

		if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}

