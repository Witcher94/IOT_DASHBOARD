// ===== ESP32 IoT Firmware =====
// Mesh network + DHT22 + Backend metrics (Bridge Mode)
// Uses painlessMesh task scheduler for safe HTTP calls

#include <WiFi.h>
#include <HTTPClient.h>
#include <WebServer.h>
#include <EEPROM.h>
#include <painlessMesh.h>
#include <DHT.h>
#include <ArduinoJson.h>

// ---------------------------
// CONFIG
// ---------------------------
#define FIRMWARE_VERSION "2.1.0"
#define DHTPIN 15
#define DHTTYPE DHT22
#define MESH_PREFIX   "LabMesh"
#define MESH_PASSWORD "LabMesh123"
#define MESH_PORT     5555
#define EEPROM_SIZE 512
#define CONFIG_MAGIC 0xDEADBEEF

DHT dht(DHTPIN, DHTTYPE);
painlessMesh mesh;
WebServer server(80);
Scheduler userScheduler;

// ---------------------------
// EEPROM STRUCTURE
// ---------------------------
struct ConfigData {
  uint32_t magic;
  char ssid[32];
  char password[64];
  char nodeName[32];
  char backendUrl[128];
  char deviceToken[64];
  uint32_t metricsIntervalMs;
  uint8_t dhtEnabled;
};

ConfigData cfg;
bool meshRunning = false;
bool isRootNode = false;

// Forward declarations
void sendMetricsTask();
void readSensorsTask();

// Tasks using mesh scheduler
Task taskSendMetrics(TASK_SECOND * 30, TASK_FOREVER, &sendMetricsTask, &userScheduler, false);
Task taskReadSensors(TASK_SECOND * 5, TASK_FOREVER, &readSensorsTask, &userScheduler, true);

// ---------------------------
// HELPERS
// ---------------------------
String getChipId() {
  uint64_t chipid = ESP.getEfuseMac();
  char id[17];
  snprintf(id, sizeof(id), "%04X%08X", (uint16_t)(chipid >> 32), (uint32_t)chipid);
  return String(id);
}

// ---------------------------
// CONFIG MANAGEMENT
// ---------------------------
void loadConfig() {
  EEPROM.begin(EEPROM_SIZE);
  EEPROM.get(0, cfg);
  
  if (cfg.magic != CONFIG_MAGIC) {
    memset(&cfg, 0, sizeof(cfg));
    cfg.magic = CONFIG_MAGIC;
    strcpy(cfg.nodeName, "ESP32-Node");
    cfg.metricsIntervalMs = 30000;
    cfg.dhtEnabled = 1;
    saveConfig();
  }
}

void saveConfig() {
  EEPROM.put(0, cfg);
  EEPROM.commit();
}

// ---------------------------
// SENSOR DATA
// ---------------------------
float lastTemp = 0, lastHum = 0;
int lastRssi = 0;

void readSensorsTask() {
  if (cfg.dhtEnabled) {
    float t = dht.readTemperature();
    float h = dht.readHumidity();
    if (!isnan(t)) lastTemp = t;
    if (!isnan(h)) lastHum = h;
  }
  lastRssi = WiFi.RSSI();
}

// ---------------------------
// BUILD JSON
// ---------------------------
String buildMetricsJSON() {
  StaticJsonDocument<1024> doc;
  
  JsonObject sys = doc.createNestedObject("system");
  sys["chip_id"] = getChipId();
  sys["mac"] = WiFi.macAddress();
  sys["platform"] = "ESP32";
  sys["firmware"] = FIRMWARE_VERSION;
  sys["free_heap"] = ESP.getFreeHeap();
  
  JsonObject sensors = doc.createNestedObject("sensors");
  sensors["temperature"] = lastTemp;
  sensors["humidity"] = lastHum;
  
  JsonObject wifi = doc.createNestedObject("wifi");
  wifi["rssi"] = lastRssi;
  
  // WiFi scan
  JsonArray scan = wifi.createNestedArray("scan");
  int n = WiFi.scanComplete();
  if (n > 0) {
    for (int i = 0; i < min(n, 5); i++) {
      JsonObject net = scan.createNestedObject();
      net["ssid"] = WiFi.SSID(i);
      net["rssi"] = WiFi.RSSI(i);
      net["bssid"] = WiFi.BSSIDstr(i);
      net["channel"] = WiFi.channel(i);
      net["enc"] = WiFi.encryptionType(i);
    }
    WiFi.scanDelete();
  }
  if (n != WIFI_SCAN_RUNNING) {
    WiFi.scanNetworks(true); // Start async scan
  }
  
  JsonObject meshObj = doc.createNestedObject("mesh_status");
  meshObj["enabled"] = meshRunning;
  if (meshRunning) {
    meshObj["node_id"] = mesh.getNodeId();
    meshObj["is_root"] = isRootNode;
    auto nodes = mesh.getNodeList();
    JsonArray nodesArr = meshObj.createNestedArray("nodes");
    for (auto &n : nodes) {
      JsonObject node = nodesArr.createNestedObject();
      node["node_id"] = n;
    }
  }
  
  doc["dht_enabled"] = cfg.dhtEnabled ? true : false;
  
  String output;
  serializeJson(doc, output);
  return output;
}

// ---------------------------
// BACKEND COMMUNICATION (via mesh scheduler - safe!)
// ---------------------------
void sendMetricsTask() {
  if (strlen(cfg.backendUrl) < 5 || strlen(cfg.deviceToken) < 5) return;
  if (!isRootNode && meshRunning) {
    // Only root node sends to backend in mesh mode
    Serial.println("[METRICS] Not root node, skipping backend push");
    return;
  }
  
  Serial.println("[METRICS] Sending to backend...");
  
  // Use WiFiClient for HTTP (not HTTPS) or WiFiClientSecure for HTTPS
  HTTPClient http;
  String url = String(cfg.backendUrl) + "/api/v1/metrics";
  
  // For HTTPS, we need to handle it differently
  if (String(cfg.backendUrl).startsWith("https")) {
    // Use insecure mode for simplicity
    WiFiClientSecure *client = new WiFiClientSecure;
    client->setInsecure();
    http.begin(*client, url);
  } else {
    http.begin(url);
  }
  
  http.addHeader("Content-Type", "application/json");
  http.addHeader("X-Device-Token", cfg.deviceToken);
  http.setTimeout(10000);
  
  String payload = buildMetricsJSON();
  int code = http.POST(payload);
  
  if (code > 0) {
    Serial.printf("[METRICS] Response: %d\n", code);
  } else {
    Serial.printf("[METRICS] Error: %s\n", http.errorToString(code).c_str());
  }
  
  http.end();
}

// ---------------------------
// MESH CALLBACKS
// ---------------------------
void meshReceived(uint32_t from, String &msg) {
  Serial.printf("[MESH] From %u: %s\n", from, msg.c_str());
  
  // If we're root and received data from another node, forward to backend
  if (isRootNode) {
    // Could aggregate and forward metrics from child nodes here
  }
}

void meshNewConnection(uint32_t nodeId) {
  Serial.printf("[MESH] New connection: %u\n", nodeId);
}

void meshChangedConnections() {
  Serial.println("[MESH] Topology changed");
  
  // Check if we're the root (connected to external WiFi)
  if (WiFi.status() == WL_CONNECTED && WiFi.localIP()[0] != 0) {
    isRootNode = true;
    Serial.println("[MESH] This node is ROOT (gateway)");
  }
}

void meshNodeTimeAdjusted(int32_t offset) {
  // Time sync callback
}

void initMesh() {
  // Use WIFI_AP_STA for bridge mode
  mesh.setDebugMsgTypes(ERROR | STARTUP | CONNECTION);
  
  // Init mesh with external scheduler
  mesh.init(MESH_PREFIX, MESH_PASSWORD, &userScheduler, MESH_PORT, WIFI_AP_STA, 6);
  
  // Set callbacks
  mesh.onReceive(&meshReceived);
  mesh.onNewConnection(&meshNewConnection);
  mesh.onChangedConnections(&meshChangedConnections);
  mesh.onNodeTimeAdjusted(&meshNodeTimeAdjusted);
  
  // Connect to external WiFi (bridge mode)
  if (strlen(cfg.ssid) > 0) {
    mesh.stationManual(cfg.ssid, cfg.password);
    Serial.printf("[MESH] Bridge mode: connecting to %s\n", cfg.ssid);
  }
  
  // Set this node as root if it has external WiFi config
  if (strlen(cfg.ssid) > 0) {
    mesh.setRoot(true);
    mesh.setContainsRoot(true);
  }
  
  meshRunning = true;
  Serial.println("[MESH] Initialized in bridge mode");
}

// ---------------------------
// WEB UI
// ---------------------------
const char HTML_PAGE[] PROGMEM = R"rawliteral(
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>ESP32 Config</title>
  <style>
    body{font-family:Arial;background:#1a1a2e;color:#eee;padding:20px;max-width:600px;margin:0 auto}
    .card{background:#16213e;padding:20px;border-radius:10px;margin:10px 0}
    input{width:100%;padding:10px;margin:5px 0;border-radius:5px;border:none;box-sizing:border-box;background:#0f3460;color:#eee}
    button{background:#e94560;color:white;padding:12px 24px;border:none;border-radius:5px;cursor:pointer;margin:5px 0;width:100%}
    button:hover{background:#ff6b6b}
    h1{color:#e94560;text-align:center}
    h3{color:#0ea5e9;margin-top:20px}
    label{display:block;margin-top:10px;color:#aaa;font-size:14px}
    .status{padding:15px;background:#0f3460;border-radius:8px;margin:15px 0;font-size:14px;line-height:1.8}
    .status b{color:#e94560}
    .badge{display:inline-block;padding:3px 8px;border-radius:4px;font-size:12px;margin-left:5px}
    .online{background:#22c55e33;color:#22c55e}
    .offline{background:#ef444433;color:#ef4444}
    .checkbox-label{display:flex;align-items:center;gap:10px;margin:10px 0}
    .checkbox-label input{width:auto}
  </style>
</head>
<body>
  <h1>🌐 ESP32 IoT</h1>
  
  <div class="status">
    <b>Chip ID:</b> %CHIP_ID%<br>
    <b>IP:</b> %IP%<br>
    <b>Mesh:</b> %MESH_STATUS%<br>
    <b>Temperature:</b> %TEMP%°C | <b>Humidity:</b> %HUM%%<br>
    <b>Free Heap:</b> %HEAP% bytes
  </div>
  
  <div class="card">
    <form action="/save" method="POST">
      <h3>📶 WiFi (Bridge)</h3>
      <label>SSID (зовнішня мережа)</label>
      <input name="ssid" value="%SSID%" placeholder="Your WiFi SSID">
      <label>Password</label>
      <input name="password" type="password" value="%PASS%">
      
      <h3>🔧 Device</h3>
      <label>Node Name</label>
      <input name="nodeName" value="%NODE%">
      
      <h3>☁️ Backend</h3>
      <label>URL (https://your-domain.com)</label>
      <input name="backendUrl" value="%BACKEND%" placeholder="https://chnu-iot.com">
      <label>Device Token</label>
      <input name="deviceToken" value="%TOKEN%" placeholder="your-device-token">
      <label>Push Interval (ms)</label>
      <input name="interval" type="number" value="%INTERVAL%" min="5000">
      
      <h3>⚙️ Options</h3>
      <div class="checkbox-label">
        <input type="checkbox" name="dhtEnabled" id="dht" %DHT_CHK%>
        <label for="dht" style="margin:0">Enable DHT22 Sensor</label>
      </div>
      
      <br>
      <button type="submit">💾 Save & Reboot</button>
    </form>
  </div>
  
  <div class="card">
    <h3>📊 Live Data</h3>
    <button onclick="location.href='/metrics'">View JSON Metrics</button>
    <button onclick="location.href='/scan'">Scan WiFi Networks</button>
  </div>
</body>
</html>
)rawliteral";

String processTemplate(String html) {
  html.replace("%CHIP_ID%", getChipId());
  html.replace("%IP%", WiFi.localIP().toString());
  html.replace("%SSID%", cfg.ssid);
  html.replace("%PASS%", cfg.password);
  html.replace("%NODE%", cfg.nodeName);
  html.replace("%BACKEND%", cfg.backendUrl);
  html.replace("%TOKEN%", cfg.deviceToken);
  html.replace("%INTERVAL%", String(cfg.metricsIntervalMs));
  html.replace("%DHT_CHK%", cfg.dhtEnabled ? "checked" : "");
  html.replace("%TEMP%", String(lastTemp, 1));
  html.replace("%HUM%", String(lastHum, 1));
  html.replace("%HEAP%", String(ESP.getFreeHeap()));
  
  String meshStatus = meshRunning ? 
    (isRootNode ? "<span class='badge online'>ROOT</span>" : "<span class='badge online'>NODE</span>") :
    "<span class='badge offline'>OFF</span>";
  meshStatus += " Nodes: " + String(meshRunning ? mesh.getNodeList().size() + 1 : 0);
  html.replace("%MESH_STATUS%", meshStatus);
  
  return html;
}

void handleRoot() {
  server.send(200, "text/html", processTemplate(FPSTR(HTML_PAGE)));
}

void handleSave() {
  if (server.hasArg("ssid")) strncpy(cfg.ssid, server.arg("ssid").c_str(), sizeof(cfg.ssid)-1);
  if (server.hasArg("password")) strncpy(cfg.password, server.arg("password").c_str(), sizeof(cfg.password)-1);
  if (server.hasArg("nodeName")) strncpy(cfg.nodeName, server.arg("nodeName").c_str(), sizeof(cfg.nodeName)-1);
  if (server.hasArg("backendUrl")) strncpy(cfg.backendUrl, server.arg("backendUrl").c_str(), sizeof(cfg.backendUrl)-1);
  if (server.hasArg("deviceToken")) strncpy(cfg.deviceToken, server.arg("deviceToken").c_str(), sizeof(cfg.deviceToken)-1);
  if (server.hasArg("interval")) {
    uint32_t interval = server.arg("interval").toInt();
    cfg.metricsIntervalMs = max(5000UL, (unsigned long)interval);
  }
  cfg.dhtEnabled = server.hasArg("dhtEnabled") ? 1 : 0;
  
  saveConfig();
  server.send(200, "text/html", "<html><body style='background:#1a1a2e;color:#eee;text-align:center;padding:50px'><h1>✅ Saved!</h1><p>Rebooting...</p></body></html>");
  delay(1000);
  ESP.restart();
}

void handleMetrics() {
  server.send(200, "application/json", buildMetricsJSON());
}

void handleScan() {
  String json = "[";
  int n = WiFi.scanComplete();
  if (n == WIFI_SCAN_FAILED) {
    WiFi.scanNetworks(true);
    server.send(200, "application/json", "{\"status\":\"scanning\"}");
    return;
  }
  if (n > 0) {
    for (int i = 0; i < n; i++) {
      if (i > 0) json += ",";
      json += "{\"ssid\":\"" + WiFi.SSID(i) + "\",\"rssi\":" + String(WiFi.RSSI(i)) + "}";
    }
    WiFi.scanDelete();
    WiFi.scanNetworks(true);
  }
  json += "]";
  server.send(200, "application/json", json);
}

void initWebServer() {
  server.on("/", handleRoot);
  server.on("/save", HTTP_POST, handleSave);
  server.on("/metrics", handleMetrics);
  server.on("/scan", handleScan);
  server.begin();
  Serial.println("[WEB] Server started");
}

// ---------------------------
// SETUP & LOOP
// ---------------------------
void setup() {
  Serial.begin(115200);
  delay(1000);
  Serial.println("\n=== ESP32 IoT v" FIRMWARE_VERSION " ===");
  Serial.println("Bridge Mode with Mesh Network");
  
  loadConfig();
  if (cfg.dhtEnabled) dht.begin();
  
  // Initialize mesh (handles WiFi internally)
  initMesh();
  
  // Start web server
  initWebServer();
  
  // Configure and enable metrics task
  if (cfg.metricsIntervalMs > 0) {
    taskSendMetrics.setInterval(cfg.metricsIntervalMs);
    taskSendMetrics.enable();
  }
  
  // Start initial WiFi scan
  WiFi.scanNetworks(true);
  
  Serial.println("[READY] Setup complete");
}

void loop() {
  // Mesh handles WiFi and calls scheduler internally
  mesh.update();
  
  // Handle web requests
  server.handleClient();
  
  // Small yield to prevent watchdog
  yield();
}
