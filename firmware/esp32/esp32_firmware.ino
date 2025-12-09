// ===== ESP32 IoT Firmware v3.0.0 =====
// Simple Backend Mode - NO MESH
// Stable WiFi + HTTP to cloud

#include <WiFi.h>
#include <WiFiClientSecure.h>
#include <HTTPClient.h>
#include <WebServer.h>
#include <EEPROM.h>
#include <DHT.h>
#include <ArduinoJson.h>

// ---------------------------
// CONFIG
// ---------------------------
#define FIRMWARE_VERSION "3.0.0"
#define DHTPIN 15
#define DHTTYPE DHT22
#define EEPROM_SIZE 512
#define CONFIG_MAGIC 0xABCD1236  // Changed again - struct size changed

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
  char deviceToken[72];  // Token is 64 chars + null
  uint32_t intervalSec;
  uint8_t dhtEnabled;
};

ConfigData cfg;
unsigned long lastPush = 0;
float lastTemp = 0, lastHum = 0;

// ---------------------------
// HELPERS
// ---------------------------
String getChipId() {
  uint64_t chipid = ESP.getEfuseMac();
  char id[17];
  snprintf(id, sizeof(id), "%04X%08X", (uint16_t)(chipid >> 32), (uint32_t)chipid);
  return String(id);
}

void loadConfig() {
  EEPROM.begin(EEPROM_SIZE);
  EEPROM.get(0, cfg);
  
  if (cfg.magic != CONFIG_MAGIC) {
    Serial.println("[CFG] Initializing defaults");
    memset(&cfg, 0, sizeof(cfg));
    cfg.magic = CONFIG_MAGIC;
    strcpy(cfg.nodeName, "ESP32");
    cfg.intervalSec = 30;
    cfg.dhtEnabled = 1;
    EEPROM.put(0, cfg);
    EEPROM.commit();
  }
}

void saveConfig() {
  EEPROM.put(0, cfg);
  EEPROM.commit();
}

// ---------------------------
// SENSORS
// ---------------------------
void readSensors() {
  if (cfg.dhtEnabled) {
    float t = dht.readTemperature();
    float h = dht.readHumidity();
    if (!isnan(t)) lastTemp = t;
    if (!isnan(h)) lastHum = h;
  }
}

// ---------------------------
// JSON
// ---------------------------
String buildJSON() {
  StaticJsonDocument<768> doc;
  
  // Root level - what backend expects
  doc["temperature"] = lastTemp;
  doc["humidity"] = lastHum;
  doc["dht_enabled"] = cfg.dhtEnabled ? true : false;
  
  // System info
  JsonObject sys = doc.createNestedObject("system");
  sys["chip_id"] = getChipId();
  sys["mac"] = WiFi.macAddress();
  sys["platform"] = "ESP32";
  sys["firmware"] = FIRMWARE_VERSION;
  sys["free_heap"] = ESP.getFreeHeap();
  
  // Current WiFi
  JsonObject wifi = doc.createNestedObject("current_wifi");
  wifi["ssid"] = WiFi.SSID();
  wifi["rssi"] = WiFi.RSSI();
  wifi["ip"] = WiFi.localIP().toString();
  
  // Mesh status
  JsonObject mesh = doc.createNestedObject("mesh_status");
  mesh["enabled"] = false;
  mesh["running"] = false;
  mesh["node_count"] = 0;
  
  String out;
  serializeJson(doc, out);
  return out;
}

// ---------------------------
// WIFI
// ---------------------------
void connectWiFi() {
  if (strlen(cfg.ssid) == 0) {
    Serial.println("[WIFI] No SSID, AP mode");
    WiFi.mode(WIFI_AP);
    WiFi.softAP("ESP32-Setup", "12345678");
    Serial.print("[WIFI] AP IP: ");
    Serial.println(WiFi.softAPIP());
    return;
  }
  
  Serial.print("[WIFI] Connecting to ");
  Serial.println(cfg.ssid);
  
  WiFi.mode(WIFI_STA);
  WiFi.begin(cfg.ssid, cfg.password);
  
  int attempts = 0;
  while (WiFi.status() != WL_CONNECTED && attempts < 40) {
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
    Serial.println("[WIFI] Failed! Starting AP");
    WiFi.mode(WIFI_AP);
    WiFi.softAP("ESP32-Setup", "12345678");
  }
}

// ---------------------------
// HTTP PUSH
// ---------------------------
void pushMetrics() {
  if (strlen(cfg.backendUrl) < 5 || strlen(cfg.deviceToken) < 5) {
    Serial.println("[HTTP] Not configured");
    Serial.printf("[HTTP] URL len: %d, Token len: %d\n", strlen(cfg.backendUrl), strlen(cfg.deviceToken));
    return;
  }
  
  if (WiFi.status() != WL_CONNECTED) {
    Serial.println("[HTTP] No WiFi");
    return;
  }
  
  String url = String(cfg.backendUrl) + "/api/v1/metrics";
  String payload = buildJSON();
  
  Serial.println("==== HTTP DEBUG ====");
  Serial.print("URL: ");
  Serial.println(url);
  Serial.print("Token: [");
  Serial.print(cfg.deviceToken);
  Serial.println("]");
  Serial.print("Token length: ");
  Serial.println(strlen(cfg.deviceToken));
  Serial.println("====================");
  
  HTTPClient http;
  
  if (url.startsWith("https")) {
    WiFiClientSecure *client = new WiFiClientSecure;
    client->setInsecure();
    http.begin(*client, url);
  } else {
    WiFiClient client;
    http.begin(client, url);
  }
  
  http.addHeader("Content-Type", "application/json");
  http.addHeader("X-Device-Token", cfg.deviceToken);
  http.setTimeout(10000);
  
  int code = http.POST(payload);
  Serial.print("[HTTP] Response: ");
  Serial.println(code);
  
  if (code == 401) {
    Serial.println("[HTTP] 401 = Invalid token! Check token in web UI");
  }
  
  http.end();
}

// ---------------------------
// WEB UI
// ---------------------------
const char HTML[] PROGMEM = R"(
<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>ESP32</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui;background:#111;color:#eee;padding:20px}
.c{max-width:400px;margin:0 auto}
h1{color:#58a6ff;text-align:center;margin-bottom:20px}
.card{background:#1a1a1a;border-radius:12px;padding:20px;margin-bottom:16px}
.stats{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:20px}
.stat{background:#222;border-radius:8px;padding:16px;text-align:center}
.stat-val{font-size:32px;font-weight:bold}
.stat-lbl{font-size:12px;color:#888;margin-top:4px}
h3{color:#58a6ff;font-size:14px;margin-bottom:12px}
label{display:block;color:#888;font-size:12px;margin-bottom:4px}
input{width:100%;padding:12px;background:#222;border:1px solid #333;border-radius:8px;color:#eee;margin-bottom:12px}
input:focus{outline:none;border-color:#58a6ff}
button{width:100%;padding:14px;background:#238636;border:none;border-radius:8px;color:#fff;font-weight:bold;cursor:pointer}
button:hover{background:#2ea043}
.info{background:#238636;color:#fff;padding:8px 16px;border-radius:20px;display:inline-block;font-size:12px}
.center{text-align:center;margin-bottom:16px}
</style>
</head>
<body>
<div class="c">
<h1>ESP32 IoT</h1>
<p class="center"><span class="info">%IP%</span></p>

<div class="stats">
<div class="stat"><div class="stat-val">%TEMP%°</div><div class="stat-lbl">Temperature</div></div>
<div class="stat"><div class="stat-val">%HUM%%</div><div class="stat-lbl">Humidity</div></div>
</div>

<div class="card">
<form method="POST" action="/save">
<h3>WiFi</h3>
<label>SSID</label>
<input name="ssid" value="%SSID%">
<label>Password</label>
<input name="pass" type="password" value="%PASS%">

<h3>Backend</h3>
<label>URL</label>
<input name="url" value="%URL%" placeholder="https://chnu-iot.com">
<label>Token</label>
<input name="token" value="%TOKEN%">
<label>Interval (sec)</label>
<input name="interval" type="number" value="%INT%" min="10" max="300">

<h3>Device</h3>
<label>Name</label>
<input name="name" value="%NAME%">

<button type="submit">Save & Reboot</button>
</form>
</div>

<div class="card">
<button onclick="location.href='/json'">View JSON</button>
</div>
</div>
</body>
</html>
)";

void handleRoot() {
  String html = FPSTR(HTML);
  html.replace("%IP%", WiFi.localIP().toString());
  html.replace("%TEMP%", String(lastTemp, 1));
  html.replace("%HUM%", String(lastHum, 1));
  html.replace("%SSID%", cfg.ssid);
  html.replace("%PASS%", cfg.password);
  html.replace("%URL%", cfg.backendUrl);
  html.replace("%TOKEN%", cfg.deviceToken);
  html.replace("%INT%", String(cfg.intervalSec));
  html.replace("%NAME%", cfg.nodeName);
  server.send(200, "text/html", html);
}

void handleSave() {
  if (server.hasArg("ssid")) strncpy(cfg.ssid, server.arg("ssid").c_str(), 31);
  if (server.hasArg("pass")) strncpy(cfg.password, server.arg("pass").c_str(), 63);
  if (server.hasArg("url")) strncpy(cfg.backendUrl, server.arg("url").c_str(), 127);
  if (server.hasArg("token")) strncpy(cfg.deviceToken, server.arg("token").c_str(), 71);
  if (server.hasArg("name")) strncpy(cfg.nodeName, server.arg("name").c_str(), 31);
  if (server.hasArg("interval")) {
    int v = server.arg("interval").toInt();
    cfg.intervalSec = (v < 10) ? 10 : ((v > 300) ? 300 : v);
  }
  saveConfig();
  
  server.send(200, "text/html", "<h1 style='color:#fff;background:#111;height:100vh;display:flex;align-items:center;justify-content:center'>Rebooting...</h1>");
  delay(1000);
  ESP.restart();
}

void handleJSON() {
  server.send(200, "application/json", buildJSON());
}

// ---------------------------
// SETUP
// ---------------------------
void setup() {
  Serial.begin(115200);
  delay(500);
  
  Serial.println("\n\n=== ESP32 IoT v" FIRMWARE_VERSION " ===");
  Serial.println("Backend Mode (No Mesh)");
  
  loadConfig();
  Serial.printf("SSID: %s\n", cfg.ssid);
  Serial.printf("Backend: %s\n", cfg.backendUrl);
  Serial.printf("Interval: %d sec\n", cfg.intervalSec);
  
  if (cfg.dhtEnabled) dht.begin();
  
  connectWiFi();
  
  server.on("/", handleRoot);
  server.on("/save", HTTP_POST, handleSave);
  server.on("/json", handleJSON);
  server.begin();
  
  Serial.println("[READY]");
}

// ---------------------------
// LOOP
// ---------------------------
void loop() {
  server.handleClient();
  
  static unsigned long lastRead = 0;
  if (millis() - lastRead > 5000) {
    readSensors();
    lastRead = millis();
  }
  
  if (millis() - lastPush > (cfg.intervalSec * 1000UL)) {
    pushMetrics();
    lastPush = millis();
  }
}
