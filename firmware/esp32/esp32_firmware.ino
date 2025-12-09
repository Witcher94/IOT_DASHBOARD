// ===== ESP32 IoT Firmware v2.5.0 =====
// Two modes: BACKEND (stable) or MESH (local network)
// Backend mode - reliable HTTP to cloud
// Mesh mode - local device network

#include <WiFi.h>
#include <WiFiClientSecure.h>
#include <HTTPClient.h>
#include <WebServer.h>
#include <EEPROM.h>
#include <DHT.h>
#include <ArduinoJson.h>

// Mesh is optional - comment out to save memory
#define ENABLE_MESH
#ifdef ENABLE_MESH
#include <painlessMesh.h>
painlessMesh mesh;
#endif

// ---------------------------
// CONFIG
// ---------------------------
#define FIRMWARE_VERSION "2.5.0"
#define DHTPIN 15
#define DHTTYPE DHT22
#define MESH_PREFIX   "LabMesh"
#define MESH_PASSWORD "LabMesh123"
#define MESH_PORT     5555
#define EEPROM_SIZE   512
#define CONFIG_MAGIC  0xDEADBEEF

DHT dht(DHTPIN, DHTTYPE);
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
  uint8_t mode; // 0 = Backend, 1 = Mesh
};

ConfigData cfg;
bool meshRunning = false;
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
    cfg.metricsIntervalMs = 15000;
    cfg.dhtEnabled = 1;
    cfg.mode = 0; // Backend mode by default
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
  
  JsonObject meshStatus = doc.createNestedObject("mesh_status");
  meshStatus["enabled"] = meshRunning;
#ifdef ENABLE_MESH
  if (meshRunning) {
    auto nodes = mesh.getNodeList();
    JsonArray nodesArr = meshStatus.createNestedArray("nodes");
    for (auto &nodeId : nodes) {
      JsonObject node = nodesArr.createNestedObject();
      node["node_id"] = nodeId;
    }
  }
#endif
  
  doc["dht_enabled"] = cfg.dhtEnabled ? true : false;
  
  String output;
  serializeJson(doc, output);
  return output;
}

// ---------------------------
// BACKEND MODE FUNCTIONS
// ---------------------------
void connectWiFi() {
  if (strlen(cfg.ssid) == 0) {
    Serial.println("[WIFI] No SSID, starting AP");
    WiFi.mode(WIFI_AP);
    WiFi.softAP("ESP32-Setup", "12345678");
    Serial.printf("[WIFI] AP IP: %s\n", WiFi.softAPIP().toString().c_str());
    return;
  }
  
  Serial.printf("[WIFI] Connecting to %s", cfg.ssid);
  WiFi.mode(WIFI_STA);
  WiFi.begin(cfg.ssid, cfg.password);
  
  int attempts = 0;
  while (WiFi.status() != WL_CONNECTED && attempts < 40) {
    delay(250);
    Serial.print(".");
    attempts++;
  }
  
  if (WiFi.status() == WL_CONNECTED) {
    Serial.printf("\n[WIFI] Connected! IP: %s\n", WiFi.localIP().toString().c_str());
  } else {
    Serial.println("\n[WIFI] Failed, starting AP");
    WiFi.mode(WIFI_AP);
    WiFi.softAP("ESP32-Setup", "12345678");
  }
}

void pushToBackend() {
  if (strlen(cfg.backendUrl) < 5 || strlen(cfg.deviceToken) < 5) return;
  if (WiFi.status() != WL_CONNECTED) return;
  
  String url = String(cfg.backendUrl) + "/api/v1/metrics";
  String payload = buildMetricsJSON();
  
  Serial.printf("[HTTP] POST %s\n", url.c_str());
  
  HTTPClient http;
  
  if (String(cfg.backendUrl).startsWith("https")) {
    WiFiClientSecure *client = new WiFiClientSecure;
    client->setInsecure();
    http.begin(*client, url);
  } else {
    WiFiClient client;
    http.begin(client, url);
  }
  
  http.addHeader("Content-Type", "application/json");
  http.addHeader("X-Device-Token", cfg.deviceToken);
  http.setTimeout(5000);
  
  int httpCode = http.POST(payload);
  Serial.printf("[HTTP] Response: %d\n", httpCode);
  
  http.end();
}

// ---------------------------
// MESH MODE FUNCTIONS
// ---------------------------
#ifdef ENABLE_MESH
void meshReceived(uint32_t from, String &msg) {
  Serial.printf("[MESH] From %u: %s\n", from, msg.c_str());
}

void meshNewConnection(uint32_t nodeId) {
  Serial.printf("[MESH] +Node: %u\n", nodeId);
}

void meshChangedConnections() {
  Serial.printf("[MESH] Nodes: %d\n", (int)mesh.getNodeList().size() + 1);
}

void startMesh() {
  Serial.println("[MESH] Starting...");
  mesh.setDebugMsgTypes(ERROR | STARTUP);
  mesh.init(MESH_PREFIX, MESH_PASSWORD, MESH_PORT);
  mesh.onReceive(&meshReceived);
  mesh.onNewConnection(&meshNewConnection);
  mesh.onChangedConnections(&meshChangedConnections);
  meshRunning = true;
  Serial.println("[MESH] Started");
}

void broadcastMetrics() {
  String msg = buildMetricsJSON();
  mesh.sendBroadcast(msg);
  Serial.println("[MESH] Broadcast sent");
}
#endif

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
    .b-purple{background:#8957e5;color:#fff}
    .card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px;margin-bottom:12px}
    h3{color:#58a6ff;font-size:14px;margin-bottom:12px}
    label{display:block;color:#8b949e;font-size:12px;margin-bottom:4px}
    input,select{width:100%;padding:10px;background:#0d1117;border:1px solid #30363d;border-radius:6px;color:#c9d1d9;font-size:14px;margin-bottom:12px}
    input:focus,select:focus{outline:none;border-color:#58a6ff}
    .chk{display:flex;align-items:center;gap:8px;margin:12px 0}
    .chk input{width:auto;margin:0}
    .chk label{margin:0;color:#c9d1d9}
    button{width:100%;padding:12px;background:#238636;border:none;border-radius:6px;color:#fff;font-size:14px;font-weight:600;cursor:pointer}
    button:hover{background:#2ea043}
    .btn-blue{background:#1f6feb}
    .mode-select{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-bottom:16px}
    .mode-btn{padding:16px;border-radius:8px;text-align:center;cursor:pointer;border:2px solid #30363d;background:#161b22}
    .mode-btn.active{border-color:#58a6ff;background:#1f6feb20}
    .mode-btn h4{margin:0 0 4px;font-size:14px}
    .mode-btn p{margin:0;font-size:11px;color:#8b949e}
    .backend-cfg,.mesh-cfg{display:none}
    .backend-cfg.show,.mesh-cfg.show{display:block}
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
    <span class="badge %MODE_BADGE%">%MODE_NAME%</span>
    <span class="badge b-blue">%IP%</span>
  </div>
  
  <div class="card">
    <form action="/save" method="POST">
      <h3>⚡ Mode</h3>
      <div class="mode-select">
        <div class="mode-btn %BACKEND_ACTIVE%" onclick="setMode(0)">
          <h4>☁️ Backend</h4>
          <p>Cloud monitoring</p>
        </div>
        <div class="mode-btn %MESH_ACTIVE%" onclick="setMode(1)">
          <h4>🕸️ Mesh</h4>
          <p>Local network</p>
        </div>
      </div>
      <input type="hidden" name="mode" id="modeInput" value="%MODE%">
      
      <div class="backend-cfg %BACKEND_SHOW%">
        <h3>📶 WiFi</h3>
        <label>SSID</label>
        <input name="ssid" value="%SSID%">
        <label>Password</label>
        <input name="password" type="password" value="%PASS%">
        
        <h3>☁️ Backend</h3>
        <label>URL</label>
        <input name="backendUrl" value="%BACKEND%" placeholder="https://chnu-iot.com">
        <label>Device Token</label>
        <input name="deviceToken" value="%TOKEN%">
        <label>Interval (sec)</label>
        <input name="interval" type="number" value="%INTERVAL%" min="15" max="300">
      </div>
      
      <h3>⚙️ Settings</h3>
      <label>Device Name</label>
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
</div>
<script>
function setMode(m) {
  document.getElementById('modeInput').value = m;
  document.querySelectorAll('.mode-btn').forEach((el,i) => el.classList.toggle('active', i===m));
  document.querySelector('.backend-cfg').classList.toggle('show', m===0);
}
</script>
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
  html.replace("%INTERVAL%", String(cfg.metricsIntervalMs / 1000));
  html.replace("%DHT%", cfg.dhtEnabled ? "checked" : "");
  html.replace("%TEMP%", String(lastTemp, 1));
  html.replace("%HUM%", String(lastHum, 1));
  html.replace("%MODE%", String(cfg.mode));
  html.replace("%MODE_NAME%", cfg.mode == 0 ? "BACKEND" : "MESH");
  html.replace("%MODE_BADGE%", cfg.mode == 0 ? "b-green" : "b-purple");
  html.replace("%BACKEND_ACTIVE%", cfg.mode == 0 ? "active" : "");
  html.replace("%MESH_ACTIVE%", cfg.mode == 1 ? "active" : "");
  html.replace("%BACKEND_SHOW%", cfg.mode == 0 ? "show" : "");
  return html;
}

void handleRoot() {
  server.send(200, "text/html", processTemplate(FPSTR(HTML_PAGE)));
}

void handleSave() {
  if (server.hasArg("mode")) cfg.mode = server.arg("mode").toInt();
  if (server.hasArg("ssid")) strncpy(cfg.ssid, server.arg("ssid").c_str(), sizeof(cfg.ssid)-1);
  if (server.hasArg("password")) strncpy(cfg.password, server.arg("password").c_str(), sizeof(cfg.password)-1);
  if (server.hasArg("nodeName")) strncpy(cfg.nodeName, server.arg("nodeName").c_str(), sizeof(cfg.nodeName)-1);
  if (server.hasArg("backendUrl")) strncpy(cfg.backendUrl, server.arg("backendUrl").c_str(), sizeof(cfg.backendUrl)-1);
  if (server.hasArg("deviceToken")) strncpy(cfg.deviceToken, server.arg("deviceToken").c_str(), sizeof(cfg.deviceToken)-1);
  if (server.hasArg("interval")) {
    long val = atol(server.arg("interval").c_str());
    cfg.metricsIntervalMs = (uint32_t)((val < 15 ? 15 : (val > 300 ? 300 : val)) * 1000);
  }
  cfg.dhtEnabled = server.hasArg("dhtEnabled") ? 1 : 0;
  saveConfig();
  
  server.send(200, "text/html", "<html><body style='background:#0d1117;color:#c9d1d9;display:flex;justify-content:center;align-items:center;height:100vh'><h1>✅ Rebooting...</h1></body></html>");
  delay(1000);
  ESP.restart();
}

void handleMetrics() {
  server.send(200, "application/json", buildMetricsJSON());
}

void initWebServer() {
  server.on("/", handleRoot);
  server.on("/save", HTTP_POST, handleSave);
  server.on("/metrics", handleMetrics);
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
  
  loadConfig();
  Serial.printf("[CFG] Mode: %s\n", cfg.mode == 0 ? "BACKEND" : "MESH");
  
  if (cfg.dhtEnabled) {
    dht.begin();
  }
  
  if (cfg.mode == 0) {
    // BACKEND MODE - connects to WiFi, sends to cloud
    Serial.println("[MODE] Backend - Cloud Monitoring");
    connectWiFi();
  } else {
    // MESH MODE - creates mesh network
    Serial.println("[MODE] Mesh - Local Network");
#ifdef ENABLE_MESH
    startMesh();
#else
    Serial.println("[MESH] Not compiled in, falling back to AP");
    WiFi.mode(WIFI_AP);
    WiFi.softAP("ESP32-Mesh", "12345678");
#endif
  }
  
  initWebServer();
  lastPushTime = millis();
  Serial.println("[READY]");
}

// ---------------------------
// LOOP
// ---------------------------
void loop() {
  server.handleClient();
  
  // Read sensors
  static unsigned long lastSensorRead = 0;
  if (millis() - lastSensorRead > 5000) {
    readSensors();
    lastSensorRead = millis();
  }
  
  if (cfg.mode == 0) {
    // BACKEND MODE - push metrics to cloud
    if (millis() - lastPushTime >= cfg.metricsIntervalMs) {
      pushToBackend();
      lastPushTime = millis();
    }
  } else {
    // MESH MODE
#ifdef ENABLE_MESH
    mesh.update();
    
    // Broadcast metrics to mesh
    if (millis() - lastPushTime >= cfg.metricsIntervalMs) {
      broadcastMetrics();
      lastPushTime = millis();
    }
#endif
  }
  
  yield();
}
