package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/database"
	"github.com/pfaka/iot-dashboard/internal/models"
	"github.com/pfaka/iot-dashboard/internal/websocket"
)

type GatewayHandler struct {
	db  *database.DB
	hub *websocket.Hub
}

func NewGatewayHandler(db *database.DB, hub *websocket.Hub) *GatewayHandler {
	return &GatewayHandler{db: db, hub: hub}
}

// ReceiveBatchMetrics handles batch metrics from a gateway
// POST /api/v1/metrics/batch
func (h *GatewayHandler) ReceiveBatchMetrics(c *gin.Context) {
	// Get gateway from context (authenticated by X-Device-Token or X-Gateway-Token)
	device, exists := c.Get("device")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Gateway not authenticated"})
		return
	}
	gateway := device.(*models.Device)

	// Verify it's a gateway
	if gateway.DeviceType != models.DeviceTypeGateway {
		log.Printf("[BATCH] Device %s is not a gateway (type: %s)", gateway.ID, gateway.DeviceType)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Device is not a gateway"})
		return
	}

	var payload models.BatchMetricsPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Printf("[BATCH] Invalid payload: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[BATCH] Received from gateway %s: %d nodes", gateway.Name, len(payload.Nodes))

	// Update gateway's last seen
	now := time.Now()
	gateway.IsOnline = true
	gateway.LastSeen = &now
	if err := h.db.UpdateDeviceOnline(c.Request.Context(), gateway.ID, true); err != nil {
		log.Printf("[BATCH] Failed to update gateway status: %v", err)
	}

	// Process each node's metrics
	processedNodes := 0
	for _, nodeData := range payload.Nodes {
		// Get or create mesh node
		meshNode, err := h.db.GetOrCreateMeshNode(
			c.Request.Context(),
			gateway.ID,
			nodeData.NodeID,
			nodeData.NodeName,
		)
		if err != nil {
			log.Printf("[BATCH] Failed to get/create mesh node %d: %v", nodeData.NodeID, err)
			continue
		}

		// Update node info
		if err := h.db.UpdateMeshNodeMetrics(
			c.Request.Context(),
			meshNode.ID,
			nodeData.ChipID,
			nodeData.MAC,
			nodeData.Platform,
			nodeData.Firmware,
		); err != nil {
			log.Printf("[BATCH] Failed to update mesh node info: %v", err)
		}

		// Create metrics record
		metric := &models.Metric{
			DeviceID:    meshNode.ID,
			Temperature: &nodeData.Temperature,
			Humidity:    &nodeData.Humidity,
			FreeHeap:    &nodeData.FreeHeap,
			RSSI:        &nodeData.RSSI,
		}
		if err := h.db.CreateMetric(c.Request.Context(), metric); err != nil {
			log.Printf("[BATCH] Failed to create metric for node %d: %v", nodeData.NodeID, err)
			continue
		}

		// Broadcast via WebSocket
		wsPayload := models.DeviceMetricsPayload{
			NodeName:    nodeData.NodeName,
			Temperature: &nodeData.Temperature,
			Humidity:    &nodeData.Humidity,
		}
		h.hub.BroadcastMetrics(gateway.UserID, meshNode.ID, wsPayload)
		h.hub.BroadcastDeviceStatus(gateway.UserID, meshNode.ID, true)

		processedNodes++
	}

	log.Printf("[BATCH] Processed %d/%d nodes from gateway %s", processedNodes, len(payload.Nodes), gateway.Name)

	c.JSON(http.StatusOK, gin.H{
		"status":          "ok",
		"processed_nodes": processedNodes,
		"total_nodes":     len(payload.Nodes),
	})
}

// GetGatewayTopology returns the topology of a gateway with all its mesh nodes
// GET /api/v1/gateways/:id/topology
func (h *GatewayHandler) GetGatewayTopology(c *gin.Context) {
	gatewayID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid gateway ID"})
		return
	}

	// Get gateway
	gateway, err := h.db.GetDeviceByID(c.Request.Context(), gatewayID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gateway not found"})
		return
	}

	// Check ownership
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")
	if gateway.UserID != userID.(uuid.UUID) && !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Verify it's a gateway
	if gateway.DeviceType != models.DeviceTypeGateway {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Device is not a gateway"})
		return
	}

	// Get mesh nodes
	meshNodes, err := h.db.GetMeshNodesByGatewayID(c.Request.Context(), gatewayID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Count online nodes
	onlineCount := 0
	for _, node := range meshNodes {
		if node.IsOnline {
			onlineCount++
		}
	}

	topology := models.GatewayTopology{
		Gateway:     *gateway,
		MeshNodes:   meshNodes,
		TotalNodes:  len(meshNodes),
		OnlineNodes: onlineCount,
	}

	c.JSON(http.StatusOK, topology)
}

// SendCommandToMeshNode sends a command through the gateway to a mesh node
// POST /api/v1/gateways/:id/nodes/:nodeId/commands
func (h *GatewayHandler) SendCommandToMeshNode(c *gin.Context) {
	gatewayID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid gateway ID"})
		return
	}

	meshNodeID, err := uuid.Parse(c.Param("nodeId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node ID"})
		return
	}

	// Get gateway
	gateway, err := h.db.GetDeviceByID(c.Request.Context(), gatewayID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gateway not found"})
		return
	}

	// Check ownership
	userID, _ := c.Get("user_id")
	isAdmin, _ := c.Get("is_admin")
	if gateway.UserID != userID.(uuid.UUID) && !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Get mesh node
	meshNode, err := h.db.GetDeviceByID(c.Request.Context(), meshNodeID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Mesh node not found"})
		return
	}

	// Verify node belongs to gateway
	if meshNode.GatewayID == nil || *meshNode.GatewayID != gatewayID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Node does not belong to this gateway"})
		return
	}

	var req models.CreateCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create command for the gateway (it will route to the mesh node)
	// Store the target mesh_node_id in params
	targetNodeID := uint32(0)
	if meshNode.MeshNodeID != nil {
		targetNodeID = *meshNode.MeshNodeID
	}
	cmd := &models.Command{
		DeviceID: gatewayID, // Command goes to gateway
		Command:  req.Command,
		Params:   fmt.Sprintf(`{"target_mesh_node_id": %d}`, targetNodeID),
		Status:   "pending",
	}

	if err := h.db.CreateCommand(c.Request.Context(), cmd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[CMD] Created command '%s' for mesh node %d via gateway %s", 
		req.Command, *meshNode.MeshNodeID, gateway.Name)

	c.JSON(http.StatusCreated, cmd)
}

// GetPendingCommands returns pending commands for a gateway to poll
// GET /api/v1/commands/pending (called by gateway with X-Device-Token)
func (h *GatewayHandler) GetPendingCommands(c *gin.Context) {
	device, exists := c.Get("device")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}
	gateway := device.(*models.Device)

	log.Printf("[CMD] Gateway %s polling for commands", gateway.Name)

	// Get pending command
	cmd, err := h.db.GetPendingCommand(c.Request.Context(), gateway.ID)
	if err != nil {
		// No pending commands
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	// Mark as sent
	if err := h.db.MarkCommandSent(c.Request.Context(), cmd.ID); err != nil {
		log.Printf("[CMD] Failed to mark command as sent: %v", err)
	}

	log.Printf("[CMD] Returning command %s (%s) to gateway %s", cmd.ID, cmd.Command, gateway.Name)

	c.JSON(http.StatusOK, cmd)
}

