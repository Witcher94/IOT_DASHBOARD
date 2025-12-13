package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.bug.st/serial"
)

// Config holds gateway configuration
type Config struct {
	SerialPort  string
	BaudRate    int
	BackendURL  string
	Debug       bool
	PollInterval time.Duration
}

// Gateway handles mesh-to-cloud communication
type Gateway struct {
	config     Config
	port       serial.Port
	httpClient *http.Client
	nodeTokens map[string]string
	tokenMu    sync.RWMutex
	running    bool
	stats      Stats
}

// Stats tracks gateway statistics
type Stats struct {
	MessagesReceived  int64
	MessagesSent      int64
	CommandsSent      int64
	Errors            int64
	StartTime         time.Time
	mu                sync.Mutex
}

// MeshMessage represents a message from the mesh bridge
type MeshMessage struct {
	Type      string          `json:"type"`
	From      uint32          `json:"from,omitempty"`
	NodeID    uint32          `json:"node_id,omitempty"`
	Timestamp uint64          `json:"timestamp,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	TotalNodes int            `json:"total_nodes,omitempty"`
	Msg       string          `json:"msg,omitempty"`
	Firmware  string          `json:"firmware,omitempty"`
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

// BackendPayload is the payload format for the cloud backend
type BackendPayload struct {
	NodeName    string  `json:"node_name"`
	Temperature float64 `json:"temperature,omitempty"`
	Humidity    float64 `json:"humidity,omitempty"`
	DHTEnabled  bool    `json:"dht_enabled"`
	System      struct {
		ChipID   string `json:"chip_id"`
		MAC      string `json:"mac"`
		Firmware string `json:"firmware"`
		Platform string `json:"platform"`
		FreeHeap int64  `json:"free_heap"`
	} `json:"system"`
	CurrentWifi struct {
		SSID    string `json:"ssid"`
		RSSI    int    `json:"rssi"`
		BSSID   string `json:"bssid"`
		IP      string `json:"ip"`
		Channel int    `json:"channel"`
	} `json:"current_wifi"`
	MeshStatus struct {
		Enabled   bool   `json:"enabled"`
		Running   bool   `json:"running"`
		NodeID    uint32 `json:"node_id"`
		NodeCount int    `json:"node_count"`
	} `json:"mesh_status"`
}

func main() {
	// Parse flags
	serialPort := flag.String("port", "/dev/ttyUSB0", "Serial port for ESP32 bridge")
	baudRate := flag.Int("baud", 115200, "Baud rate")
	backendURL := flag.String("backend", "https://chnu-iot.com", "Backend URL")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	config := Config{
		SerialPort:   *serialPort,
		BaudRate:     *baudRate,
		BackendURL:   *backendURL,
		Debug:        *debug,
		PollInterval: 5 * time.Second,
	}

	gateway := NewGateway(config)

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		gateway.Stop()
	}()

	// Run gateway
	if err := gateway.Run(); err != nil {
		log.Fatalf("Gateway error: %v", err)
	}
}

// NewGateway creates a new gateway instance
func NewGateway(config Config) *Gateway {
	return &Gateway{
		config:     config,
		nodeTokens: make(map[string]string),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		stats: Stats{
			StartTime: time.Now(),
		},
	}
}

// Run starts the gateway
func (g *Gateway) Run() error {
	log.Println("==================================================")
	log.Println("Mesh Gateway (Go) Starting")
	log.Printf("Serial: %s @ %d", g.config.SerialPort, g.config.BaudRate)
	log.Printf("Backend: %s", g.config.BackendURL)
	log.Printf("Debug: %v", g.config.Debug)
	log.Println("==================================================")

	// Connect to serial
	if err := g.connectSerial(); err != nil {
		return fmt.Errorf("serial connection failed: %w", err)
	}
	defer g.port.Close()

	g.running = true

	// Start reading
	g.readSerial()

	g.printStats()
	return nil
}

// Stop stops the gateway
func (g *Gateway) Stop() {
	g.running = false
	if g.port != nil {
		g.port.Close()
	}
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

	g.port = port
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
				}
				reader = bufio.NewReader(g.port)
				continue
			}
			log.Printf("Serial read error: %v", err)
			continue
		}

		line = line[:len(line)-1] // Remove newline
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
		if g.config.Debug {
			log.Printf("Non-JSON: %s", line)
		}
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
		log.Printf("[MESH] Node disconnected: %d (total: %d)", msg.NodeID, msg.TotalNodes)
	case "heartbeat":
		if g.config.Debug {
			log.Printf("[HEARTBEAT] Nodes: %d", msg.TotalNodes)
		}
	case "ready":
		log.Printf("[BRIDGE] Ready: %s (ID: %d)", msg.Firmware, msg.NodeID)
	case "boot":
		log.Printf("[BRIDGE] Boot: %s", msg.Msg)
	case "ack":
		if g.config.Debug {
			log.Println("[ACK] Command acknowledged")
		}
	case "error":
		log.Printf("[ERROR] Bridge error: %s", msg.Msg)
	default:
		if g.config.Debug {
			log.Printf("[UNKNOWN] Type: %s", msg.Type)
		}
	}
}

// handleMeshData handles metrics data from a mesh node
func (g *Gateway) handleMeshData(msg MeshMessage) {
	var data MetricsData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("Failed to parse mesh data: %v", err)
		return
	}

	if data.MsgType != "metrics" {
		return
	}

	log.Printf("[METRICS] Node %d (%s): T=%.1f°C, H=%.1f%%",
		msg.From, data.NodeName, data.Temperature, data.Humidity)

	// Send to backend
	go g.sendToBackend(msg.From, data)
}

// sendToBackend sends metrics to the cloud backend
func (g *Gateway) sendToBackend(nodeID uint32, data MetricsData) {
	// Get token for this node
	g.tokenMu.RLock()
	token := g.nodeTokens[fmt.Sprintf("%d", nodeID)]
	g.tokenMu.RUnlock()

	if token == "" {
		log.Printf("[WARN] No token for node %d. Add device in dashboard and set token here.", nodeID)
		log.Printf("[INFO] Node info: ChipID=%s, MAC=%s, Platform=%s",
			data.System.ChipID, data.System.MAC, data.System.Platform)
		return
	}

	// Build payload
	payload := BackendPayload{
		NodeName:    data.NodeName,
		Temperature: data.Temperature,
		Humidity:    data.Humidity,
		DHTEnabled:  data.DHTEnabled,
	}
	payload.System.ChipID = data.System.ChipID
	payload.System.MAC = data.System.MAC
	payload.System.Firmware = data.System.Firmware
	payload.System.Platform = data.System.Platform
	payload.System.FreeHeap = data.System.FreeHeap
	payload.CurrentWifi.SSID = "mesh_network"
	payload.CurrentWifi.RSSI = data.Mesh.RSSI
	payload.CurrentWifi.Channel = 6
	payload.MeshStatus.Enabled = true
	payload.MeshStatus.Running = true
	payload.MeshStatus.NodeID = nodeID
	payload.MeshStatus.NodeCount = data.Mesh.NodeCount

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal payload: %v", err)
		return
	}

	// Send HTTP request
	url := g.config.BackendURL + "/api/v1/metrics"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-Token", token)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		log.Printf("HTTP error: %v", err)
		g.stats.mu.Lock()
		g.stats.Errors++
		g.stats.mu.Unlock()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		g.stats.mu.Lock()
		g.stats.MessagesSent++
		g.stats.mu.Unlock()
		if g.config.Debug {
			log.Printf("[HTTP] Sent to backend for node %d", nodeID)
		}
	} else if resp.StatusCode == 401 {
		log.Printf("[HTTP] Invalid token for node %d", nodeID)
		g.tokenMu.Lock()
		delete(g.nodeTokens, fmt.Sprintf("%d", nodeID))
		g.tokenMu.Unlock()
	} else {
		log.Printf("[HTTP] Backend error: %d", resp.StatusCode)
	}
}

// SendCommand sends a command to a mesh node
func (g *Gateway) SendCommand(nodeID uint32, command string, value interface{}) error {
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

	log.Printf("[CMD] Sent %s to node %d", command, nodeID)
	return nil
}

// SetNodeToken sets the token for a mesh node
func (g *Gateway) SetNodeToken(nodeID string, token string) {
	g.tokenMu.Lock()
	g.nodeTokens[nodeID] = token
	g.tokenMu.Unlock()
	log.Printf("Token set for node %s", nodeID)
}

// printStats prints gateway statistics
func (g *Gateway) printStats() {
	g.stats.mu.Lock()
	defer g.stats.mu.Unlock()

	uptime := time.Since(g.stats.StartTime)
	log.Println("==================================================")
	log.Println("Gateway Statistics")
	log.Printf("Uptime: %s", uptime.Round(time.Second))
	log.Printf("Messages received: %d", g.stats.MessagesReceived)
	log.Printf("Sent to backend: %d", g.stats.MessagesSent)
	log.Printf("Commands sent: %d", g.stats.CommandsSent)
	log.Printf("Errors: %d", g.stats.Errors)
	log.Println("==================================================")
}

