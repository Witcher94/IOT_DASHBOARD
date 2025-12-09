// ===== ESP Mesh + WiFi Metrics + DHT22 + Config UI + OTA =====
// Працює на ESP32 і ESP8266, mesh як окрема мережа, WiFi STA для інтернету
// З підтримкою токен авторизації та OTA оновлень

// ---------------------------
// ПЛАТФОРМО-ЗАЛЕЖНІ ІНКЛЮДИ
// ---------------------------
#if defined(ESP32)
  #include <WiFi.h>
  #include <WebServer.h>
  #include <HTTPClient.h>
  #include <base64.h>
  #include <Update.h>
  #include <HTTPUpdate.h>
  WebServer server(80);
#elif defined(ESP8266)
  #include <ESP8266WiFi.h>
  #include <ESP8266WebServer.h>
  #include <ESP8266HTTPClient.h>
  #include <base64.h>
  #include <ESP8266httpUpdate.h>
  ESP8266WebServer server(80);
#else
  #error "Only ESP32 or ESP8266 are supported"
#endif

#include <EEPROM.h>
#include <painlessMesh.h>
#include <DHT.h>
#include <ArduinoJson.h>

// ---------------------------
// DHT CONFIG
// ---------------------------
#define DHTPIN 15
#define DHTTYPE DHT22
DHT dht(DHTPIN, DHTTYPE);

// ---------------------------
// MESH CONFIG
// ---------------------------
#define MESH_PREFIX   "LabMesh"
#define MESH_PASSWORD "LabMesh123"
#define MESH_PORT     5555

painlessMesh mesh;

// ---------------------------
// FIRMWARE VERSION
// ---------------------------
#define FIRMWARE_VERSION "1.0.0"

// ---------------------------
// EEPROM CONFIG
// ---------------------------
#define EEPROM_SIZE 512
#define CONFIG_MAGIC 0xDEADBEEF

struct ConfigData {
  uint32_t magic;

  char ssid[32];
  char password[64];
  char nodeName[32];

  char backendUrl[80];
  char deviceToken[64];  // Токен пристрою замість user/pass

  uint32_t metricsIntervalMs;
  uint8_t  meshEnabled;
  uint8_t  sendToBackend;
  uint8_t  dhtEnabled;     // Чи увімкнено DHT датчик

  uint8_t  reserved[16];
};

ConfigData cfg;

bool meshRunning = false;
unsigned long lastBackendPush = 0;
unsigned long lastCommandCheck = 0;
#define COMMAND_CHECK_INTERVAL 10000  // Перевірка команд кожні 10 сек

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

String encTypeToString(uint8_t type) {
#if defined(ESP32)
  wifi_auth_mode_t t = (wifi_auth_mode_t)type;
  switch (t) {
    case WIFI_AUTH_OPEN:        return "OPEN";
    case WIFI_AUTH_WEP:         return "WEP";
    case WIFI_AUTH_WPA_PSK:     return "WPA";
    case WIFI_AUTH_WPA2_PSK:    return "WPA2";
    case WIFI_AUTH_WPA_WPA2_PSK:return "WPA/WPA2";
    case WIFI_AUTH_WPA3_PSK:    return "WPA3";
    default:                    return "UNKNOWN";
  }
#else
  switch (type) {
    case ENC_TYPE_NONE: return "OPEN";
    case ENC_TYPE_WEP:  return "WEP";
    case ENC_TYPE_TKIP: return "WPA";
    case ENC_TYPE_CCMP: return "WPA2";
    case ENC_TYPE_AUTO: return "AUTO";
    default:            return "UNKNOWN";
  }
#endif
}

// ---------------------------
// EEPROM LOAD/SAVE
// ---------------------------
void saveConfig() {
  cfg.magic = CONFIG_MAGIC;
  EEPROM.begin(EEPROM_SIZE);
  EEPROM.put(0, cfg);
  EEPROM.commit();
}

void loadConfig() {
  EEPROM.begin(EEPROM_SIZE);
  EEPROM.get(0, cfg);

  if (cfg.magic != CONFIG_MAGIC) {
    memset(&cfg, 0, sizeof(cfg));
    cfg.magic = CONFIG_MAGIC;
    strcpy(cfg.nodeName, "ESP Node");
    cfg.metricsIntervalMs = 30000;
    cfg.meshEnabled = 1;
    cfg.sendToBackend = 0;
    cfg.dhtEnabled = 1;
    saveConfig();
  }
}

// ---------------------------
// WIFI / MESH JSON
// ---------------------------
String getWifiScanJSON() {
  int n = WiFi.scanNetworks(false, true);
  String json = "\"wifi_scan\":[";

  for (int i = 0; i < n; i++) {
    if (i > 0) json += ",";
    json += "{";
    json += "\"ssid\":\"" + WiFi.SSID(i) + "\",";
    json += "\"rssi\":" + String(WiFi.RSSI(i)) + ",";
    json += "\"bssid\":\"" + WiFi.BSSIDstr(i) + "\",";
    json += "\"channel\":" + String(WiFi.channel(i)) + ",";
    json += "\"enc\":\"" + encTypeToString(WiFi.encryptionType(i)) + "\"";
    json += "}";
  }

  json += "]";
  return json;
}

String getCurrentWifiJSON() {
  if (WiFi.status() != WL_CONNECTED)
    return "\"current_wifi\":null";

  String json = "\"current_wifi\":{";
  json += "\"ssid\":\"" + WiFi.SSID() + "\",";
  json += "\"rssi\":" + String(WiFi.RSSI()) + ",";
  json += "\"bssid\":\"" + WiFi.BSSIDstr() + "\",";
  json += "\"ip\":\"" + WiFi.localIP().toString() + "\",";
  json += "\"channel\":" + String(WiFi.channel());
  json += "}";
  return json;
}

String getMeshNeighborsJSON() {
  String json = "\"mesh_neighbors\":[";
  if (meshRunning) {
    auto nodes = mesh.getNodeList(true);
    size_t idx = 0;

    for (auto id : nodes) {
      json += "{\"id\":" + String(id) + "}";
      if (idx < nodes.size() - 1) json += ",";
      idx++;
    }
  }
  json += "]";
  return json;
}

String getMeshStatusJSON() {
  String json = "\"mesh_status\":{";
  json += "\"enabled\":" + String(cfg.meshEnabled ? "true" : "false") + ",";
  json += "\"running\":" + String(meshRunning ? "true" : "false") + ",";
  if (meshRunning) {
    json += "\"node_id\":" + String(mesh.getNodeId()) + ",";
    json += "\"node_count\":" + String(mesh.getNodeList(true).size() + 1);
  } else {
    json += "\"node_id\":0,";
    json += "\"node_count\":0";
  }
  json += "}";
  return json;
}

// ---------------------------
// SYSTEM INFO JSON
// ---------------------------
String getSystemInfoJSON() {
  String json = "\"system\":{";
  json += "\"chip_id\":\"" + getChipId() + "\",";
  json += "\"mac\":\"" + getMacAddress() + "\",";
  json += "\"firmware\":\"" + String(FIRMWARE_VERSION) + "\",";
#if defined(ESP32)
  json += "\"platform\":\"ESP32\",";
  json += "\"free_heap\":" + String(ESP.getFreeHeap()) + ",";
  json += "\"cpu_freq\":" + String(ESP.getCpuFreqMHz());
#else
  json += "\"platform\":\"ESP8266\",";
  json += "\"free_heap\":" + String(ESP.getFreeHeap()) + ",";
  json += "\"cpu_freq\":" + String(ESP.getCpuFreqMHz());
#endif
  json += "}";
  return json;
}

// ---------------------------
// METRICS JSON
// ---------------------------
String buildMetricsJSON() {
  float h = 0, t = 0;
  if (cfg.dhtEnabled) {
    h = dht.readHumidity();
    t = dht.readTemperature();
  }

  String json = "{";

  json += "\"node_name\":\"" + String(cfg.nodeName) + "\",";
  json += "\"node_id\":" + String(meshRunning ? mesh.getNodeId() : 0) + ",";
  json += "\"device_token\":\"" + String(cfg.deviceToken) + "\",";

  if (cfg.dhtEnabled) {
    json += "\"temperature\":" + String(isnan(t) ? 0 : t) + ",";
    json += "\"humidity\":" + String(isnan(h) ? 0 : h) + ",";
    json += "\"dht_enabled\":true,";
  } else {
    json += "\"temperature\":null,";
    json += "\"humidity\":null,";
    json += "\"dht_enabled\":false,";
  }

  json += getSystemInfoJSON() + ",";
  json += getCurrentWifiJSON() + ",";
  json += getWifiScanJSON() + ",";
  json += getMeshNeighborsJSON() + ",";
  json += getMeshStatusJSON();

  json += "}";
  return json;
}

// ---------------------------
// CONFIG PAGE (DARK UI)
// ---------------------------
String buildConfigPage() {
  String page = R"rawliteral(
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>ESP IoT Config</title>
  <style>
    * { box-sizing: border-box; }
    body {
      background: linear-gradient(135deg, #0a0a12 0%, #1a1a2e 100%);
      min-height: 100vh;
      color: #e0e0e0;
      font-family: 'SF Pro Display', -apple-system, BlinkMacSystemFont, "Segoe UI", Arial, sans-serif;
      padding: 30px;
      margin: 0;
    }
    .card {
      max-width: 520px;
      margin: auto;
      background: rgba(20, 20, 30, 0.95);
      padding: 32px;
      border-radius: 20px;
      box-shadow: 0 8px 32px rgba(0,0,0,0.6), 0 0 60px rgba(78, 195, 255, 0.1);
      border: 1px solid rgba(78, 195, 255, 0.2);
      backdrop-filter: blur(10px);
    }
    h2 {
      text-align: center;
      background: linear-gradient(135deg, #4ec3ff, #63ffa3);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
      background-clip: text;
      font-weight: 600;
      margin-top: 0;
      font-size: 26px;
    }
    .subtitle {
      text-align: center;
      color: #6a6a8a;
      margin-top: -10px;
      margin-bottom: 24px;
      font-size: 13px;
    }
    label {
      display: block;
      margin-top: 16px;
      font-size: 13px;
      color: #9090a8;
      font-weight: 500;
      text-transform: uppercase;
      letter-spacing: 0.5px;
    }
    select, input[type="text"], input[type="password"], input[type="number"] {
      width: 100%;
      padding: 12px 14px;
      margin-top: 6px;
      background: rgba(27, 27, 40, 0.8);
      border: 1px solid #343450;
      color: #f0f0ff;
      border-radius: 10px;
      font-size: 14px;
      outline: none;
      transition: all 0.3s ease;
    }
    select:focus, input:focus {
      border-color: #4ec3ff;
      box-shadow: 0 0 0 3px rgba(78,195,255,0.15);
      background: rgba(30, 30, 50, 0.9);
    }
    .row {
      display: flex;
      gap: 12px;
    }
    .row .col {
      flex: 1;
    }
    .checkbox-row {
      display: flex;
      align-items: center;
      gap: 10px;
      margin-top: 14px;
      padding: 10px 14px;
      background: rgba(27, 27, 40, 0.5);
      border-radius: 10px;
      cursor: pointer;
      transition: background 0.2s;
    }
    .checkbox-row:hover {
      background: rgba(40, 40, 60, 0.6);
    }
    .checkbox-row input[type=checkbox] {
      width: 18px;
      height: 18px;
      accent-color: #4ec3ff;
    }
    .checkbox-row span {
      font-size: 14px;
    }
    button {
      margin-top: 28px;
      width: 100%;
      padding: 14px;
      background: linear-gradient(135deg, #4ec3ff, #63ffa3);
      border: none;
      color: #000;
      font-size: 15px;
      border-radius: 999px;
      cursor: pointer;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 1px;
      transition: transform 0.2s, box-shadow 0.2s;
    }
    button:hover {
      transform: translateY(-2px);
      box-shadow: 0 8px 24px rgba(78, 195, 255, 0.3);
    }
    #manual_ssid_field {
      display: none;
    }
    small {
      color: #606080;
      font-size: 11px;
    }
    hr {
      border: none;
      border-top: 1px solid rgba(78, 195, 255, 0.15);
      margin: 24px 0;
    }
    .section-title {
      font-size: 12px;
      color: #4ec3ff;
      text-transform: uppercase;
      letter-spacing: 1px;
      margin-top: 24px;
      margin-bottom: -8px;
    }
    .info-box {
      background: rgba(78, 195, 255, 0.08);
      border: 1px solid rgba(78, 195, 255, 0.2);
      border-radius: 10px;
      padding: 12px;
      margin-top: 12px;
      font-size: 12px;
      color: #90a0c0;
    }
    .info-box code {
      background: rgba(0,0,0,0.3);
      padding: 2px 6px;
      border-radius: 4px;
      color: #4ec3ff;
    }
  </style>
  <script>
    function toggleManualSSID() {
      var sel = document.getElementById('ssid_select');
      var manual = document.getElementById('manual_ssid_field');
      if (sel.value === 'manual') {
        manual.style.display = 'block';
      } else {
        manual.style.display = 'none';
      }
    }
  </script>
</head>
<body>
  <div class="card">
    <h2>ESP IoT Configuration</h2>
    <p class="subtitle">Mesh Network & Sensor Dashboard</p>
    <form method="POST" action="/save">
)rawliteral";

  // System info
  page += "<div class=\"info-box\">";
  page += "<strong>Device:</strong> <code>" + getChipId() + "</code><br>";
  page += "<strong>MAC:</strong> <code>" + getMacAddress() + "</code><br>";
  page += "<strong>Firmware:</strong> <code>v" + String(FIRMWARE_VERSION) + "</code>";
  page += "</div>";

  // WiFi scan
  int n = WiFi.scanNetworks();
  page += R"rawliteral(
      <p class="section-title">WiFi Settings</p>
      <label>Wi-Fi Network</label>
      <select id="ssid_select" name="ssid_select" onchange="toggleManualSSID()">
        <option value="" disabled selected>Select Wi-Fi</option>
)rawliteral";

  for (int i = 0; i < n; i++) {
    String s = WiFi.SSID(i);
    int rssi = WiFi.RSSI(i);
    page += "<option value=\"" + s + "\">" + s + " (" + String(rssi) + " dBm)</option>";
  }

  page += R"rawliteral(
        <option value="manual">Manual input…</option>
      </select>

      <div id="manual_ssid_field">
        <label>Manual SSID</label>
        <input type="text" name="ssid_manual" placeholder="Enter Wi-Fi SSID">
      </div>

      <label>Password</label>
      <input type="password" name="password" placeholder="Wi-Fi password">
)rawliteral";

  // Device settings
  page += "<p class=\"section-title\">Device Settings</p>";
  page += "<label>Device Name</label><input type=\"text\" name=\"nodeName\" value=\"" +
           String(cfg.nodeName) + "\" placeholder=\"ESP Node\">";

  page += "<label>Metrics interval (sec)</label><input type=\"number\" name=\"interval\" min=\"5\" value=\"" +
           String(cfg.metricsIntervalMs / 1000) + "\">";

  // Mesh + DHT toggles
  page += R"rawliteral(
      <div class="checkbox-row">
        <input type="checkbox" name="mesh" value="1" )rawliteral";
  if (cfg.meshEnabled) page += "checked";
  page += R"rawliteral(>
        <span>Enable Mesh Network</span>
      </div>
      <div class="checkbox-row">
        <input type="checkbox" name="dhtEnabled" value="1" )rawliteral";
  if (cfg.dhtEnabled) page += "checked";
  page += R"rawliteral(>
        <span>Enable DHT22 Sensor</span>
      </div>

      <hr>
      <p class="section-title">Backend Connection</p>

      <label>Backend URL</label>
)rawliteral";

  page += "<input type=\"text\" name=\"backendUrl\" placeholder=\"https://your-server.com\" value=\"" +
          String(cfg.backendUrl) + "\">";

  page += R"rawliteral(
      <label>Device Token</label>
)rawliteral";

  page += "<input type=\"password\" name=\"deviceToken\" placeholder=\"Paste token from dashboard\" value=\"" +
          String(cfg.deviceToken) + "\">";
  page += "<small>Get this token from your IoT Dashboard when adding a new device</small>";

  page += R"rawliteral(
      <div class="checkbox-row">
        <input type="checkbox" name="sendToBackend" value="1" )rawliteral";
  if (cfg.sendToBackend) page += "checked";
  page += R"rawliteral(>
        <span>Push metrics to backend</span>
      </div>

      <button type="submit">Save & Reboot</button>
    </form>
  </div>
</body>
</html>
)rawliteral";

  return page;
}

// ---------------------------
// HTTP HANDLERS
// ---------------------------
void handleConfigPage() {
  server.send(200, "text/html", buildConfigPage());
}

void handleSaveConfig() {
  String selected = server.arg("ssid_select");
  String manual   = server.arg("ssid_manual");

  String newSSID;
  if (selected == "manual") newSSID = manual;
  else newSSID = selected;

  String newPassword   = server.arg("password");
  String newNodeName   = server.arg("nodeName");
  int    intervalSec   = server.arg("interval").toInt();
  if (intervalSec < 5) intervalSec = 5;

  String backendUrl  = server.arg("backendUrl");
  String deviceToken = server.arg("deviceToken");

  memset(cfg.ssid, 0, sizeof(cfg.ssid));
  memset(cfg.password, 0, sizeof(cfg.password));
  memset(cfg.nodeName, 0, sizeof(cfg.nodeName));
  memset(cfg.backendUrl, 0, sizeof(cfg.backendUrl));
  memset(cfg.deviceToken, 0, sizeof(cfg.deviceToken));

  newSSID.toCharArray(cfg.ssid, sizeof(cfg.ssid));
  newPassword.toCharArray(cfg.password, sizeof(cfg.password));
  newNodeName.toCharArray(cfg.nodeName, sizeof(cfg.nodeName));
  backendUrl.toCharArray(cfg.backendUrl, sizeof(cfg.backendUrl));
  deviceToken.toCharArray(cfg.deviceToken, sizeof(cfg.deviceToken));

  cfg.metricsIntervalMs = (uint32_t)intervalSec * 1000UL;
  cfg.meshEnabled       = (server.hasArg("mesh") && server.arg("mesh") == "1") ? 1 : 0;
  cfg.sendToBackend     = (server.hasArg("sendToBackend") && server.arg("sendToBackend") == "1") ? 1 : 0;
  cfg.dhtEnabled        = (server.hasArg("dhtEnabled") && server.arg("dhtEnabled") == "1") ? 1 : 0;

  saveConfig();

  server.send(200, "text/html",
    R"rawliteral(
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>Saved</title>
  <style>
    body {
      background: linear-gradient(135deg, #0a0a12 0%, #1a1a2e 100%);
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      font-family: 'SF Pro Display', -apple-system, sans-serif;
      color: #e0e0e0;
    }
    .msg {
      text-align: center;
      animation: pulse 1.5s ease-in-out infinite;
    }
    .icon { font-size: 64px; margin-bottom: 20px; }
    h2 { color: #63ffa3; margin: 0; }
    p { color: #9090a8; }
    @keyframes pulse {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.6; }
    }
  </style>
</head>
<body>
  <div class="msg">
    <div class="icon">✓</div>
    <h2>Configuration Saved</h2>
    <p>Device is rebooting...</p>
  </div>
</body>
</html>
)rawliteral");

  delay(800);
  ESP.restart();
}

void handleHealth() {
  String json = "{\"status\":\"ok\",\"firmware\":\"" + String(FIRMWARE_VERSION) + "\",\"uptime\":" + String(millis()/1000) + "}";
  server.send(200, "application/json", json);
}

void handleMetrics() {
  server.send(200, "application/json", buildMetricsJSON());
}

void handleDashboard() {
  String html = R"rawliteral(
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta http-equiv="refresh" content="5">
  <title>ESP Dashboard</title>
  <style>
    body {
      background: linear-gradient(135deg, #0a0a12 0%, #1a1a2e 100%);
      min-height: 100vh;
      color: #e0e0e0;
      font-family: 'SF Mono', 'Fira Code', monospace;
      padding: 20px;
      margin: 0;
    }
    h2 {
      background: linear-gradient(135deg, #4ec3ff, #63ffa3);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
    }
    pre {
      background: rgba(20, 20, 30, 0.9);
      padding: 20px;
      border-radius: 12px;
      border: 1px solid rgba(78, 195, 255, 0.2);
      overflow-x: auto;
      font-size: 13px;
      line-height: 1.5;
    }
  </style>
</head>
<body>
  <h2>ESP Metrics Dashboard</h2>
  <pre>)rawliteral";
  html += buildMetricsJSON();
  html += "</pre></body></html>";

  server.send(200, "text/html", html);
}

// ---------------------------
// OTA UPDATE
// ---------------------------
void performOTAUpdate(String firmwareUrl) {
  Serial.println("[OTA] Starting update from: " + firmwareUrl);

  WiFiClient client;

#if defined(ESP32)
  httpUpdate.setLedPin(LED_BUILTIN, LOW);
  t_httpUpdate_return ret = httpUpdate.update(client, firmwareUrl);

  switch (ret) {
    case HTTP_UPDATE_FAILED:
      Serial.printf("[OTA] Update failed: %s\n", httpUpdate.getLastErrorString().c_str());
      break;
    case HTTP_UPDATE_NO_UPDATES:
      Serial.println("[OTA] No updates available");
      break;
    case HTTP_UPDATE_OK:
      Serial.println("[OTA] Update successful, rebooting...");
      break;
  }
#else
  ESPhttpUpdate.setLedPin(LED_BUILTIN, LOW);
  t_httpUpdate_return ret = ESPhttpUpdate.update(client, firmwareUrl);

  switch (ret) {
    case HTTP_UPDATE_FAILED:
      Serial.printf("[OTA] Update failed: %s\n", ESPhttpUpdate.getLastErrorString().c_str());
      break;
    case HTTP_UPDATE_NO_UPDATES:
      Serial.println("[OTA] No updates available");
      break;
    case HTTP_UPDATE_OK:
      Serial.println("[OTA] Update successful, rebooting...");
      break;
  }
#endif
}

// ---------------------------
// BACKEND COMMUNICATION
// ---------------------------
void pushMetricsToBackend() {
  if (!cfg.sendToBackend) return;

  if (strlen(cfg.backendUrl) < 5 || strlen(cfg.deviceToken) < 5) {
    Serial.println("[BACKEND] URL or Token not set");
    return;
  }

  WiFiClient client;
  HTTPClient http;

  String url = String(cfg.backendUrl) + "/api/v1/metrics";

  http.begin(client, url);
  http.addHeader("Content-Type", "application/json");
  http.addHeader("X-Device-Token", cfg.deviceToken);

  String body = buildMetricsJSON();
  int httpCode = http.POST(body);

  if (httpCode > 0) {
    Serial.printf("[BACKEND] Metrics sent, response: %d\n", httpCode);
    if (httpCode == 200) {
      String response = http.getString();
      Serial.println("[BACKEND] Response: " + response);
    }
  } else {
    Serial.printf("[BACKEND] Failed: %s\n", http.errorToString(httpCode).c_str());
  }

  http.end();
}

void checkForCommands() {
  if (!cfg.sendToBackend) return;

  if (strlen(cfg.backendUrl) < 5 || strlen(cfg.deviceToken) < 5) {
    return;
  }

  WiFiClient client;
  HTTPClient http;

  String url = String(cfg.backendUrl) + "/api/v1/devices/commands";

  http.begin(client, url);
  http.addHeader("X-Device-Token", cfg.deviceToken);

  int httpCode = http.GET();

  if (httpCode == 200) {
    String response = http.getString();
    Serial.println("[CMD] Received: " + response);

    // Parse JSON command
    StaticJsonDocument<512> doc;
    DeserializationError error = deserializeJson(doc, response);

    if (!error) {
      String command = doc["command"] | "";
      String commandId = doc["id"] | "";

      if (command == "reboot") {
        Serial.println("[CMD] Executing reboot...");
        acknowledgeCommand(commandId, "success");
        delay(500);
        ESP.restart();
      }
      else if (command == "update_firmware") {
        String firmwareUrl = doc["firmware_url"] | "";
        if (firmwareUrl.length() > 0) {
          acknowledgeCommand(commandId, "starting");
          performOTAUpdate(firmwareUrl);
        }
      }
      else if (command == "toggle_dht") {
        cfg.dhtEnabled = !cfg.dhtEnabled;
        saveConfig();
        acknowledgeCommand(commandId, "success");
        Serial.println("[CMD] DHT toggled to: " + String(cfg.dhtEnabled ? "ON" : "OFF"));
      }
      else if (command == "toggle_mesh") {
        cfg.meshEnabled = !cfg.meshEnabled;
        saveConfig();
        acknowledgeCommand(commandId, "success");
        Serial.println("[CMD] Mesh toggled, rebooting...");
        delay(500);
        ESP.restart();
      }
      else if (command == "set_interval") {
        int newInterval = doc["interval"] | 30;
        cfg.metricsIntervalMs = (uint32_t)newInterval * 1000UL;
        saveConfig();
        acknowledgeCommand(commandId, "success");
        Serial.printf("[CMD] Interval set to: %d sec\n", newInterval);
      }
      else if (command == "set_name") {
        String newName = doc["name"] | "";
        if (newName.length() > 0) {
          memset(cfg.nodeName, 0, sizeof(cfg.nodeName));
          newName.toCharArray(cfg.nodeName, sizeof(cfg.nodeName));
          saveConfig();
          acknowledgeCommand(commandId, "success");
          Serial.println("[CMD] Name set to: " + newName);
        }
      }
    }
  }

  http.end();
}

void acknowledgeCommand(String commandId, String status) {
  if (commandId.length() == 0) return;

  WiFiClient client;
  HTTPClient http;

  String url = String(cfg.backendUrl) + "/api/v1/devices/commands/" + commandId + "/ack";

  http.begin(client, url);
  http.addHeader("Content-Type", "application/json");
  http.addHeader("X-Device-Token", cfg.deviceToken);

  String body = "{\"status\":\"" + status + "\"}";
  http.POST(body);
  http.end();
}

// ---------------------------
// MESH INIT
// ---------------------------
void initMesh() {
  mesh.setDebugMsgTypes(ERROR | STARTUP | CONNECTION);
  mesh.init(MESH_PREFIX, MESH_PASSWORD, MESH_PORT, WIFI_STA);
  mesh.setContainsRoot(true);
  meshRunning = true;

  Serial.println("[MESH] Started mesh node, id=" + String(mesh.getNodeId()));
}

// ---------------------------
// MODES
// ---------------------------
void startConfigAP() {
  meshRunning = false;

#if defined(ESP32)
  WiFi.mode(WIFI_AP);
#elif defined(ESP8266)
  WiFi.mode(WIFI_AP);
#endif

  WiFi.softAP("ESP-IOT-CONFIG", "");

  Serial.println("[CONFIG] AP Ready at " + WiFi.softAPIP().toString());

  server.on("/", handleConfigPage);
  server.on("/save", HTTP_POST, handleSaveConfig);
  server.begin();
}

void startNormalWiFi() {
#if defined(ESP32)
  WiFi.mode(WIFI_STA);
#elif defined(ESP8266)
  WiFi.mode(WIFI_STA);
#endif

  WiFi.begin(cfg.ssid, cfg.password);

  Serial.print("[WiFi] Connecting");
  unsigned long start = millis();
  while (WiFi.status() != WL_CONNECTED && millis() - start < 20000) {
    Serial.print(".");
    delay(300);
  }
  Serial.println();

  if (WiFi.status() == WL_CONNECTED) {
    Serial.println("[WiFi] Connected, IP: " + WiFi.localIP().toString());
  } else {
    Serial.println("[WiFi] Failed to connect, falling back to CONFIG AP");
    startConfigAP();
    return;
  }

  if (cfg.meshEnabled) {
    initMesh();
  } else {
    meshRunning = false;
  }

  server.on("/", handleConfigPage);
  server.on("/metrics", handleMetrics);
  server.on("/health", handleHealth);
  server.on("/dashboard", handleDashboard);
  server.begin();
}

// ---------------------------
// SETUP / LOOP
// ---------------------------
void setup() {
  Serial.begin(115200);
  delay(500);

  Serial.println();
  Serial.println("=== ESP IoT Mesh + Sensors v" + String(FIRMWARE_VERSION) + " ===");

  if (cfg.dhtEnabled) {
    dht.begin();
  }
  loadConfig();

  Serial.println("Loaded config:");
  Serial.println("SSID: " + String(cfg.ssid));
  Serial.println("Node: " + String(cfg.nodeName));
  Serial.println("Interval: " + String(cfg.metricsIntervalMs) + " ms");
  Serial.println("Mesh enabled: " + String(cfg.meshEnabled ? "yes" : "no"));
  Serial.println("DHT enabled: " + String(cfg.dhtEnabled ? "yes" : "no"));

  if (strlen(cfg.ssid) == 0) {
    Serial.println("[BOOT] No SSID → CONFIG MODE");
    startConfigAP();
  } else {
    Serial.println("[BOOT] SSID found → NORMAL MODE");
    startNormalWiFi();
  }

  lastBackendPush = millis();
  lastCommandCheck = millis();
}

void loop() {
  if (meshRunning) {
    mesh.update();
  }
  server.handleClient();

  unsigned long now = millis();

  // Періодичний пуш метрик на бекенд
  if (cfg.sendToBackend &&
      now - lastBackendPush >= cfg.metricsIntervalMs &&
      WiFi.status() == WL_CONNECTED) {
    lastBackendPush = now;
    pushMetricsToBackend();
  }

  // Перевірка команд з бекенду
  if (cfg.sendToBackend &&
      now - lastCommandCheck >= COMMAND_CHECK_INTERVAL &&
      WiFi.status() == WL_CONNECTED) {
    lastCommandCheck = now;
    checkForCommands();
  }
}
