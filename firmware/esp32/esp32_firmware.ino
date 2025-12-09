// ===== ESP32 IoT Firmware v2.2.0 =====
// Two modes: MESH mode OR BACKEND mode (not both)
// - MESH mode: participates in mesh network
// - BACKEND mode: sends metrics to cloud backend

#include <WiFi.h>
#include <WiFiClientSecure.h>
#include <HTTPClient.h>
#include <WebServer.h>
#include <EEPROM.h>
#include <DHT.h>
#include <ArduinoJson.h>

// Only include mesh if needed
#ifndef DISABLE_MESH
#include <painlessMesh.h>
#endif

// ---------------------------
// CONFIG
// ---------------------------
#define FIRMWARE_VERSION "2.2.0"
#define DHTPIN 15
#define DHTTYPE DHT22
#define MESH_PREFIX   "LabMesh"
#define MESH_PASSWORD "LabMesh123"
#define MESH_PORT     5555
#define EEPROM_SIZE 512
#define CONFIG_MAGIC 0xDEADBEEF

DHT dht(DHTPIN, DHTTYPE);
WebServer server(80);

#ifndef DISABLE_MESH
painlessMesh mesh;
bool meshRunning = false;
#endif

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
  uint8_t meshMode;  // 0 = backend mode, 1 = mesh mode
};

ConfigData cfg;
unsigned long lastPush = 0;

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
    cfg.meshMode = 0; // Default to backend mode
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
  
  // WiFi scan results
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
    WiFi.scanNetworks(true);
  }
  
  JsonObject meshObj = doc.createNestedObject("mesh_status");
#ifndef DISABLE_MESH
  meshObj["enabled"] = meshRunning;
  if (meshRunning) {
    auto nodes = mesh.getNodeList();
    JsonArray nodesArr = meshObj.createNestedArray("nodes");
    for (auto &nodeId : nodes) {
      JsonObject node = nodesArr.createNestedObject();
      node["node_id"] = nodeId;
    }
  }
#else
  meshObj["enabled"] = false;
#endif
  
  doc["dht_enabled"] = cfg.dhtEnabled ? true : false;
  
  String output;
  serializeJson(doc, output);
  return output;
}

// ---------------------------
// BACKEND MODE: HTTP Push
// ---------------------------
void pushMetricsToBackend() {
  if (strlen(cfg.backendUrl) < 5 || strlen(cfg.deviceToken) < 5) return;
  if (WiFi.status() != WL_CONNECTED) return;
  
  Serial.println("[BACKEND] Pushing metrics...");
  
  HTTPClient http;
  String url = String(cfg.backendUrl) + "/api/v1/metrics";
  
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
  http.setTimeout(15000);
  
  String payload = buildMetricsJSON();
  int code = http.POST(payload);
  
  if (code > 0) {
    Serial.printf("[BACKEND] Response: %d\n", code);
  } else {
    Serial.printf("[BACKEND] Error: %s\n", http.errorToString(code).c_str());
  }
  
  http.end();
}

// ---------------------------
// MESH MODE
// ---------------------------
#ifndef DISABLE_MESH
void meshReceived(uint32_t from, String &msg) {
  Serial.printf("[MESH] From %u: %s\n", from, msg.c_str());
}

void meshNewConnection(uint32_t nodeId) {
  Serial.printf("[MESH] New node: %u\n", nodeId);
}

void meshChangedConnections() {
  Serial.printf("[MESH] Topology changed. Nodes: %d\n", mesh.getNodeList().size());
}

void initMesh() {
  mesh.setDebugMsgTypes(ERROR | STARTUP);
  mesh.init(MESH_PREFIX, MESH_PASSWORD, MESH_PORT);
  mesh.onReceive(&meshReceived);
  mesh.onNewConnection(&meshNewConnection);
  mesh.onChangedConnections(&meshChangedConnections);
  meshRunning = true;
  Serial.println("[MESH] Started");
}

void broadcastToMesh() {
  if (!meshRunning) return;
  String msg = buildMetricsJSON();
  mesh.sendBroadcast(msg);
  Serial.println("[MESH] Broadcast sent");
}
#endif

// ---------------------------
// WIFI (for Backend mode)
// ---------------------------
void connectWiFi() {
  if (strlen(cfg.ssid) == 0) {
    WiFi.mode(WIFI_AP);
    WiFi.softAP("ESP32-Config", "12345678");
    Serial.print("[WIFI] AP Mode, IP: ");
    Serial.println(WiFi.softAPIP());
    return;
  }
  
  WiFi.mode(WIFI_STA);
  WiFi.begin(cfg.ssid, cfg.password);
  Serial.printf("[WIFI] Connecting to %s", cfg.ssid);
  
  int attempts = 0;
  while (WiFi.status() != WL_CONNECTED && attempts < 30) {
    delay(500);
    Serial.print(".");
    attempts++;
  }
  
  if (WiFi.status() == WL_CONNECTED) {
    Serial.printf("\n[WIFI] Connected! IP: %s\n", WiFi.localIP().toString().c_str());
  } else {
    Serial.println("\n[WIFI] Failed, starting AP");
    WiFi.mode(WIFI_AP);
    WiFi.softAP("ESP32-Config", "12345678");
  }
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
  <title>ESP32 IoT Config</title>
  <style>
    *{box-sizing:border-box}
    body{font-family:system-ui,-apple-system,sans-serif;background:linear-gradient(135deg,#1a1a2e 0%,#16213e 100%);color:#eee;padding:20px;margin:0;min-height:100vh}
    .container{max-width:500px;margin:0 auto}
    h1{color:#e94560;text-align:center;margin-bottom:5px}
    .subtitle{text-align:center;color:#888;margin-bottom:20px;font-size:14px}
    .card{background:rgba(22,33,62,0.8);backdrop-filter:blur(10px);padding:20px;border-radius:12px;margin:15px 0;border:1px solid rgba(255,255,255,0.1)}
    input,select{width:100%;padding:12px;margin:8px 0;border-radius:8px;border:none;background:#0f3460;color:#eee;font-size:14px}
    input:focus,select:focus{outline:2px solid #e94560}
    button{background:linear-gradient(135deg,#e94560,#ff6b6b);color:white;padding:14px;border:none;border-radius:8px;cursor:pointer;width:100%;font-size:16px;font-weight:bold;margin-top:10px}
    button:hover{transform:translateY(-2px);box-shadow:0 5px 20px rgba(233,69,96,0.4)}
    button:active{transform:translateY(0)}
    h3{color:#0ea5e9;margin:20px 0 10px;font-size:16px}
    label{display:block;color:#aaa;font-size:13px;margin-top:12px}
    .status{display:grid;grid-template-columns:1fr 1fr;gap:10px;font-size:13px}
    .status-item{background:#0f3460;padding:10px;border-radius:6px}
    .status-item b{color:#e94560;display:block;font-size:11px;margin-bottom:3px}
    .mode-selector{display:flex;gap:10px;margin:10px 0}
    .mode-btn{flex:1;padding:15px;border-radius:8px;border:2px solid #333;background:#0f3460;cursor:pointer;text-align:center;transition:all 0.3s}
    .mode-btn.active{border-color:#e94560;background:#e9456020}
    .mode-btn h4{margin:0 0 5px;color:#fff}
    .mode-btn p{margin:0;font-size:11px;color:#888}
    .checkbox-row{display:flex;align-items:center;gap:10px;margin:15px 0}
    .checkbox-row input{width:auto}
    .badge{display:inline-block;padding:3px 8px;border-radius:4px;font-size:11px}
    .badge.online{background:#22c55e33;color:#22c55e}
    .badge.offline{background:#ef444433;color:#ef4444}
    .hidden{display:none}
  </style>
</head>
<body>
  <div class="container">
    <h1>🌐 ESP32 IoT</h1>
    <p class="subtitle">v%VERSION% • %CHIP_ID%</p>
    
    <div class="status">
      <div class="status-item"><b>IP</b>%IP%</div>
      <div class="status-item"><b>MODE</b>%MODE%</div>
      <div class="status-item"><b>TEMP</b>%TEMP%°C</div>
      <div class="status-item"><b>HUMIDITY</b>%HUM%%</div>
    </div>
    
    <div class="card">
      <form action="/save" method="POST">
        <h3>⚡ Operation Mode</h3>
        <div class="mode-selector">
          <div class="mode-btn %BACKEND_ACTIVE%" onclick="setMode(0)">
            <h4>☁️ Backend</h4>
            <p>Send to cloud</p>
          </div>
          <div class="mode-btn %MESH_ACTIVE%" onclick="setMode(1)">
            <h4>🕸️ Mesh</h4>
            <p>Local network</p>
          </div>
        </div>
        <input type="hidden" name="meshMode" id="meshMode" value="%MESH_MODE%">
        
        <div id="wifiSection">
          <h3>📶 WiFi</h3>
          <label>SSID</label>
          <input name="ssid" value="%SSID%" placeholder="Your WiFi network">
          <label>Password</label>
          <input name="password" type="password" value="%PASS%">
        </div>
        
        <div id="backendSection" class="%BACKEND_SECTION%">
          <h3>☁️ Backend</h3>
          <label>URL</label>
          <input name="backendUrl" value="%BACKEND%" placeholder="https://your-domain.com">
          <label>Device Token</label>
          <input name="deviceToken" value="%TOKEN%">
          <label>Push Interval (ms)</label>
          <input name="interval" type="number" value="%INTERVAL%" min="5000">
        </div>
        
        <h3>🔧 Device</h3>
        <label>Name</label>
        <input name="nodeName" value="%NODE%">
        
        <div class="checkbox-row">
          <input type="checkbox" name="dhtEnabled" id="dht" %DHT_CHK%>
          <label for="dht" style="margin:0;color:#eee">Enable DHT22 sensor</label>
        </div>
        
        <button type="submit">💾 Save & Reboot</button>
      </form>
    </div>
    
    <div class="card">
      <button onclick="location.href='/metrics'" style="background:#0ea5e9">📊 View Live Data (JSON)</button>
    </div>
  </div>
  
  <script>
    function setMode(mode) {
      document.getElementById('meshMode').value = mode;
      document.querySelectorAll('.mode-btn').forEach((el, i) => {
        el.classList.toggle('active', i === mode);
      });
      document.getElementById('backendSection').classList.toggle('hidden', mode === 1);
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
  html.replace("%INTERVAL%", String(cfg.metricsIntervalMs));
  html.replace("%DHT_CHK%", cfg.dhtEnabled ? "checked" : "");
  html.replace("%TEMP%", String(lastTemp, 1));
  html.replace("%HUM%", String(lastHum, 1));
  html.replace("%MESH_MODE%", String(cfg.meshMode));
  html.replace("%MODE%", cfg.meshMode ? "<span class='badge online'>MESH</span>" : "<span class='badge online'>BACKEND</span>");
  html.replace("%BACKEND_ACTIVE%", cfg.meshMode == 0 ? "active" : "");
  html.replace("%MESH_ACTIVE%", cfg.meshMode == 1 ? "active" : "");
  html.replace("%BACKEND_SECTION%", cfg.meshMode == 1 ? "hidden" : "");
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
  if (server.hasArg("interval")) cfg.metricsIntervalMs = max(5000, server.arg("interval").toInt());
  if (server.hasArg("meshMode")) cfg.meshMode = server.arg("meshMode").toInt();
  cfg.dhtEnabled = server.hasArg("dhtEnabled") ? 1 : 0;
  
  saveConfig();
  server.send(200, "text/html", R"(<html><body style='background:#1a1a2e;color:#eee;display:flex;justify-content:center;align-items:center;height:100vh;font-family:system-ui'><div style='text-align:center'><h1>✅ Saved!</h1><p>Rebooting in 2 seconds...</p></div></body></html>)");
  delay(2000);
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
  Serial.println("[WEB] Server ready");
}

// ---------------------------
// SETUP & LOOP
// ---------------------------
void setup() {
  Serial.begin(115200);
  delay(1000);
  Serial.printf("\n=== ESP32 IoT v%s ===\n", FIRMWARE_VERSION);
  
  loadConfig();
  if (cfg.dhtEnabled) dht.begin();
  
  if (cfg.meshMode == 1) {
    // MESH MODE
    Serial.println("[MODE] Mesh Network");
#ifndef DISABLE_MESH
    initMesh();
#endif
  } else {
    // BACKEND MODE
    Serial.println("[MODE] Backend (Cloud)");
    connectWiFi();
  }
  
  initWebServer();
  WiFi.scanNetworks(true);
  
  Serial.println("[READY]");
}

void loop() {
  server.handleClient();
  
  static unsigned long lastRead = 0;
  if (millis() - lastRead > 5000) {
    readSensors();
    lastRead = millis();
  }
  
  if (cfg.meshMode == 1) {
    // MESH MODE
#ifndef DISABLE_MESH
    mesh.update();
    
    if (millis() - lastPush > cfg.metricsIntervalMs) {
      broadcastToMesh();
      lastPush = millis();
    }
#endif
  } else {
    // BACKEND MODE
    if (millis() - lastPush > cfg.metricsIntervalMs) {
      pushMetricsToBackend();
      lastPush = millis();
    }
  }
}
