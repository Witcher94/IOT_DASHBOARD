// ===== ESP Mesh + WiFi + DHT22 + Backend =====
// ESP32/ESP8266 з mesh мережею та відправкою метрик

// ---------------------------
// PLATFORM INCLUDES
// ---------------------------
#if defined(ESP32)
  #include <WiFi.h>
  #include <WiFiClientSecure.h>
  #include <WebServer.h>
  #include <HTTPClient.h>
  #include <Update.h>
  WebServer server(80);
  WiFiClientSecure secureClient;  // Global secure client
#elif defined(ESP8266)
  #include <ESP8266WiFi.h>
  #include <WiFiClientSecure.h>
  #include <ESP8266WebServer.h>
  #include <ESP8266HTTPClient.h>
  ESP8266WebServer server(80);
  WiFiClientSecure secureClient;  // Global secure client
#endif

#include <EEPROM.h>
#include <painlessMesh.h>
#include <DHT.h>
#include <ArduinoJson.h>

// ---------------------------
// CONFIG
// ---------------------------
#define FIRMWARE_VERSION "2.0.0"
#define DHTPIN 15
#define DHTTYPE DHT22
#define MESH_PREFIX   "LabMesh"
#define MESH_PASSWORD "LabMesh123"
#define MESH_PORT     5555
#define EEPROM_SIZE 512
#define CONFIG_MAGIC 0xDEADBEEF

DHT dht(DHTPIN, DHTTYPE);
painlessMesh mesh;

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
  uint8_t meshEnabled;
  uint8_t dhtEnabled;
};

ConfigData cfg;
bool meshRunning = false;
unsigned long lastPush = 0;

// ---------------------------
// HELPERS
// ---------------------------
String getChipId() {
#if defined(ESP32)
  uint64_t chipid = ESP.getEfuseMac();
  char id[17];
  snprintf(id, sizeof(id), "%04X%08X", (uint16_t)(chipid >> 32), (uint32_t)chipid);
  return String(id);
#else
  return String(ESP.getChipId(), HEX);
#endif
}

String getMacAddress() {
  return WiFi.macAddress();
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
    strcpy(cfg.ssid, "");
    strcpy(cfg.password, "");
    strcpy(cfg.nodeName, "ESP-Node");
    strcpy(cfg.backendUrl, "");
    strcpy(cfg.deviceToken, "");
    cfg.metricsIntervalMs = 30000;
    cfg.meshEnabled = 1;
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
  
  // System info
  JsonObject sys = doc.createNestedObject("system");
  sys["chip_id"] = getChipId();
  sys["mac"] = getMacAddress();
  sys["platform"] = "ESP32";
  sys["firmware"] = FIRMWARE_VERSION;
  sys["free_heap"] = ESP.getFreeHeap();
  sys["uptime"] = millis() / 1000;
  
  // WiFi
  JsonObject wifi = doc.createNestedObject("wifi");
  wifi["ssid"] = WiFi.SSID();
  wifi["rssi"] = lastRssi;
  wifi["ip"] = WiFi.localIP().toString();
  
  // DHT
  if (cfg.dhtEnabled) {
    JsonObject dhtObj = doc.createNestedObject("dht");
    dhtObj["temperature"] = lastTemp;
    dhtObj["humidity"] = lastHum;
  }
  
  // Mesh
  JsonObject meshObj = doc.createNestedObject("mesh");
  meshObj["enabled"] = cfg.meshEnabled ? true : false;
  if (meshRunning) {
    meshObj["node_id"] = mesh.getNodeId();
    auto nodes = mesh.getNodeList();
    JsonArray nodesArr = meshObj.createNestedArray("nodes");
    for (auto &n : nodes) nodesArr.add(n);
  }
  
  String output;
  serializeJson(doc, output);
  return output;
}

// ---------------------------
// BACKEND COMMUNICATION
// ---------------------------
void pushMetrics() {
  if (strlen(cfg.backendUrl) < 5 || strlen(cfg.deviceToken) < 5) {
    return;
  }
  
  // Pause mesh to avoid TCP conflicts
  bool wasMeshRunning = meshRunning;
  if (meshRunning) {
    mesh.stop();
    meshRunning = false;
    delay(200);
  }
  
  HTTPClient http;
  String url = String(cfg.backendUrl) + "/api/v1/metrics";
  Serial.println("[BACKEND] POST " + url);
  
  bool isHttps = String(cfg.backendUrl).startsWith("https");
  
  if (isHttps) {
    secureClient.setInsecure();  // Skip cert verification
    http.begin(secureClient, url);
  } else {
    WiFiClient client;
    http.begin(client, url);
  }
  
  http.addHeader("Content-Type", "application/json");
  http.addHeader("X-Device-Token", cfg.deviceToken);
  http.setTimeout(15000);
  
  String body = buildMetricsJSON();
  int code = http.POST(body);
  
  if (code > 0) {
    Serial.printf("[BACKEND] Response: %d\n", code);
  } else {
    Serial.printf("[BACKEND] Error: %s\n", http.errorToString(code).c_str());
  }
  
  http.end();
  
  // Resume mesh after delay
  if (wasMeshRunning && cfg.meshEnabled) {
    delay(500);
    initMesh();
  }
}

// ---------------------------
// MESH
// ---------------------------
void meshReceived(uint32_t from, String &msg) {
  Serial.printf("[MESH] From %u: %s\n", from, msg.c_str());
}

void meshNewConnection(uint32_t nodeId) {
  Serial.printf("[MESH] New node: %u\n", nodeId);
}

void meshDroppedConnection(uint32_t nodeId) {
  Serial.printf("[MESH] Node left: %u\n", nodeId);
}

void initMesh() {
  if (!cfg.meshEnabled) return;
  
  mesh.setDebugMsgTypes(ERROR | STARTUP);
  mesh.init(MESH_PREFIX, MESH_PASSWORD, MESH_PORT);
  mesh.onReceive(&meshReceived);
  mesh.onNewConnection(&meshNewConnection);
  mesh.onDroppedConnection(&meshDroppedConnection);
  
  if (strlen(cfg.ssid) > 0) {
    mesh.stationManual(cfg.ssid, cfg.password);
  }
  
  meshRunning = true;
  Serial.println("[MESH] Started");
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
  <title>ESP Config</title>
  <style>
    body { font-family: Arial; background: #1a1a2e; color: #eee; padding: 20px; }
    .card { background: #16213e; padding: 20px; border-radius: 10px; margin: 10px 0; }
    input, select { width: 100%; padding: 10px; margin: 5px 0; border-radius: 5px; border: none; }
    button { background: #e94560; color: white; padding: 12px 24px; border: none; border-radius: 5px; cursor: pointer; margin: 5px; }
    button:hover { background: #ff6b6b; }
    h1 { color: #e94560; }
    label { display: block; margin-top: 10px; color: #aaa; }
    .status { padding: 10px; background: #0f3460; border-radius: 5px; margin: 10px 0; }
  </style>
</head>
<body>
  <h1>🔧 ESP IoT Config</h1>
  
  <div class="status">
    <b>Chip:</b> %CHIP_ID% | <b>IP:</b> %IP% | <b>RSSI:</b> %RSSI% dBm
  </div>
  
  <div class="card">
    <h3>📡 WiFi</h3>
    <form action="/save" method="POST">
      <label>SSID</label>
      <input name="ssid" value="%SSID%">
      <label>Password</label>
      <input name="password" type="password" value="%PASS%">
      <label>Node Name</label>
      <input name="nodeName" value="%NODE%">
      
      <h3>☁️ Backend</h3>
      <label>Backend URL (e.g. https://example.com)</label>
      <input name="backendUrl" value="%BACKEND%">
      <label>Device Token</label>
      <input name="deviceToken" value="%TOKEN%">
      
      <h3>⚙️ Settings</h3>
      <label>Metrics Interval (ms)</label>
      <input name="interval" type="number" value="%INTERVAL%">
      
      <label><input type="checkbox" name="meshEnabled" %MESH_CHK%> Enable Mesh</label>
      <label><input type="checkbox" name="dhtEnabled" %DHT_CHK%> Enable DHT Sensor</label>
      
      <br><br>
      <button type="submit">💾 Save</button>
      <button type="button" onclick="location.href='/reboot'">🔄 Reboot</button>
    </form>
  </div>
  
  <div class="card">
    <h3>📊 Current Readings</h3>
    <p>🌡️ Temperature: %TEMP% °C</p>
    <p>💧 Humidity: %HUM% %</p>
    <p>📶 WiFi RSSI: %RSSI% dBm</p>
    <p>🔗 Mesh Nodes: %MESH_NODES%</p>
  </div>
</body>
</html>
)rawliteral";

String processTemplate(String html) {
  html.replace("%CHIP_ID%", getChipId());
  html.replace("%IP%", WiFi.localIP().toString());
  html.replace("%RSSI%", String(lastRssi));
  html.replace("%SSID%", cfg.ssid);
  html.replace("%PASS%", cfg.password);
  html.replace("%NODE%", cfg.nodeName);
  html.replace("%BACKEND%", cfg.backendUrl);
  html.replace("%TOKEN%", cfg.deviceToken);
  html.replace("%INTERVAL%", String(cfg.metricsIntervalMs));
  html.replace("%MESH_CHK%", cfg.meshEnabled ? "checked" : "");
  html.replace("%DHT_CHK%", cfg.dhtEnabled ? "checked" : "");
  html.replace("%TEMP%", String(lastTemp, 1));
  html.replace("%HUM%", String(lastHum, 1));
  html.replace("%MESH_NODES%", meshRunning ? String(mesh.getNodeList().size()) : "N/A");
  return html;
}

void handleRoot() {
  String html = FPSTR(HTML_PAGE);
  server.send(200, "text/html", processTemplate(html));
}

void handleSave() {
  if (server.hasArg("ssid")) strncpy(cfg.ssid, server.arg("ssid").c_str(), sizeof(cfg.ssid)-1);
  if (server.hasArg("password")) strncpy(cfg.password, server.arg("password").c_str(), sizeof(cfg.password)-1);
  if (server.hasArg("nodeName")) strncpy(cfg.nodeName, server.arg("nodeName").c_str(), sizeof(cfg.nodeName)-1);
  if (server.hasArg("backendUrl")) strncpy(cfg.backendUrl, server.arg("backendUrl").c_str(), sizeof(cfg.backendUrl)-1);
  if (server.hasArg("deviceToken")) strncpy(cfg.deviceToken, server.arg("deviceToken").c_str(), sizeof(cfg.deviceToken)-1);
  if (server.hasArg("interval")) cfg.metricsIntervalMs = server.arg("interval").toInt();
  cfg.meshEnabled = server.hasArg("meshEnabled") ? 1 : 0;
  cfg.dhtEnabled = server.hasArg("dhtEnabled") ? 1 : 0;
  
  saveConfig();
  server.send(200, "text/html", "<h1>Saved! Rebooting...</h1><script>setTimeout(()=>location='/',3000)</script>");
  delay(1000);
  ESP.restart();
}

void handleReboot() {
  server.send(200, "text/html", "<h1>Rebooting...</h1><script>setTimeout(()=>location='/',5000)</script>");
  delay(500);
  ESP.restart();
}

void handleMetrics() {
  server.send(200, "application/json", buildMetricsJSON());
}

void initWebServer() {
  server.on("/", handleRoot);
  server.on("/save", HTTP_POST, handleSave);
  server.on("/reboot", handleReboot);
  server.on("/metrics", handleMetrics);
  server.begin();
  Serial.println("[WEB] Server started on port 80");
}

// ---------------------------
// WIFI
// ---------------------------
void connectWiFi() {
  if (strlen(cfg.ssid) == 0) {
    Serial.println("[WIFI] No SSID configured, starting AP...");
    WiFi.mode(WIFI_AP);
    WiFi.softAP("ESP-Config", "12345678");
    Serial.print("[WIFI] AP IP: ");
    Serial.println(WiFi.softAPIP());
    return;
  }
  
  Serial.printf("[WIFI] Connecting to %s", cfg.ssid);
  WiFi.mode(WIFI_STA);
  WiFi.begin(cfg.ssid, cfg.password);
  
  int attempts = 0;
  while (WiFi.status() != WL_CONNECTED && attempts < 30) {
    delay(500);
    Serial.print(".");
    attempts++;
  }
  
  if (WiFi.status() == WL_CONNECTED) {
    Serial.println();
    Serial.print("[WIFI] Connected! IP: ");
    Serial.println(WiFi.localIP());
  } else {
    Serial.println();
    Serial.println("[WIFI] Failed! Starting AP...");
    WiFi.mode(WIFI_AP);
    WiFi.softAP("ESP-Config", "12345678");
  }
}

// ---------------------------
// SETUP
// ---------------------------
void setup() {
  Serial.begin(115200);
  delay(1000);
  
  Serial.println("\n=== ESP IoT v" FIRMWARE_VERSION " ===");
  
  loadConfig();
  Serial.printf("Node: %s, Interval: %dms\n", cfg.nodeName, cfg.metricsIntervalMs);
  
  if (cfg.dhtEnabled) {
    dht.begin();
  }
  
  connectWiFi();
  
  // Initialize secure client early (before mesh)
  secureClient.setInsecure();
  Serial.println("[SSL] Client ready");
  
  initWebServer();
  
  if (cfg.meshEnabled && WiFi.status() == WL_CONNECTED) {
    delay(1000);  // Let WiFi stabilize
    initMesh();
  }
  
  Serial.println("[READY]");
}

// ---------------------------
// LOOP
// ---------------------------
void loop() {
  server.handleClient();
  
  if (meshRunning) {
    mesh.update();
  }
  
  // Read sensors periodically
  static unsigned long lastRead = 0;
  if (millis() - lastRead > 5000) {
    readSensors();
    lastRead = millis();
  }
  
  // Push metrics to backend
  if (strlen(cfg.backendUrl) > 5 && strlen(cfg.deviceToken) > 5) {
    if (millis() - lastPush > cfg.metricsIntervalMs) {
      pushMetrics();
      lastPush = millis();
    }
  }
}
