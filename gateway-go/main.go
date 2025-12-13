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
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.bug.st/serial"
	"gopkg.in/yaml.v3"
)

const VERSION = "2.2.0"
const TOKEN_FILE = "/etc/mesh-gateway/token"

// ConfigFile represents the YAML config structure
type ConfigFile struct {
	Serial struct {
		Port string `yaml:"port"`
		Baud int    `yaml:"baud"`
	} `yaml:"serial"`
	Backend struct {
		URL           string `yaml:"url"`
		Token         string `yaml:"token"`
		BatchInterval int    `yaml:"batch_interval"`
	} `yaml:"backend"`
	Web struct {
		Port    int  `yaml:"port"`
		Enabled bool `yaml:"enabled"`
	} `yaml:"web"`
	Logging struct {
		Level string `yaml:"level"`
		File  string `yaml:"file"`
	} `yaml:"logging"`
	Nodes struct {
		Timeout      int  `yaml:"timeout"`
		AutoRegister bool `yaml:"auto_register"`
	} `yaml:"nodes"`
}

// Config holds runtime configuration
type Config struct {
	SerialPort    string
	BaudRate      int
	BackendURL    string
	GatewayToken  string
	WebPort       int
	WebEnabled    bool
	BatchInterval time.Duration
	NodeTimeout   time.Duration
	Debug         bool
	LogFile       string
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
	IsOnline    bool      `json:"is_online"`
	IsRoot      bool      `json:"is_root"`
	LastSeen    time.Time `json:"last_seen"`
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
	logFile    *os.File
}

// Stats tracks gateway statistics
type Stats struct {
	MessagesReceived int64
	MetricsReceived  int64
	BatchesSent      int64
	CommandsSent     int64
	Errors           int64
	LastBatchTime    time.Time
	StartTime        time.Time
	mu               sync.Mutex
}

// MeshMessage from ESP32 bridge
type MeshMessage struct {
	Type       string          `json:"type"`
	From       uint32          `json:"from,omitempty"`
	NodeID     uint32          `json:"node_id,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	TotalNodes int             `json:"total,omitempty"`
	Msg        string          `json:"msg,omitempty"`
	Firmware   string          `json:"firmware,omitempty"`
	Nodes      int             `json:"nodes,omitempty"`
	Heap       uint32          `json:"heap,omitempty"`
	Uptime     uint64          `json:"uptime,omitempty"`
	Temp       float64         `json:"temp,omitempty"`
	Hum        float64         `json:"hum,omitempty"`
}

// MetricsData from mesh node
type MetricsData struct {
	MsgType     string  `json:"msg_type"`
	NodeName    string  `json:"node_name"`
	NodeID      uint32  `json:"node_id"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	ChipID      string  `json:"chip_id"`
	MAC         string  `json:"mac"`
	Firmware    string  `json:"firmware"`
	Platform    string  `json:"platform"`
	FreeHeap    int64   `json:"free_heap"`
	RSSI        int     `json:"rssi"`
	IsRoot      bool    `json:"is_root"`
}

// BatchMetricsPayload for backend
type BatchMetricsPayload struct {
	GatewayToken string       `json:"gateway_token"`
	Timestamp    time.Time    `json:"timestamp"`
	Nodes        []NodeMetric `json:"nodes"`
}

type NodeMetric struct {
	MeshNodeID  uint32  `json:"mesh_node_id"`
	NodeName    string  `json:"node_name"`
	ChipID      string  `json:"chip_id"`
	MAC         string  `json:"mac"`
	Platform    string  `json:"platform"`
	Firmware    string  `json:"firmware"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	FreeHeap    int64   `json:"free_heap"`
	RSSI        int     `json:"rssi"`
	IsRoot      bool    `json:"is_root"`
}

func main() {
	// Parse flags
	configPath := flag.String("config", "", "Path to config file (YAML)")
	serialPort := flag.String("port", "/dev/ttyUSB0", "Serial port")
	baudRate := flag.Int("baud", 115200, "Baud rate")
	backendURL := flag.String("backend", "https://chnu-iot.com", "Backend URL")
	gatewayToken := flag.String("token", "", "Gateway token")
	webPort := flag.Int("web", 8080, "Web UI port")
	batchInterval := flag.Int("interval", 30, "Batch interval (seconds)")
	debug := flag.Bool("debug", false, "Debug mode")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Mesh Gateway v%s\n", VERSION)
		os.Exit(0)
	}

	// Build config
	config := Config{
		SerialPort:    *serialPort,
		BaudRate:      *baudRate,
		BackendURL:    *backendURL,
		GatewayToken:  *gatewayToken,
		WebPort:       *webPort,
		WebEnabled:    true,
		BatchInterval: time.Duration(*batchInterval) * time.Second,
		NodeTimeout:   2 * time.Minute,
		Debug:         *debug,
	}

	// Load config file if specified
	if *configPath != "" {
		if err := loadConfigFile(*configPath, &config); err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	}

	// Override with env vars
	if token := os.Getenv("GATEWAY_TOKEN"); token != "" {
		config.GatewayToken = token
	}

	// Load token from file if not set
	if config.GatewayToken == "" {
		if token := loadTokenFromFile(); token != "" {
			config.GatewayToken = token
			log.Println("Token loaded from file")
		}
	}

	gateway := NewGateway(config)

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down gracefully...")
		gateway.Stop()
		os.Exit(0)
	}()

	if err := gateway.Run(); err != nil {
		log.Fatalf("Gateway error: %v", err)
	}
}

// Token file management
func loadTokenFromFile() string {
	data, err := os.ReadFile(TOKEN_FILE)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveTokenToFile(token string) error {
	dir := "/etc/mesh-gateway"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(TOKEN_FILE, []byte(token), 0600)
}

func loadConfigFile(path string, config *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var cf ConfigFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return err
	}

	// Apply config
	if cf.Serial.Port != "" {
		config.SerialPort = cf.Serial.Port
	}
	if cf.Serial.Baud > 0 {
		config.BaudRate = cf.Serial.Baud
	}
	if cf.Backend.URL != "" {
		config.BackendURL = cf.Backend.URL
	}
	if cf.Backend.Token != "" {
		config.GatewayToken = cf.Backend.Token
	}
	if cf.Backend.BatchInterval > 0 {
		config.BatchInterval = time.Duration(cf.Backend.BatchInterval) * time.Second
	}
	if cf.Web.Port > 0 {
		config.WebPort = cf.Web.Port
	}
	config.WebEnabled = cf.Web.Enabled
	if cf.Nodes.Timeout > 0 {
		config.NodeTimeout = time.Duration(cf.Nodes.Timeout) * time.Second
	}
	if cf.Logging.Level == "debug" {
		config.Debug = true
	}
	config.LogFile = cf.Logging.File

	return nil
}

func NewGateway(config Config) *Gateway {
	return &Gateway{
		config:     config,
		nodes:      make(map[uint32]*MeshNode),
		httpClient: &http.Client{Timeout: 15 * time.Second},
		stats:      Stats{StartTime: time.Now()},
	}
}

func (g *Gateway) Run() error {
	// Setup logging
	if g.config.LogFile != "" {
		f, err := os.OpenFile(g.config.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			g.logFile = f
			log.SetOutput(io.MultiWriter(os.Stdout, f))
		}
	}

	log.Println("╔══════════════════════════════════════════════════════════╗")
	log.Printf("║  🌐 Mesh Gateway v%s                               ║", VERSION)
	log.Println("╚══════════════════════════════════════════════════════════╝")
	log.Printf("Serial:   %s @ %d baud", g.config.SerialPort, g.config.BaudRate)
	log.Printf("Backend:  %s", g.config.BackendURL)
	log.Printf("Token:    %s", maskToken(g.config.GatewayToken))
	log.Printf("Web UI:   http://0.0.0.0:%d", g.config.WebPort)
	log.Printf("Interval: %s", g.config.BatchInterval)
	log.Println("────────────────────────────────────────────────────────────")

	// Validate token
	if g.config.GatewayToken == "" {
		log.Println("⚠️  WARNING: No gateway token configured!")
		log.Println("   Metrics will NOT be sent to backend.")
		log.Println("   Set token in config file or use --token flag")
	}

	// Connect serial
	if err := g.connectSerial(); err != nil {
		log.Printf("⚠️  Serial connection failed: %v", err)
		log.Println("   Running in web-only mode...")
	} else {
		log.Println("✅ Serial connected")
	}

	g.running = true

	// Start workers
	go g.batchSender()
	go g.nodeTimeoutChecker()
	go g.startWebServer()

	// Serial reader (blocking)
	if g.port != nil {
		g.readSerial()
	} else {
		select {} // Wait forever
	}

	return nil
}

func maskToken(token string) string {
	if token == "" {
		return "(not set)"
	}
	if len(token) > 8 {
		return token[:4] + "..." + token[len(token)-4:]
	}
	return "****"
}

func (g *Gateway) Stop() {
	g.running = false
	if g.port != nil {
		g.port.Close()
	}
	if g.logFile != nil {
		g.logFile.Close()
	}
	g.printStats()
}

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

	return nil
}

func (g *Gateway) readSerial() {
	reader := bufio.NewReader(g.port)
	reconnectDelay := 5 * time.Second

	for g.running {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				log.Println("Serial disconnected, reconnecting...")
				time.Sleep(reconnectDelay)
				if err := g.connectSerial(); err != nil {
					log.Printf("Reconnect failed: %v", err)
					continue
				}
				reader = bufio.NewReader(g.port)
				log.Println("✅ Serial reconnected")
				continue
			}
			log.Printf("Serial error: %v", err)
			continue
		}

		line = strings.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue // Skip non-JSON
		}

		if g.config.Debug {
			log.Printf("[RX] %s", line)
		}

		g.processMessage(line)
	}
}

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
	case "heartbeat":
		g.handleHeartbeat(msg)
	case "node_connected":
		log.Printf("📡 Node connected: %d (total: %d)", msg.NodeID, msg.TotalNodes)
	case "node_disconnected":
		g.setNodeOffline(msg.NodeID)
		log.Printf("📴 Node disconnected: %d", msg.NodeID)
	case "ready":
		log.Printf("✅ Bridge ready: %s (ID: %d)", msg.Firmware, msg.NodeID)
	case "boot":
		log.Printf("🔄 Bridge booting: %s", msg.Firmware)
	case "ack":
		if g.config.Debug {
			log.Printf("[ACK] Command acknowledged")
		}
	}
}

func (g *Gateway) handleHeartbeat(msg MeshMessage) {
	// Bridge sends heartbeat with its own metrics
	if msg.Temp > 0 || msg.Hum > 0 {
		g.nodesMu.Lock()
		node, exists := g.nodes[msg.NodeID]
		if !exists {
			node = &MeshNode{
				NodeID:   msg.NodeID,
				NodeName: "Bridge",
				IsRoot:   true,
				Platform: "ESP32",
			}
			g.nodes[msg.NodeID] = node
			log.Printf("🌟 Bridge node registered: %d", msg.NodeID)
		}
		node.Temperature = msg.Temp
		node.Humidity = msg.Hum
		node.FreeHeap = int64(msg.Heap)
		node.IsOnline = true
		node.LastSeen = time.Now()
		g.nodesMu.Unlock()
	}
}

func (g *Gateway) handleMeshData(msg MeshMessage) {
	var data MetricsData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		// Try flat format (from bridge itself)
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}
	}

	if data.MsgType != "metrics" {
		return
	}

	g.stats.mu.Lock()
	g.stats.MetricsReceived++
	g.stats.mu.Unlock()

	// Update node
	g.nodesMu.Lock()
	node, exists := g.nodes[msg.From]
	if !exists {
		node = &MeshNode{NodeID: msg.From}
		g.nodes[msg.From] = node
		log.Printf("🆕 New node discovered: %d (%s)", msg.From, data.NodeName)
	}

	node.NodeName = data.NodeName
	node.ChipID = data.ChipID
	node.MAC = data.MAC
	node.Platform = data.Platform
	node.Firmware = data.Firmware
	node.Temperature = data.Temperature
	node.Humidity = data.Humidity
	node.FreeHeap = data.FreeHeap
	node.RSSI = data.RSSI
	node.IsRoot = data.IsRoot
	node.IsOnline = true
	node.LastSeen = time.Now()
	g.nodesMu.Unlock()

	log.Printf("📊 [%s] T=%.1f°C H=%.0f%% RSSI=%d", data.NodeName, data.Temperature, data.Humidity, data.RSSI)
}

func (g *Gateway) setNodeOffline(nodeID uint32) {
	g.nodesMu.Lock()
	if node, exists := g.nodes[nodeID]; exists {
		node.IsOnline = false
	}
	g.nodesMu.Unlock()
}

func (g *Gateway) nodeTimeoutChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for g.running {
		<-ticker.C
		now := time.Now()
		g.nodesMu.Lock()
		for _, node := range g.nodes {
			if node.IsOnline && now.Sub(node.LastSeen) > g.config.NodeTimeout {
				node.IsOnline = false
				log.Printf("⏰ Node %d (%s) timed out", node.NodeID, node.NodeName)
			}
		}
		g.nodesMu.Unlock()
	}
}

func (g *Gateway) batchSender() {
	ticker := time.NewTicker(g.config.BatchInterval)
	defer ticker.Stop()

	for g.running {
		<-ticker.C
		g.sendBatchMetrics()
	}
}

func (g *Gateway) sendBatchMetrics() {
	if g.config.GatewayToken == "" {
		return // No token, skip
	}

	g.nodesMu.RLock()
	var metrics []NodeMetric
	for _, node := range g.nodes {
		if node.IsOnline {
			metrics = append(metrics, NodeMetric{
				MeshNodeID:  node.NodeID,
				NodeName:    node.NodeName,
				ChipID:      node.ChipID,
				MAC:         node.MAC,
				Platform:    node.Platform,
				Firmware:    node.Firmware,
				Temperature: node.Temperature,
				Humidity:    node.Humidity,
				FreeHeap:    node.FreeHeap,
				RSSI:        node.RSSI,
				IsRoot:      node.IsRoot,
			})
		}
	}
	g.nodesMu.RUnlock()

	if len(metrics) == 0 {
		return
	}

	payload := BatchMetricsPayload{
		GatewayToken: g.config.GatewayToken,
		Timestamp:    time.Now(),
		Nodes:        metrics,
	}

	jsonData, _ := json.Marshal(payload)

	url := g.config.BackendURL + "/api/v1/gateway/metrics"
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gateway-Token", g.config.GatewayToken)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		log.Printf("❌ Backend error: %v", err)
		g.stats.mu.Lock()
		g.stats.Errors++
		g.stats.mu.Unlock()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		g.stats.mu.Lock()
		g.stats.BatchesSent++
		g.stats.LastBatchTime = time.Now()
		g.stats.mu.Unlock()
		log.Printf("✅ Sent %d nodes to backend", len(metrics))
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("❌ Backend responded %d: %s", resp.StatusCode, string(body))
		g.stats.mu.Lock()
		g.stats.Errors++
		g.stats.mu.Unlock()
	}
}

func (g *Gateway) SendCommand(nodeID uint32, command string, value interface{}) error {
	g.portMu.Lock()
	defer g.portMu.Unlock()

	if g.port == nil {
		return fmt.Errorf("serial not connected")
	}

	cmd := map[string]interface{}{
		"type":   "send",
		"target": nodeID,
		"data":   map[string]interface{}{"cmd": command, "value": value},
	}

	data, _ := json.Marshal(cmd)
	data = append(data, '\n')

	if _, err := g.port.Write(data); err != nil {
		return err
	}

	g.stats.mu.Lock()
	g.stats.CommandsSent++
	g.stats.mu.Unlock()

	log.Printf("📤 Command '%s' → Node %d", command, nodeID)
	return nil
}

func (g *Gateway) BroadcastCommand(command string, value interface{}) error {
	g.portMu.Lock()
	defer g.portMu.Unlock()

	if g.port == nil {
		return fmt.Errorf("serial not connected")
	}

	cmd := map[string]interface{}{
		"type": "broadcast",
		"data": map[string]interface{}{"cmd": command, "value": value},
	}

	data, _ := json.Marshal(cmd)
	data = append(data, '\n')

	if _, err := g.port.Write(data); err != nil {
		return err
	}

	log.Printf("📢 Broadcast '%s' to all nodes", command)
	return nil
}

// ============== WEB SERVER ==============

func (g *Gateway) startWebServer() {
	if !g.config.WebEnabled {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", g.handleIndex)
	mux.HandleFunc("/settings", g.handleSettings)
	mux.HandleFunc("/api/nodes", g.handleAPINodes)
	mux.HandleFunc("/api/stats", g.handleAPIStats)
	mux.HandleFunc("/api/command", g.handleAPICommand)
	mux.HandleFunc("/api/broadcast", g.handleAPIBroadcast)
	mux.HandleFunc("/api/settings", g.handleAPISettings)

	addr := fmt.Sprintf(":%d", g.config.WebPort)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("Web server error: %v", err)
	}
}

func (g *Gateway) handleIndex(w http.ResponseWriter, r *http.Request) {
	g.nodesMu.RLock()
	nodes := make([]*MeshNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodes = append(nodes, n)
	}
	g.nodesMu.RUnlock()

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsRoot != nodes[j].IsRoot {
			return nodes[i].IsRoot
		}
		return nodes[i].NodeID < nodes[j].NodeID
	})

	g.stats.mu.Lock()
	messagesReceived := g.stats.MessagesReceived
	batchesSent := g.stats.BatchesSent
	startTime := g.stats.StartTime
	g.stats.mu.Unlock()

	onlineCount := 0
	for _, n := range nodes {
		if n.IsOnline {
			onlineCount++
		}
	}

	uptime := formatDuration(time.Since(startTime))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	g.renderHTML(w, nodes, onlineCount, messagesReceived, batchesSent, uptime)
}

func (g *Gateway) renderHTML(w http.ResponseWriter, nodes []*MeshNode, onlineCount int, messages, batches int64, uptime string) {
	totalNodes := len(nodes)
	hasToken := g.config.GatewayToken != ""

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width,initial-scale=1,maximum-scale=1,user-scalable=no">
    <title>Mesh Gateway</title>
    <link rel="icon" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🌐</text></svg>">
    <style>
        :root{--bg:#0f172a;--card:#1e293b;--border:#334155;--text:#e2e8f0;--muted:#94a3b8;--primary:#38bdf8;--success:#22c55e;--danger:#ef4444;--warning:#f59e0b}
        *{box-sizing:border-box;margin:0;padding:0;-webkit-tap-highlight-color:transparent}
        body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;background:var(--bg);color:var(--text);min-height:100vh;min-height:100dvh;padding:12px;padding:max(12px,env(safe-area-inset-top)) max(12px,env(safe-area-inset-right)) max(12px,env(safe-area-inset-bottom)) max(12px,env(safe-area-inset-left))}
        .container{max-width:600px;margin:0 auto}
        header{display:flex;justify-content:space-between;align-items:center;margin-bottom:16px;gap:8px}
        h1{font-size:1.25rem;display:flex;align-items:center;gap:8px;flex:1}
        .version{font-size:0.625rem;background:var(--card);padding:3px 8px;border-radius:12px;color:var(--muted)}
        .settings-btn{width:40px;height:40px;border-radius:10px;background:var(--card);border:1px solid var(--border);color:var(--text);font-size:1.25rem;cursor:pointer;display:flex;align-items:center;justify-content:center}
        .stats{display:grid;grid-template-columns:repeat(3,1fr);gap:8px;margin-bottom:16px}
        .stat{background:var(--card);padding:12px 8px;border-radius:12px;text-align:center;border:1px solid var(--border)}
        .stat-value{font-size:1.5rem;font-weight:700;color:var(--primary);line-height:1}
        .stat-value.success{color:var(--success)}
        .stat-label{font-size:0.625rem;color:var(--muted);margin-top:4px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
        .nodes{display:flex;flex-direction:column;gap:12px;margin-bottom:16px}
        .node{background:var(--card);border-radius:16px;border:1px solid var(--border);overflow:hidden}
        .node.offline{opacity:0.5}
        .node.root{border-color:var(--warning);border-width:2px}
        .node-header{display:flex;justify-content:space-between;align-items:center;padding:14px 16px}
        .node-name{font-weight:600;font-size:1rem;display:flex;align-items:center;gap:8px}
        .root-badge{font-size:0.5rem;background:var(--warning);color:#000;padding:2px 6px;border-radius:4px;font-weight:700;text-transform:uppercase}
        .status{width:12px;height:12px;border-radius:50%%;flex-shrink:0}
        .status.online{background:var(--success);box-shadow:0 0 8px var(--success)}
        .status.offline{background:var(--danger)}
        .metrics{display:grid;grid-template-columns:repeat(3,1fr);padding:0 12px 12px;gap:8px}
        .metric{text-align:center;padding:12px 8px;background:var(--bg);border-radius:12px}
        .metric-val{font-size:1.5rem;font-weight:700;line-height:1}
        .metric-val.temp{color:#fb923c}
        .metric-val.hum{color:#22d3ee}
        .metric-val.rssi{color:#a78bfa}
        .metric-lbl{font-size:0.5rem;color:var(--muted);text-transform:uppercase;margin-top:4px;letter-spacing:0.5px}
        .node-footer{display:flex;gap:8px;padding:12px;border-top:1px solid var(--border)}
        .btn{flex:1;padding:12px;border:none;border-radius:10px;cursor:pointer;font-size:0.875rem;font-weight:600;transition:all 0.15s;-webkit-appearance:none}
        .btn:active{transform:scale(0.97)}
        .btn-primary{background:var(--primary);color:#000}
        .btn-danger{background:var(--danger);color:#fff}
        .broadcast{background:var(--card);padding:16px;border-radius:16px;border:1px solid var(--border)}
        .broadcast h2{margin-bottom:12px;font-size:0.875rem;color:var(--muted)}
        .broadcast-btns{display:grid;grid-template-columns:1fr 1fr;gap:8px}
        .empty{text-align:center;padding:48px 24px;color:var(--muted)}
        .empty-icon{font-size:3rem;margin-bottom:12px}
        .empty-text{font-size:0.875rem}
        .token-warning{background:rgba(245,158,11,0.15);border:1px solid var(--warning);color:var(--warning);padding:12px 16px;border-radius:12px;margin-bottom:16px;font-size:0.875rem;display:flex;align-items:center;gap:8px}
        .token-warning a{color:var(--warning);font-weight:600}
    </style>
</head>
<body>
<div class="container">
    <header>
        <h1>🌐 Gateway <span class="version">v%s</span></h1>
        <button class="settings-btn" onclick="location.href='/settings'" title="Settings">⚙️</button>
    </header>
    %s
    <div class="stats">
        <div class="stat"><div class="stat-value success">%d</div><div class="stat-label">Online</div></div>
        <div class="stat"><div class="stat-value">%d</div><div class="stat-label">Messages</div></div>
        <div class="stat"><div class="stat-value">%s</div><div class="stat-label">Uptime</div></div>
    </div>
    <div class="nodes">`,
		VERSION,
		func() string {
			if !hasToken {
				return `<div class="token-warning">⚠️ <span>No token configured. <a href="/settings">Set up now</a></span></div>`
			}
			return ""
		}(),
		onlineCount, messages, uptime)
	if len(nodes) == 0 {
		html += `<div class="empty"><div class="empty-icon">📡</div><div class="empty-text">Waiting for mesh nodes...</div></div>`
	}
	_ = totalNodes // unused now

	for _, n := range nodes {
		statusClass := "online"
		nodeClass := ""
		if !n.IsOnline {
			statusClass = "offline"
			nodeClass = "offline"
		}
		if n.IsRoot {
			nodeClass += " root"
		}

		rootBadge := ""
		if n.IsRoot {
			rootBadge = `<span class="root-badge">ROOT</span>`
		}

		html += fmt.Sprintf(`
        <div class="node %s">
            <div class="node-header">
                <div class="node-name">%s %s</div>
                <div class="status %s"></div>
            </div>
            <div class="metrics">
                <div class="metric"><div class="metric-val temp">%.0f°</div><div class="metric-lbl">Temp</div></div>
                <div class="metric"><div class="metric-val hum">%.0f%%</div><div class="metric-lbl">Hum</div></div>
                <div class="metric"><div class="metric-val rssi">%d</div><div class="metric-lbl">RSSI</div></div>
            </div>
            <div class="node-footer">
                <button class="btn btn-primary" onclick="cmd(%d,'status')">🔄 Refresh</button>
                <button class="btn btn-danger" onclick="cmd(%d,'reboot')">🔃 Reboot</button>
            </div>
        </div>`,
			nodeClass, n.NodeName, rootBadge, statusClass,
			n.Temperature, n.Humidity, n.RSSI,
			n.NodeID, n.NodeID)
	}

	html += `
    </div>
    <div class="broadcast">
        <h2>📢 Broadcast</h2>
        <div class="broadcast-btns">
            <button class="btn btn-primary" onclick="bcast('status')">🔄 Refresh All</button>
            <button class="btn btn-danger" onclick="if(confirm('Reboot ALL?'))bcast('reboot')">🔃 Reboot All</button>
        </div>
    </div>
</div>
<script>
function cmd(id,c){
    fetch('/api/command',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({node_id:id,command:c})})
    .then(r=>r.json()).then(d=>{if(d.error)alert(d.error);else setTimeout(()=>location.reload(),1000)});
}
function bcast(c){
    fetch('/api/broadcast',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({command:c})})
    .then(r=>r.json()).then(d=>{if(d.error)alert(d.error);else setTimeout(()=>location.reload(),1000)});
}
setTimeout(()=>location.reload(),15000);
</script>
</body></html>`

	w.Write([]byte(html))
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

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

func (g *Gateway) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	g.stats.mu.Lock()
	data := map[string]interface{}{
		"messages":       g.stats.MessagesReceived,
		"metrics":        g.stats.MetricsReceived,
		"batches":        g.stats.BatchesSent,
		"commands":       g.stats.CommandsSent,
		"errors":         g.stats.Errors,
		"uptime_seconds": time.Since(g.stats.StartTime).Seconds(),
	}
	g.stats.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (g *Gateway) handleAPICommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var req struct {
		NodeID  uint32 `json:"node_id"`
		Command string `json:"command"`
		Value   any    `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if err := g.SendCommand(req.NodeID, req.Command, req.Value); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Command sent"})
}

func (g *Gateway) handleAPIBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var req struct {
		Command string `json:"command"`
		Value   any    `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if err := g.BroadcastCommand(req.Command, req.Value); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Broadcast sent"})
}

func (g *Gateway) printStats() {
	g.stats.mu.Lock()
	defer g.stats.mu.Unlock()
	log.Println("════════════════════════════════════════")
	log.Println("📊 Final Statistics")
	log.Printf("   Uptime: %s", formatDuration(time.Since(g.stats.StartTime)))
	log.Printf("   Messages: %d", g.stats.MessagesReceived)
	log.Printf("   Batches: %d", g.stats.BatchesSent)
	log.Printf("   Commands: %d", g.stats.CommandsSent)
	log.Printf("   Errors: %d", g.stats.Errors)
	log.Println("════════════════════════════════════════")
}

func (g *Gateway) handleSettings(w http.ResponseWriter, r *http.Request) {
	hasToken := g.config.GatewayToken != ""
	tokenMasked := ""
	if hasToken {
		t := g.config.GatewayToken
		if len(t) > 8 {
			tokenMasked = t[:4] + "..." + t[len(t)-4:]
		} else {
			tokenMasked = "****"
		}
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width,initial-scale=1">
    <title>Gateway Settings</title>
    <style>
        :root{--bg:#0f172a;--card:#1e293b;--border:#334155;--text:#e2e8f0;--muted:#94a3b8;--primary:#38bdf8;--success:#22c55e;--danger:#ef4444}
        *{box-sizing:border-box;margin:0;padding:0}
        body{font-family:system-ui,sans-serif;background:var(--bg);color:var(--text);min-height:100vh;padding:16px}
        .container{max-width:500px;margin:0 auto}
        h1{font-size:1.5rem;margin-bottom:24px;display:flex;align-items:center;gap:12px}
        .card{background:var(--card);border-radius:16px;padding:24px;margin-bottom:16px;border:1px solid var(--border)}
        .form-group{margin-bottom:20px}
        label{display:block;font-size:0.875rem;color:var(--muted);margin-bottom:8px}
        input[type="text"]{width:100%%;padding:12px 16px;border:1px solid var(--border);border-radius:8px;background:var(--bg);color:var(--text);font-size:1rem}
        input[type="text"]:focus{outline:none;border-color:var(--primary)}
        .btn{display:block;width:100%%;padding:14px;border:none;border-radius:8px;font-size:1rem;font-weight:600;cursor:pointer;transition:opacity 0.2s}
        .btn-primary{background:var(--primary);color:#000}
        .btn-secondary{background:var(--border);color:var(--text);margin-top:12px}
        .btn:hover{opacity:0.9}
        .status{display:flex;align-items:center;gap:8px;padding:12px 16px;border-radius:8px;margin-bottom:20px;font-size:0.875rem}
        .status.success{background:rgba(34,197,94,0.15);color:var(--success)}
        .status.warning{background:rgba(245,158,11,0.15);color:#f59e0b}
        .hint{font-size:0.75rem;color:var(--muted);margin-top:8px}
        .back{display:inline-flex;align-items:center;gap:8px;color:var(--muted);text-decoration:none;margin-bottom:16px;font-size:0.875rem}
        .back:hover{color:var(--text)}
    </style>
</head>
<body>
<div class="container">
    <a href="/" class="back">← Back to Dashboard</a>
    <h1>⚙️ Settings</h1>
    
    <div class="card">
        <div class="status %s">
            <span>%s</span>
        </div>
        
        <form id="tokenForm">
            <div class="form-group">
                <label>Gateway Token</label>
                <input type="text" id="token" placeholder="Paste your device token here" value="">
                <div class="hint">Get token from IoT Dashboard → Add Device → Copy Token</div>
            </div>
            <button type="submit" class="btn btn-primary">💾 Save Token</button>
        </form>
        <a href="/" class="btn btn-secondary">Cancel</a>
    </div>
    
    <div class="card">
        <label>Current Configuration</label>
        <p style="font-size:0.875rem;color:var(--muted);margin-top:8px">
            Serial: %s<br>
            Backend: %s<br>
            Token: %s
        </p>
    </div>
</div>
<script>
document.getElementById('tokenForm').onsubmit = async (e) => {
    e.preventDefault();
    const token = document.getElementById('token').value.trim();
    if (!token) { alert('Please enter a token'); return; }
    if (token.length < 32) { alert('Token seems too short'); return; }
    
    try {
        const res = await fetch('/api/settings', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({token: token})
        });
        const data = await res.json();
        if (data.error) {
            alert('Error: ' + data.error);
        } else {
            alert('Token saved! Gateway will now send metrics to backend.');
            window.location.href = '/';
        }
    } catch (err) {
        alert('Failed to save: ' + err);
    }
};
</script>
</body>
</html>`,
		func() string {
			if hasToken {
				return "success"
			} else {
				return "warning"
			}
		}(),
		func() string {
			if hasToken {
				return "✅ Token configured"
			} else {
				return "⚠️ No token - metrics won't be sent to cloud"
			}
		}(),
		g.config.SerialPort,
		g.config.BackendURL,
		func() string {
			if hasToken {
				return tokenMasked
			} else {
				return "(not set)"
			}
		}())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func (g *Gateway) handleAPISettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "GET" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"has_token":   g.config.GatewayToken != "",
			"backend_url": g.config.BackendURL,
			"serial_port": g.config.SerialPort,
		})
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "Token is required"})
		return
	}

	// Save to file
	if err := saveTokenToFile(req.Token); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save: " + err.Error()})
		return
	}

	// Update runtime config
	g.config.GatewayToken = req.Token
	log.Printf("✅ Token updated via web UI")

	json.NewEncoder(w).Encode(map[string]string{"message": "Token saved successfully"})
}
