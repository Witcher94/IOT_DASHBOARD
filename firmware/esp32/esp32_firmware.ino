// ===== ESP32 IoT Firmware v2.3.0 =====
// Mesh + Backend simultaneously using AsyncHTTPClient
// Install: AsyncTCP, ESPAsyncWebServer

#include <WiFi.h>
#include <WebServer.h>
#include <EEPROM.h>
#include <painlessMesh.h>
#include <DHT.h>
#include <ArduinoJson.h>
#include <AsyncTCP.h>

// ---------------------------
// CONFIG
// ---------------------------
#define FIRMWARE_VERSION "2.3.0"
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

// Async HTTP client
AsyncClient* asyncClient = nullptr;
String httpRequestData;
bool httpInProgress = false;

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
unsigned long lastPush = 0;

// Forward declarations
void sendMetricsTask();
void readSensorsTask();

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

// Parse URL
String getHost(String url) {
  url.replace("http://", "");
  url.replace("https://", "");
  int slashPos = url.indexOf('/');
  if (slashPos > 0) return url.substring(0, slashPos);
  return url;
}

String getPath(String url) {
  url.replace("http://", "");
  url.replace("https://", "");
  int slashPos = url.indexOf('/');
  if (slashPos > 0) return url.substring(slashPos);
  return "/";
}

bool isHttps(String url) {
  return url.startsWith("https");
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
  StaticJsonDocument<1536> doc;
  
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
  
  JsonArray scan = wifi.createNestedArray("scan");
  int n = WiFi.scanComplete();
  if (n > 0) {
    for (int i = 0; i < min(n, (int)5); i++) {
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
  meshObj["enabled"] = meshRunning;
  if (meshRunning) {
    auto nodes = mesh.getNodeList();
    JsonArray nodesArr = meshObj.createNestedArray("nodes");
    for (auto &nodeId : nodes) {
      JsonObject node = nodesArr.createNestedObject();
      node["node_id"] = nodeId;
    }
  }
  
  doc["dht_enabled"] = cfg.dhtEnabled ? true : false;
  
  String output;
  serializeJson(doc, output);
  return output;
}

// ---------------------------
// ASYNC HTTP CLIENT CALLBACKS
// ---------------------------
void onAsyncConnect(void* arg, AsyncClient* client) {
  Serial.println("[HTTP] Connected, sending request...");
  
  String host = getHost(String(cfg.backendUrl));
  String path = "/api/v1/metrics";
  
  String request = "POST " + path + " HTTP/1.1\r\n";
  request += "Host: " + host + "\r\n";
  request += "Content-Type: application/json\r\n";
  request += "X-Device-Token: " + String(cfg.deviceToken) + "\r\n";
  request += "Content-Length: " + String(httpRequestData.length()) + "\r\n";
  request += "Connection: close\r\n\r\n";
  request += httpRequestData;
  
  client->write(request.c_str());
}

void onAsyncData(void* arg, AsyncClient* client, void* data, size_t len) {
  char* d = (char*)data;
  // Find HTTP status code
  size_t maxLen = (len < 50) ? len : 50;
  String response = String(d).substring(0, maxLen);
  int statusStart = response.indexOf(' ') + 1;
  int statusEnd = response.indexOf(' ', statusStart);
  String status = response.substring(statusStart, statusEnd);
  Serial.printf("[HTTP] Response: %s\n", status.c_str());
}

void onAsyncDisconnect(void* arg, AsyncClient* client) {
  Serial.println("[HTTP] Disconnected");
  httpInProgress = false;
  if (asyncClient) {
    delete asyncClient;
    asyncClient = nullptr;
  }
}

void onAsyncError(void* arg, AsyncClient* client, int8_t error) {
  Serial.printf("[HTTP] Error: %d\n", error);
  httpInProgress = false;
  if (asyncClient) {
    delete asyncClient;
    asyncClient = nullptr;
  }
}

void onAsyncTimeout(void* arg, AsyncClient* client, uint32_t time) {
  Serial.println("[HTTP] Timeout");
  client->close();
  httpInProgress = false;
}

// ---------------------------
// SEND METRICS (Async - works with mesh!)
// ---------------------------
void sendMetricsTask() {
  if (strlen(cfg.backendUrl) < 5 || strlen(cfg.deviceToken) < 5) return;
  if (!isRootNode) {
    Serial.println("[METRICS] Not root, skipping");
    return;
  }
  if (httpInProgress) {
    Serial.println("[METRICS] HTTP in progress, skipping");
    return;
  }
  
  Serial.println("[METRICS] Sending via AsyncTCP...");
  
  httpRequestData = buildMetricsJSON();
  String host = getHost(String(cfg.backendUrl));
  uint16_t port = isHttps(String(cfg.backendUrl)) ? 443 : 80;
  
  asyncClient = new AsyncClient();
  asyncClient->onConnect(onAsyncConnect);
  asyncClient->onData(onAsyncData);
  asyncClient->onDisconnect(onAsyncDisconnect);
  asyncClient->onError(onAsyncError);
  asyncClient->onTimeout(onAsyncTimeout);
  asyncClient->setRxTimeout(10);
  
  httpInProgress = true;
  
  if (!asyncClient->connect(host.c_str(), port)) {
    Serial.println("[HTTP] Connect failed");
    httpInProgress = false;
    delete asyncClient;
    asyncClient = nullptr;
  }
}

// ---------------------------
// MESH CALLBACKS
// ---------------------------
void meshReceived(uint32_t from, String &msg) {
  Serial.printf("[MESH] From %u: %s\n", from, msg.c_str());
}

void meshNewConnection(uint32_t nodeId) {
  Serial.printf("[MESH] New node: %u\n", nodeId);
}

void meshChangedConnections() {
  Serial.printf("[MESH] Nodes: %d\n", mesh.getNodeList().size() + 1);
}

void meshNodeTimeAdjusted(int32_t offset) {}

void initMesh() {
  mesh.setDebugMsgTypes(ERROR | STARTUP);
  mesh.init(MESH_PREFIX, MESH_PASSWORD, &userScheduler, MESH_PORT, WIFI_AP_STA, 6);
  
  mesh.onReceive(&meshReceived);
  mesh.onNewConnection(&meshNewConnection);
  mesh.onChangedConnections(&meshChangedConnections);
  mesh.onNodeTimeAdjusted(&meshNodeTimeAdjusted);
  
  // Bridge to external WiFi
  if (strlen(cfg.ssid) > 0) {
    mesh.stationManual(cfg.ssid, cfg.password);
    mesh.setRoot(true);
    mesh.setContainsRoot(true);
    isRootNode = true;
    Serial.printf("[MESH] Bridge mode -> %s\n", cfg.ssid);
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
  <title>ESP32 IoT</title>
  <style>
    *{box-sizing:border-box}
    body{font-family:system-ui;background:linear-gradient(135deg,#0f0f1a,#1a1a2e);color:#eee;padding:20px;margin:0;min-height:100vh}
    .container{max-width:500px;margin:0 auto}
    h1{color:#e94560;text-align:center;margin-bottom:5px}
    .subtitle{text-align:center;color:#666;margin-bottom:20px;font-size:13px}
    .card{background:rgba(22,33,62,0.9);padding:20px;border-radius:12px;margin:15px 0;border:1px solid #333}
    input{width:100%;padding:12px;margin:8px 0;border-radius:8px;border:1px solid #333;background:#0a0a15;color:#eee}
    input:focus{outline:none;border-color:#e94560}
    button{background:linear-gradient(135deg,#e94560,#ff6b6b);color:#fff;padding:14px;border:none;border-radius:8px;cursor:pointer;width:100%;font-weight:bold;font-size:15px}
    button:hover{opacity:0.9}
    h3{color:#0ea5e9;margin:20px 0 10px;font-size:15px}
    label{display:block;color:#888;font-size:12px;margin-top:10px}
    .grid{display:grid;grid-template-columns:1fr 1fr;gap:10px}
    .stat{background:#0a0a15;padding:12px;border-radius:8px;text-align:center}
    .stat-value{font-size:24px;font-weight:bold;color:#fff}
    .stat-label{font-size:11px;color:#666;margin-top:4px}
    .badge{display:inline-block;padding:4px 10px;border-radius:20px;font-size:11px;font-weight:bold}
    .online{background:#22c55e22;color:#22c55e}
    .root{background:#e9456022;color:#e94560}
    .info{background:#0ea5e922;color:#0ea5e9}
    .checkbox-row{display:flex;align-items:center;gap:10px;margin:15px 0}
    .checkbox-row input{width:auto;margin:0}
  </style>
</head>
<body>
  <div class="container">
    <h1>🌐 ESP32 IoT</h1>
    <p class="subtitle">v%VERSION% • %CHIP_ID%</p>
    
    <div class="grid">
      <div class="stat">
        <div class="stat-value">%TEMP%°</div>
        <div class="stat-label">Temperature</div>
      </div>
      <div class="stat">
        <div class="stat-value">%HUM%%</div>
        <div class="stat-label">Humidity</div>
      </div>
    </div>
    
    <div class="card" style="text-align:center">
      <span class="badge online">MESH %MESH_STATUS%</span>
      %ROOT_BADGE%
      <span class="badge info">%IP%</span>
    </div>
    
    <div class="card">
      <form action="/save" method="POST">
        <h3>📶 WiFi Bridge (for Backend)</h3>
        <label>SSID (your router)</label>
        <input name="ssid" value="%SSID%">
        <label>Password</label>
        <input name="password" type="password" value="%PASS%">
        
        <h3>☁️ Backend</h3>
        <label>URL</label>
        <input name="backendUrl" value="%BACKEND%" placeholder="https://chnu-iot.com">
        <label>Device Token</label>
        <input name="deviceToken" value="%TOKEN%">
        <label>Interval (ms)</label>
        <input name="interval" type="number" value="%INTERVAL%" min="10000">
        
        <h3>🔧 Settings</h3>
        <label>Node Name</label>
        <input name="nodeName" value="%NODE%">
        <div class="checkbox-row">
          <input type="checkbox" name="dhtEnabled" id="dht" %DHT_CHK%>
          <label for="dht" style="margin:0;color:#eee">DHT22 Sensor</label>
        </div>
        
        <button type="submit">💾 Save & Reboot</button>
      </form>
    </div>
    
    <div class="card">
      <button onclick="location.href='/metrics'" style="background:#0ea5e9">📊 JSON Metrics</button>
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
  html.replace("%DHT_CHK%", cfg.dhtEnabled ? "checked" : "");
  html.replace("%TEMP%", String(lastTemp, 1));
  html.replace("%HUM%", String(lastHum, 1));
  html.replace("%MESH_STATUS%", meshRunning ? "ON" : "OFF");
  html.replace("%ROOT_BADGE%", isRootNode ? "<span class='badge root'>ROOT</span>" : "");
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
    long interval = server.arg("interval").toInt();
    cfg.metricsIntervalMs = (uint32_t)max(10000L, interval);
  }
  cfg.dhtEnabled = server.hasArg("dhtEnabled") ? 1 : 0;
  saveConfig();
  
  server.send(200, "text/html", "<html><body style='background:#0f0f1a;color:#eee;display:flex;justify-content:center;align-items:center;height:100vh'><h1>✅ Rebooting...</h1></body></html>");
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
}

// ---------------------------
// SETUP & LOOP
// ---------------------------
void setup() {
  Serial.begin(115200);
  delay(1000);
  Serial.printf("\n=== ESP32 IoT v%s ===\n", FIRMWARE_VERSION);
  Serial.println("Mesh + Backend Bridge Mode");
  
  loadConfig();
  if (cfg.dhtEnabled) dht.begin();
  
  initMesh();
  initWebServer();
  
  taskSendMetrics.setInterval((unsigned long)cfg.metricsIntervalMs);
  taskSendMetrics.enable();
  
  WiFi.scanNetworks(true);
  Serial.println("[READY]");
}

void loop() {
  mesh.update();
  server.handleClient();
}
