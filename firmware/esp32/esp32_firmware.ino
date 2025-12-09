// ===== ESP32 IoT Firmware v2.4.0 =====
// Mesh + Backend with time-sliced approach
// Mesh stops briefly for HTTP push, then restarts

#include <WiFi.h>
#include <WiFiClientSecure.h>
#include <HTTPClient.h>
#include <WebServer.h>
#include <EEPROM.h>
#include <painlessMesh.h>
#include <DHT.h>
#include <ArduinoJson.h>

// ---------------------------
// CONFIG
// ---------------------------
#define FIRMWARE_VERSION "2.4.0"
#define DHTPIN 15
#define DHTTYPE DHT22
#define MESH_PREFIX   "LabMesh"
#define MESH_PASSWORD "LabMesh123"
#define MESH_PORT     5555
#define EEPROM_SIZE   512
#define CONFIG_MAGIC  0xDEADBEEF

#define HTTP_TIMEOUT_MS 2000
#define MESH_RESTART_DELAY_MS 200

DHT dht(DHTPIN, DHTTYPE);
painlessMesh mesh;
WebServer server(80);

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
unsigned long lastPushTime = 0;

// Sensor data
float lastTemp = 0, lastHum = 0;
int lastRssi = 0;

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
// SENSOR READING
// ---------------------------
void readSensors() {
  if (cfg.dhtEnabled) {
    float t = dht.readTemperature();
    float h = dht.readHumidity();
    if (!isnan(t)) lastTemp = t;
    if (!isnan(h)) lastHum = h;
  }
  lastRssi = WiFi.RSSI();
}

// ---------------------------
// BUILD JSON PAYLOAD
// ---------------------------
String buildMetricsJSON() {
  StaticJsonDocument<1024> doc;
  
  JsonObject sys = doc.createNestedObject("system");
  sys["chip_id"] = getChipId();
  sys["mac"] = WiFi.macAddress();
  sys["platform"] = "ESP32";
  sys["firmware"] = FIRMWARE_VERSION;
  sys["free_heap"] = (long)ESP.getFreeHeap();
  
  JsonObject sensors = doc.createNestedObject("sensors");
  sensors["temperature"] = lastTemp;
  sensors["humidity"] = lastHum;
  
  JsonObject wifi = doc.createNestedObject("wifi");
  wifi["rssi"] = lastRssi;
  JsonArray scan = wifi.createNestedArray("scan");
  // Skip scan in JSON to keep it small
  
  JsonObject meshStatus = doc.createNestedObject("mesh_status");
  meshStatus["enabled"] = true;
  JsonArray nodes = meshStatus.createNestedArray("nodes");
  // Nodes info not available during HTTP push (mesh is stopped)
  
  doc["dht_enabled"] = cfg.dhtEnabled ? true : false;
  
  String output;
  serializeJson(doc, output);
  return output;
}

// ---------------------------
// MESH FUNCTIONS
// ---------------------------
void meshReceived(uint32_t from, String &msg) {
  Serial.printf("[MESH] From %u: %s\n", from, msg.c_str());
}

void meshNewConnection(uint32_t nodeId) {
  Serial.printf("[MESH] +Node: %u\n", nodeId);
}

void meshChangedConnections() {
  Serial.printf("[MESH] Topology changed, nodes: %d\n", (int)mesh.getNodeList().size() + 1);
}

void startMesh() {
  if (meshRunning) return;
  
  Serial.println("[MESH] Starting...");
  mesh.setDebugMsgTypes(ERROR | STARTUP);
  mesh.init(MESH_PREFIX, MESH_PASSWORD, MESH_PORT, WIFI_AP_STA, 6);
  mesh.onReceive(&meshReceived);
  mesh.onNewConnection(&meshNewConnection);
  mesh.onChangedConnections(&meshChangedConnections);
  
  // If we have external WiFi configured, set as root
  if (strlen(cfg.ssid) > 0) {
    mesh.stationManual(cfg.ssid, cfg.password);
    mesh.setRoot(true);
    mesh.setContainsRoot(true);
    isRootNode = true;
  }
  
  meshRunning = true;
  Serial.println("[MESH] Started");
}

void stopMesh() {
  if (!meshRunning) return;
  
  Serial.println("[MESH] Stopping...");
  mesh.stop();
  meshRunning = false;
  delay(100);
  Serial.println("[MESH] Stopped");
}

// ---------------------------
// HTTP PUSH (fire-and-forget, while mesh is stopped)
// ---------------------------
void pushToBackend() {
  if (strlen(cfg.backendUrl) < 5 || strlen(cfg.deviceToken) < 5) {
    Serial.println("[HTTP] No backend configured");
    return;
  }
  
  // Connect to WiFi (with shorter timeout)
  Serial.printf("[HTTP] Connecting to %s...\n", cfg.ssid);
  WiFi.mode(WIFI_STA);
  WiFi.begin(cfg.ssid, cfg.password);
  
  unsigned long startTime = millis();
  while (WiFi.status() != WL_CONNECTED && (millis() - startTime) < 5000) {
    delay(50);
  }
  
  if (WiFi.status() != WL_CONNECTED) {
    Serial.println("[HTTP] WiFi failed");
    WiFi.disconnect(true);
    WiFi.mode(WIFI_OFF);
    return;
  }
  Serial.printf("[HTTP] IP: %s\n", WiFi.localIP().toString().c_str());
  
  // Build request
  String url = String(cfg.backendUrl) + "/api/v1/metrics";
  String payload = buildMetricsJSON();
  
  Serial.println("[HTTP] Sending (fire-and-forget)...");
  
  HTTPClient http;
  WiFiClient* client;
  WiFiClientSecure* secClient = nullptr;
  
  if (String(cfg.backendUrl).startsWith("https")) {
    secClient = new WiFiClientSecure;
    secClient->setInsecure();
    secClient->setTimeout(2); // 2 second timeout
    http.begin(*secClient, url);
    client = secClient;
  } else {
    client = new WiFiClient;
    client->setTimeout(2);
    http.begin(*client, url);
  }
  
  http.addHeader("Content-Type", "application/json");
  http.addHeader("X-Device-Token", cfg.deviceToken);
  http.setTimeout(2000); // 2 second timeout - fire and forget
  
  // Fire and forget - just send, don't care about response
  int httpCode = http.POST(payload);
  Serial.printf("[HTTP] Sent, code: %d\n", httpCode);
  
  http.end();
  
  // Cleanup
  if (secClient) delete secClient;
  else delete client;
  
  WiFi.disconnect(true);
  WiFi.mode(WIFI_OFF);
  
  Serial.println("[HTTP] Done");
}

// ---------------------------
// MAIN PUSH CYCLE (time-sliced)
// ---------------------------
void doPushCycle() {
  if (!isRootNode) {
    Serial.println("[PUSH] Not root node, skipping");
    return;
  }
  
  Serial.println("\n========== PUSH CYCLE ==========");
  
  // 1. Stop mesh
  stopMesh();
  delay(MESH_RESTART_DELAY_MS);
  
  // 2. Push to backend
  pushToBackend();
  delay(MESH_RESTART_DELAY_MS);
  
  // 3. Restart mesh
  startMesh();
  
  Serial.println("========== CYCLE DONE ==========\n");
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
  <title>ESP32 IoT</title>
  <style>
    *{box-sizing:border-box;margin:0;padding:0}
    body{font-family:-apple-system,system-ui,sans-serif;background:#0d1117;color:#c9d1d9;min-height:100vh;padding:16px}
    .container{max-width:480px;margin:0 auto}
    h1{color:#58a6ff;text-align:center;font-size:24px;margin-bottom:4px}
    .sub{text-align:center;color:#8b949e;font-size:12px;margin-bottom:16px}
    .stats{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-bottom:16px}
    .stat{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px;text-align:center}
    .stat-val{font-size:28px;font-weight:700;color:#f0f6fc}
    .stat-lbl{font-size:11px;color:#8b949e;margin-top:4px}
    .badges{display:flex;gap:8px;justify-content:center;flex-wrap:wrap;margin-bottom:16px}
    .badge{padding:4px 12px;border-radius:16px;font-size:11px;font-weight:600}
    .b-green{background:#238636;color:#fff}
    .b-blue{background:#1f6feb;color:#fff}
    .b-orange{background:#9e6a03;color:#fff}
    .card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px;margin-bottom:12px}
    h3{color:#58a6ff;font-size:14px;margin-bottom:12px;display:flex;align-items:center;gap:6px}
    label{display:block;color:#8b949e;font-size:12px;margin-bottom:4px}
    input{width:100%;padding:10px;background:#0d1117;border:1px solid #30363d;border-radius:6px;color:#c9d1d9;font-size:14px;margin-bottom:12px}
    input:focus{outline:none;border-color:#58a6ff}
    .chk{display:flex;align-items:center;gap:8px;margin:12px 0}
    .chk input{width:auto;margin:0}
    .chk label{margin:0;color:#c9d1d9}
    button{width:100%;padding:12px;background:#238636;border:none;border-radius:6px;color:#fff;font-size:14px;font-weight:600;cursor:pointer}
    button:hover{background:#2ea043}
    .btn-blue{background:#1f6feb}
    .btn-blue:hover{background:#388bfd}
  </style>
</head>
<body>
<div class="container">
  <h1>🌐 ESP32 IoT</h1>
  <p class="sub">v%VERSION% • %CHIP_ID%</p>
  
  <div class="stats">
    <div class="stat"><div class="stat-val">%TEMP%°</div><div class="stat-lbl">Temperature</div></div>
    <div class="stat"><div class="stat-val">%HUM%%</div><div class="stat-lbl">Humidity</div></div>
  </div>
  
  <div class="badges">
    <span class="badge b-green">MESH %MESH%</span>
    %ROOT%
    <span class="badge b-blue">%IP%</span>
  </div>
  
  <div class="card">
    <form action="/save" method="POST">
      <h3>📶 WiFi (for backend)</h3>
      <label>SSID</label>
      <input name="ssid" value="%SSID%">
      <label>Password</label>
      <input name="password" type="password" value="%PASS%">
      
      <h3>☁️ Backend</h3>
      <label>URL</label>
      <input name="backendUrl" value="%BACKEND%" placeholder="https://chnu-iot.com">
      <label>Device Token</label>
      <input name="deviceToken" value="%TOKEN%">
      <label>Push Interval (ms, min 30000)</label>
      <input name="interval" type="number" value="%INTERVAL%" min="30000">
      
      <h3>⚙️ Device</h3>
      <label>Name</label>
      <input name="nodeName" value="%NODE%">
      <div class="chk">
        <input type="checkbox" name="dhtEnabled" id="dht" %DHT%>
        <label for="dht">DHT22 Sensor</label>
      </div>
      
      <button type="submit">💾 Save & Reboot</button>
    </form>
  </div>
  
  <div class="card">
    <button class="btn-blue" onclick="location.href='/metrics'">📊 View JSON</button>
  </div>
  
  <div class="card">
    <button class="btn-blue" onclick="location.href='/push'">🚀 Push Now</button>
  </div>
</div>
</body>
</html>
)rawliteral";

String processTemplate(String html) {
  html.replace("%VERSION%", FIRMWARE_VERSION);
  html.replace("%CHIP_ID%", getChipId());
  html.replace("%IP%", WiFi.localIP().toString());
  html.replace("%SSID%", cfg.ssid);
  html.replace("%PASS%", cfg.password);
  html.replace("%NODE%", cfg.nodeName);
  html.replace("%BACKEND%", cfg.backendUrl);
  html.replace("%TOKEN%", cfg.deviceToken);
  html.replace("%INTERVAL%", String(cfg.metricsIntervalMs));
  html.replace("%DHT%", cfg.dhtEnabled ? "checked" : "");
  html.replace("%TEMP%", String(lastTemp, 1));
  html.replace("%HUM%", String(lastHum, 1));
  html.replace("%MESH%", meshRunning ? "ON" : "OFF");
  html.replace("%ROOT%", isRootNode ? "<span class='badge b-orange'>ROOT</span>" : "");
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
    long val = atol(server.arg("interval").c_str());
    cfg.metricsIntervalMs = (uint32_t)(val < 30000 ? 30000 : val);
  }
  cfg.dhtEnabled = server.hasArg("dhtEnabled") ? 1 : 0;
  saveConfig();
  
  server.send(200, "text/html", "<html><body style='background:#0d1117;color:#c9d1d9;display:flex;justify-content:center;align-items:center;height:100vh;font-family:system-ui'><h1>✅ Rebooting...</h1></body></html>");
  delay(1000);
  ESP.restart();
}

void handleMetrics() {
  server.send(200, "application/json", buildMetricsJSON());
}

void handlePush() {
  server.send(200, "text/html", "<html><body style='background:#0d1117;color:#c9d1d9;display:flex;justify-content:center;align-items:center;height:100vh;font-family:system-ui'><h1>🚀 Pushing...</h1></body></html>");
  delay(500);
  doPushCycle();
}

void initWebServer() {
  server.on("/", handleRoot);
  server.on("/save", HTTP_POST, handleSave);
  server.on("/metrics", handleMetrics);
  server.on("/push", handlePush);
  server.begin();
  Serial.println("[WEB] Server started");
}

// ---------------------------
// SETUP
// ---------------------------
void setup() {
  Serial.begin(115200);
  delay(1000);
  Serial.printf("\n\n=== ESP32 IoT v%s ===\n", FIRMWARE_VERSION);
  Serial.println("Time-sliced Mesh + Backend");
  
  loadConfig();
  Serial.printf("[CFG] SSID: %s\n", cfg.ssid);
  Serial.printf("[CFG] Backend: %s\n", cfg.backendUrl);
  Serial.printf("[CFG] Interval: %lu ms\n", (unsigned long)cfg.metricsIntervalMs);
  
  if (cfg.dhtEnabled) {
    dht.begin();
    Serial.println("[DHT] Initialized");
  }
  
  // Determine if this is root node
  isRootNode = (strlen(cfg.ssid) > 0);
  Serial.printf("[NODE] Root: %s\n", isRootNode ? "YES" : "NO");
  
  // Start mesh
  startMesh();
  
  // Start web server
  initWebServer();
  
  lastPushTime = millis();
  Serial.println("[READY]");
}

// ---------------------------
// LOOP
// ---------------------------
void loop() {
  // Update mesh
  if (meshRunning) {
    mesh.update();
  }
  
  // Handle web requests
  server.handleClient();
  
  // Read sensors periodically
  static unsigned long lastSensorRead = 0;
  if (millis() - lastSensorRead > 5000) {
    readSensors();
    lastSensorRead = millis();
  }
  
  // Push cycle for root node
  if (isRootNode && strlen(cfg.backendUrl) > 5) {
    if (millis() - lastPushTime >= cfg.metricsIntervalMs) {
      doPushCycle();
      lastPushTime = millis();
    }
  }
  
  yield();
}
