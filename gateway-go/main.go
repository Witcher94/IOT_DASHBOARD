package main

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"go.bug.st/serial"
)

//go:embed templates/*
var templatesFS embed.FS

// Config holds gateway configuration
type Config struct {
	SerialPort    string
	BaudRate      int
	BackendURL    string
	GatewayToken  string
	WebPort       int
	BatchInterval time.Duration
	Debug         bool
}

// MeshNode represents a node in the mesh network
type MeshNode struct {
	NodeID      uint32    `json:"node_id"`
	NodeName    string    `json:"node_name"`
	ChipID      string    `json:"chip_id"`
	MAC         string    `json:"mac"`
	Platform    string    `json:"platform"`
	Firmware    string    `json:"firmware"`
	Temperature float64   `json:"temperature"`
	Humidity    float64   `json:"humidity"`
	FreeHeap    int64     `json:"free_heap"`
	RSSI        int       `json:"rssi"`
	DHTEnabled  bool      `json:"dht_enabled"`
	IsOnline    bool      `json:"is_online"`
	LastSeen    time.Time `json:"last_seen"`
	DeviceToken string    `json:"device_token,omitempty"` // Token for backend
	BackendID   string    `json:"backend_id,omitempty"`   // UUID in backend
}

// Gateway handles mesh-to-cloud communication
type Gateway struct {
	config     Config
	port       serial.Port
	portMu     sync.Mutex
	httpClient *http.Client
	nodes      map[uint32]*MeshNode
	nodesMu    sync.RWMutex
	running    bool
	stats      Stats
	templates  *template.Template
}

// Stats tracks gateway statistics
type Stats struct {
	MessagesReceived int64
	BatchesSent      int64
	CommandsSent     int64
	Errors           int64
	StartTime        time.Time
	mu               sync.Mutex
}

// MeshMessage represents a message from the mesh bridge
type MeshMessage struct {
	Type       string          `json:"type"`
	From       uint32          `json:"from,omitempty"`
	NodeID     uint32          `json:"node_id,omitempty"`
	Timestamp  uint64          `json:"timestamp,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	TotalNodes int             `json:"total_nodes,omitempty"`
	Msg        string          `json:"msg,omitempty"`
	Firmware   string          `json:"firmware,omitempty"`
}

// MetricsData represents sensor data from a mesh node
type MetricsData struct {
	MsgType     string  `json:"msg_type"`
	NodeName    string  `json:"node_name"`
	NodeID      uint32  `json:"node_id"`
	Temperature float64 `json:"temperature,omitempty"`
	Humidity    float64 `json:"humidity,omitempty"`
	DHTEnabled  bool    `json:"dht_enabled"`
	System      struct {
		ChipID   string `json:"chip_id"`
		MAC      string `json:"mac"`
		Firmware string `json:"firmware"`
		Platform string `json:"platform"`
		FreeHeap int64  `json:"free_heap"`
		UptimeMs int64  `json:"uptime_ms"`
	} `json:"system"`
	Mesh struct {
		NodeCount int `json:"node_count"`
		RSSI      int `json:"rssi"`
	} `json:"mesh"`
}

// BatchMetricsPayload is sent to backend
type BatchMetricsPayload struct {
	GatewayID string       `json:"gateway_id"`
	Timestamp time.Time    `json:"timestamp"`
	Nodes     []NodeMetric `json:"nodes"`
}

// NodeMetric is a single node's metrics in batch
type NodeMetric struct {
	NodeID      uint32  `json:"node_id"`
	NodeName    string  `json:"node_name"`
	ChipID      string  `json:"chip_id"`
	MAC         string  `json:"mac"`
	Platform    string  `json:"platform"`
	Firmware    string  `json:"firmware"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	FreeHeap    int64   `json:"free_heap"`
	RSSI        int     `json:"rssi"`
	DHTEnabled  bool    `json:"dht_enabled"`
}

func main() {
	// Parse flags
	serialPort := flag.String("port", "/dev/ttyUSB0", "Serial port for ESP32 bridge")
	baudRate := flag.Int("baud", 115200, "Baud rate")
	backendURL := flag.String("backend", "https://chnu-iot.com", "Backend URL")
	gatewayToken := flag.String("token", "", "Gateway authentication token")
	webPort := flag.Int("web", 8080, "Local web UI port")
	batchInterval := flag.Int("interval", 30, "Batch send interval in seconds")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	config := Config{
		SerialPort:    *serialPort,
		BaudRate:      *baudRate,
		BackendURL:    *backendURL,
		GatewayToken:  *gatewayToken,
		WebPort:       *webPort,
		BatchInterval: time.Duration(*batchInterval) * time.Second,
		Debug:         *debug,
	}

	gateway := NewGateway(config)

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		gateway.Stop()
		os.Exit(0)
	}()

	// Run gateway
	if err := gateway.Run(); err != nil {
		log.Fatalf("Gateway error: %v", err)
	}
}

// NewGateway creates a new gateway instance
func NewGateway(config Config) *Gateway {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		log.Printf("Warning: Failed to parse templates: %v", err)
	}

	return &Gateway{
		config:     config,
		nodes:      make(map[uint32]*MeshNode),
		httpClient: &http.Client{Timeout: 15 * time.Second},
		stats:      Stats{StartTime: time.Now()},
		templates:  tmpl,
	}
}

// Run starts the gateway
func (g *Gateway) Run() error {
	log.Println("==================================================")
	log.Println("  Mesh Gateway v2.0 - Centralized Hub")
	log.Println("==================================================")
	log.Printf("Serial: %s @ %d", g.config.SerialPort, g.config.BaudRate)
	log.Printf("Backend: %s", g.config.BackendURL)
	log.Printf("Web UI: http://localhost:%d", g.config.WebPort)
	log.Printf("Batch interval: %s", g.config.BatchInterval)
	log.Println("==================================================")

	// Connect to serial
	if err := g.connectSerial(); err != nil {
		log.Printf("Warning: Serial connection failed: %v", err)
		log.Println("Running in web-only mode...")
	}

	g.running = true

	// Start batch sender
	go g.batchSender()

	// Start node timeout checker
	go g.nodeTimeoutChecker()

	// Start web server (blocking)
	go g.startWebServer()

	// Start serial reader
	if g.port != nil {
		g.readSerial()
	} else {
		// Keep running for web server
		select {}
	}

	return nil
}

// Stop stops the gateway
func (g *Gateway) Stop() {
	g.running = false
	if g.port != nil {
		g.port.Close()
	}
	g.printStats()
}

// connectSerial connects to the ESP32 bridge
func (g *Gateway) connectSerial() error {
	mode := &serial.Mode{
		BaudRate: g.config.BaudRate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(g.config.SerialPort, mode)
	if err != nil {
		return err
	}

	g.portMu.Lock()
	g.port = port
	g.portMu.Unlock()

	log.Printf("Serial connected: %s", g.config.SerialPort)
	return nil
}

// readSerial reads and processes serial data
func (g *Gateway) readSerial() {
	reader := bufio.NewReader(g.port)

	for g.running {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				log.Println("Serial EOF, reconnecting...")
				time.Sleep(5 * time.Second)
				if err := g.connectSerial(); err != nil {
					log.Printf("Reconnect failed: %v", err)
					continue
				}
				reader = bufio.NewReader(g.port)
				continue
			}
			log.Printf("Serial read error: %v", err)
			continue
		}

		line = line[:len(line)-1]
		if len(line) == 0 {
			continue
		}

		if g.config.Debug {
			log.Printf("[SERIAL] %s", line)
		}

		g.processMessage(line)
	}
}

// processMessage processes a message from the bridge
func (g *Gateway) processMessage(line string) {
	var msg MeshMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return
	}

	g.stats.mu.Lock()
	g.stats.MessagesReceived++
	g.stats.mu.Unlock()

	switch msg.Type {
	case "mesh_data":
		g.handleMeshData(msg)
	case "node_connected":
		log.Printf("[MESH] Node connected: %d (total: %d)", msg.NodeID, msg.TotalNodes)
	case "node_disconnected":
		g.setNodeOffline(msg.NodeID)
		log.Printf("[MESH] Node disconnected: %d (total: %d)", msg.NodeID, msg.TotalNodes)
	case "ready":
		log.Printf("[BRIDGE] Ready: %s (ID: %d)", msg.Firmware, msg.NodeID)
	case "boot":
		log.Printf("[BRIDGE] Boot: %s", msg.Msg)
	}
}

// handleMeshData handles metrics from a mesh node
func (g *Gateway) handleMeshData(msg MeshMessage) {
	var data MetricsData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	if data.MsgType != "metrics" {
		return
	}

	// Update or create node
	g.nodesMu.Lock()
	node, exists := g.nodes[msg.From]
	if !exists {
		node = &MeshNode{NodeID: msg.From}
		g.nodes[msg.From] = node
		log.Printf("[NEW NODE] Discovered node %d (%s)", msg.From, data.NodeName)
	}

	node.NodeName = data.NodeName
	node.ChipID = data.System.ChipID
	node.MAC = data.System.MAC
	node.Platform = data.System.Platform
	node.Firmware = data.System.Firmware
	node.Temperature = data.Temperature
	node.Humidity = data.Humidity
	node.FreeHeap = data.System.FreeHeap
	node.RSSI = data.Mesh.RSSI
	node.DHTEnabled = data.DHTEnabled
	node.IsOnline = true
	node.LastSeen = time.Now()
	g.nodesMu.Unlock()

	if g.config.Debug {
		log.Printf("[METRICS] Node %d (%s): T=%.1f°C, H=%.1f%%",
			msg.From, data.NodeName, data.Temperature, data.Humidity)
	}
}

// setNodeOffline marks a node as offline
func (g *Gateway) setNodeOffline(nodeID uint32) {
	g.nodesMu.Lock()
	if node, exists := g.nodes[nodeID]; exists {
		node.IsOnline = false
	}
	g.nodesMu.Unlock()
}

// nodeTimeoutChecker marks nodes offline after timeout
func (g *Gateway) nodeTimeoutChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for g.running {
		<-ticker.C
		now := time.Now()
		g.nodesMu.Lock()
		for _, node := range g.nodes {
			if node.IsOnline && now.Sub(node.LastSeen) > 2*time.Minute {
				node.IsOnline = false
				log.Printf("[TIMEOUT] Node %d (%s) marked offline", node.NodeID, node.NodeName)
			}
		}
		g.nodesMu.Unlock()
	}
}

// batchSender sends aggregated metrics to backend
func (g *Gateway) batchSender() {
	ticker := time.NewTicker(g.config.BatchInterval)
	defer ticker.Stop()

	for g.running {
		<-ticker.C
		g.sendBatchMetrics()
	}
}

// sendBatchMetrics sends all node metrics in one request
func (g *Gateway) sendBatchMetrics() {
	g.nodesMu.RLock()
	if len(g.nodes) == 0 {
		g.nodesMu.RUnlock()
		return
	}

	// Collect online nodes
	var nodeMetrics []NodeMetric
	for _, node := range g.nodes {
		if node.IsOnline {
			nodeMetrics = append(nodeMetrics, NodeMetric{
				NodeID:      node.NodeID,
				NodeName:    node.NodeName,
				ChipID:      node.ChipID,
				MAC:         node.MAC,
				Platform:    node.Platform,
				Firmware:    node.Firmware,
				Temperature: node.Temperature,
				Humidity:    node.Humidity,
				FreeHeap:    node.FreeHeap,
				RSSI:        node.RSSI,
				DHTEnabled:  node.DHTEnabled,
			})
		}
	}
	g.nodesMu.RUnlock()

	if len(nodeMetrics) == 0 {
		return
	}

	// Build batch payload
	payload := BatchMetricsPayload{
		GatewayID: "rpi-gateway-01",
		Timestamp: time.Now(),
		Nodes:     nodeMetrics,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal batch: %v", err)
		return
	}

	// Send to backend
	url := g.config.BackendURL + "/api/v1/metrics/batch"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if g.config.GatewayToken != "" {
		req.Header.Set("X-Gateway-Token", g.config.GatewayToken)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		log.Printf("[BATCH] HTTP error: %v", err)
		g.stats.mu.Lock()
		g.stats.Errors++
		g.stats.mu.Unlock()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		g.stats.mu.Lock()
		g.stats.BatchesSent++
		g.stats.mu.Unlock()
		log.Printf("[BATCH] Sent %d nodes to backend", len(nodeMetrics))
	} else {
		log.Printf("[BATCH] Backend error: %d", resp.StatusCode)
	}
}

// SendCommand sends a command to a mesh node
func (g *Gateway) SendCommand(nodeID uint32, command string, value interface{}) error {
	g.portMu.Lock()
	defer g.portMu.Unlock()

	if g.port == nil {
		return fmt.Errorf("serial not connected")
	}

	cmd := map[string]interface{}{
		"type":   "send",
		"target": nodeID,
		"data": map[string]interface{}{
			"cmd":   command,
			"value": value,
		},
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	data = append(data, '\n')
	_, err = g.port.Write(data)
	if err != nil {
		return err
	}

	g.stats.mu.Lock()
	g.stats.CommandsSent++
	g.stats.mu.Unlock()

	log.Printf("[CMD] Sent '%s' to node %d", command, nodeID)
	return nil
}

// BroadcastCommand sends a command to all nodes
func (g *Gateway) BroadcastCommand(command string, value interface{}) error {
	g.portMu.Lock()
	defer g.portMu.Unlock()

	if g.port == nil {
		return fmt.Errorf("serial not connected")
	}

	cmd := map[string]interface{}{
		"type": "broadcast",
		"data": map[string]interface{}{
			"cmd":   command,
			"value": value,
		},
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	data = append(data, '\n')
	_, err = g.port.Write(data)
	if err != nil {
		return err
	}

	log.Printf("[CMD] Broadcast '%s' to all nodes", command)
	return nil
}

// ============== WEB SERVER ==============

func (g *Gateway) startWebServer() {
	mux := http.NewServeMux()

	// Pages
	mux.HandleFunc("/", g.handleIndex)

	// API
	mux.HandleFunc("/api/nodes", g.handleAPINodes)
	mux.HandleFunc("/api/nodes/", g.handleAPINode)
	mux.HandleFunc("/api/command", g.handleAPICommand)
	mux.HandleFunc("/api/broadcast", g.handleAPIBroadcast)
	mux.HandleFunc("/api/stats", g.handleAPIStats)

	addr := fmt.Sprintf(":%d", g.config.WebPort)
	log.Printf("Web UI started on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Web server error: %v", err)
	}
}

func (g *Gateway) handleIndex(w http.ResponseWriter, r *http.Request) {
	g.nodesMu.RLock()
	nodes := make([]*MeshNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodes = append(nodes, n)
	}
	g.nodesMu.RUnlock()

	// Sort by node ID
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeID < nodes[j].NodeID
	})

	g.stats.mu.Lock()
	stats := g.stats
	g.stats.mu.Unlock()

	data := map[string]interface{}{
		"Nodes":      nodes,
		"Stats":      stats,
		"BackendURL": g.config.BackendURL,
		"SerialPort": g.config.SerialPort,
		"Uptime":     time.Since(stats.StartTime).Round(time.Second).String(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Inline template if embedded templates failed
	if g.templates == nil {
		g.serveInlineHTML(w, data)
		return
	}

	if err := g.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		g.serveInlineHTML(w, data)
	}
}

func (g *Gateway) serveInlineHTML(w http.ResponseWriter, data map[string]interface{}) {
	nodes := data["Nodes"].([]*MeshNode)
	stats := data["Stats"].(Stats)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Mesh Gateway</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #0f172a; color: #e2e8f0; padding: 20px; }
        .container { max-width: 1200px; margin: 0 auto; }
        h1 { color: #38bdf8; margin-bottom: 20px; }
        .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 16px; margin-bottom: 30px; }
        .stat { background: #1e293b; padding: 20px; border-radius: 12px; text-align: center; }
        .stat-value { font-size: 2rem; font-weight: bold; color: #38bdf8; }
        .stat-label { font-size: 0.875rem; color: #94a3b8; margin-top: 4px; }
        .nodes { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 16px; }
        .node { background: #1e293b; padding: 20px; border-radius: 12px; border: 1px solid #334155; }
        .node.offline { opacity: 0.6; }
        .node-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
        .node-name { font-size: 1.25rem; font-weight: 600; }
        .node-status { padding: 4px 12px; border-radius: 20px; font-size: 0.75rem; }
        .node-status.online { background: #166534; color: #4ade80; }
        .node-status.offline { background: #7f1d1d; color: #f87171; }
        .node-metrics { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-bottom: 16px; }
        .metric { text-align: center; padding: 12px; background: #0f172a; border-radius: 8px; }
        .metric-value { font-size: 1.5rem; font-weight: bold; }
        .metric-label { font-size: 0.75rem; color: #94a3b8; }
        .temp { color: #fb923c; }
        .hum { color: #22d3ee; }
        .rssi { color: #a78bfa; }
        .node-info { font-size: 0.75rem; color: #64748b; }
        .actions { display: flex; gap: 8px; margin-top: 12px; }
        .btn { padding: 8px 16px; border: none; border-radius: 8px; cursor: pointer; font-size: 0.875rem; }
        .btn-primary { background: #2563eb; color: white; }
        .btn-danger { background: #dc2626; color: white; }
        .btn:hover { opacity: 0.9; }
        .broadcast-section { background: #1e293b; padding: 20px; border-radius: 12px; margin-top: 30px; }
        .broadcast-section h2 { margin-bottom: 16px; color: #38bdf8; }
        .broadcast-buttons { display: flex; gap: 12px; flex-wrap: wrap; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🌐 Mesh Gateway</h1>
        
        <div class="stats">
            <div class="stat">
                <div class="stat-value">%d</div>
                <div class="stat-label">Nodes</div>
            </div>
            <div class="stat">
                <div class="stat-value">%d</div>
                <div class="stat-label">Messages</div>
            </div>
            <div class="stat">
                <div class="stat-value">%d</div>
                <div class="stat-label">Batches Sent</div>
            </div>
            <div class="stat">
                <div class="stat-value">%s</div>
                <div class="stat-label">Uptime</div>
            </div>
        </div>

        <div class="nodes">`, len(nodes), stats.MessagesReceived, stats.BatchesSent, data["Uptime"])

	for _, node := range nodes {
		statusClass := "online"
		statusText := "Online"
		if !node.IsOnline {
			statusClass = "offline"
			statusText = "Offline"
		}

		html += fmt.Sprintf(`
            <div class="node %s">
                <div class="node-header">
                    <div class="node-name">%s</div>
                    <div class="node-status %s">%s</div>
                </div>
                <div class="node-metrics">
                    <div class="metric">
                        <div class="metric-value temp">%.1f°</div>
                        <div class="metric-label">Temperature</div>
                    </div>
                    <div class="metric">
                        <div class="metric-value hum">%.0f%%</div>
                        <div class="metric-label">Humidity</div>
                    </div>
                    <div class="metric">
                        <div class="metric-value rssi">%d</div>
                        <div class="metric-label">RSSI</div>
                    </div>
                </div>
                <div class="node-info">
                    ID: %d | %s | %s | %s
                </div>
                <div class="actions">
                    <button class="btn btn-primary" onclick="sendCmd(%d, 'status')">Refresh</button>
                    <button class="btn btn-danger" onclick="sendCmd(%d, 'reboot')">Reboot</button>
                </div>
            </div>`,
			statusClass, node.NodeName, statusClass, statusText,
			node.Temperature, node.Humidity, node.RSSI,
			node.NodeID, node.Platform, node.ChipID, node.MAC,
			node.NodeID, node.NodeID)
	}

	html += `
        </div>

        <div class="broadcast-section">
            <h2>📡 Broadcast Commands</h2>
            <div class="broadcast-buttons">
                <button class="btn btn-primary" onclick="broadcast('status')">Request All Status</button>
                <button class="btn btn-danger" onclick="broadcast('reboot')">Reboot All</button>
            </div>
        </div>
    </div>

    <script>
        function sendCmd(nodeId, cmd) {
            fetch('/api/command', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({node_id: nodeId, command: cmd})
            }).then(r => r.json()).then(d => {
                alert(d.message || d.error);
                if(d.message) location.reload();
            });
        }
        function broadcast(cmd) {
            if(!confirm('Send ' + cmd + ' to ALL nodes?')) return;
            fetch('/api/broadcast', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({command: cmd})
            }).then(r => r.json()).then(d => {
                alert(d.message || d.error);
            });
        }
        // Auto-refresh every 10 seconds
        setTimeout(() => location.reload(), 10000);
    </script>
</body>
</html>`

	w.Write([]byte(html))
}

// API Handlers
func (g *Gateway) handleAPINodes(w http.ResponseWriter, r *http.Request) {
	g.nodesMu.RLock()
	nodes := make([]*MeshNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodes = append(nodes, n)
	}
	g.nodesMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func (g *Gateway) handleAPINode(w http.ResponseWriter, r *http.Request) {
	// Extract node ID from URL
	// TODO: implement single node operations
	w.WriteHeader(http.StatusNotImplemented)
}

func (g *Gateway) handleAPICommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		NodeID  uint32 `json:"node_id"`
		Command string `json:"command"`
		Value   interface{} `json:"value,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := g.SendCommand(req.NodeID, req.Command, req.Value); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprintf("Command '%s' sent to node %d", req.Command, req.NodeID)})
}

func (g *Gateway) handleAPIBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command string      `json:"command"`
		Value   interface{} `json:"value,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := g.BroadcastCommand(req.Command, req.Value); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprintf("Command '%s' broadcasted to all nodes", req.Command)})
}

func (g *Gateway) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	g.stats.mu.Lock()
	stats := map[string]interface{}{
		"messages_received": g.stats.MessagesReceived,
		"batches_sent":      g.stats.BatchesSent,
		"commands_sent":     g.stats.CommandsSent,
		"errors":            g.stats.Errors,
		"uptime_seconds":    time.Since(g.stats.StartTime).Seconds(),
	}
	g.stats.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (g *Gateway) printStats() {
	g.stats.mu.Lock()
	defer g.stats.mu.Unlock()

	uptime := time.Since(g.stats.StartTime)
	log.Println("==================================================")
	log.Println("Gateway Statistics")
	log.Printf("Uptime: %s", uptime.Round(time.Second))
	log.Printf("Messages received: %d", g.stats.MessagesReceived)
	log.Printf("Batches sent: %d", g.stats.BatchesSent)
	log.Printf("Commands sent: %d", g.stats.CommandsSent)
	log.Printf("Errors: %d", g.stats.Errors)
	log.Println("==================================================")
}
