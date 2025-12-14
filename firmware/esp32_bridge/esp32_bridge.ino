// ===== ESP32 Mesh Bridge + Node v1.4.0 =====
// ROOT node: Gateway to RPi + sends own metrics
// Requires ESP32 Core 2.0.17!

#include <painlessMesh.h>
#include <ArduinoJson.h>
#include <DHT.h>

#define FIRMWARE_VERSION "bridge-1.4.0"
#define MESH_PREFIX   "iot_mesh"
#define MESH_PASSWORD "mesh12345"
#define MESH_PORT     5555
#define MESH_CHANNEL  6
#define SERIAL_BAUD   115200

// Hardware
#define DHTPIN       15
#define DHTTYPE      DHT22
#define LED_PIN      2

painlessMesh mesh;
DHT dht(DHTPIN, DHTTYPE);

uint32_t nodeCount = 0;
unsigned long lastMetrics = 0;
unsigned long lastHeartbeat = 0;
float lastTemp = NAN;
float lastHum = NAN;

#define METRICS_INTERVAL 30000
#define HEARTBEAT_INTERVAL 30000

// Read DHT sensor
void readSensors() {
    float t = dht.readTemperature();
    float h = dht.readHumidity();
    if (!isnan(t)) lastTemp = t;
    if (!isnan(h)) lastHum = h;
}

// Send own metrics to RPi (as mesh_data from self)
void sendOwnMetrics() {
    readSensors();
    
    StaticJsonDocument<512> doc;
    doc["type"] = "mesh_data";
    doc["from"] = mesh.getNodeId();
    
    JsonObject data = doc.createNestedObject("data");
    data["msg_type"] = "metrics";
    data["node_name"] = "Bridge";
    data["node_id"] = mesh.getNodeId();
    if (!isnan(lastTemp)) data["temperature"] = lastTemp;
    if (!isnan(lastHum)) data["humidity"] = lastHum;
    data["chip_id"] = String((uint32_t)ESP.getEfuseMac(), HEX);
    data["mac"] = WiFi.macAddress();
    data["firmware"] = FIRMWARE_VERSION;
    data["platform"] = "ESP32";
    data["free_heap"] = ESP.getFreeHeap();
    data["is_root"] = true;
    data["rssi"] = 0; // Bridge is root, connected via USB - no wireless RSSI
    
    String out;
    serializeJson(doc, out);
    Serial.println(out);
    
    digitalWrite(LED_PIN, !digitalRead(LED_PIN));
}

// Forward mesh data to RPi
void receivedCallback(uint32_t from, String &msg) {
    StaticJsonDocument<512> doc;
    doc["type"] = "mesh_data";
    doc["from"] = from;
    
    StaticJsonDocument<384> data;
    if (deserializeJson(data, msg) == DeserializationError::Ok) {
        doc["data"] = data;
    } else {
        doc["raw"] = msg;
    }
    
    String out;
    serializeJson(doc, out);
    Serial.println(out);
}

void newConnectionCallback(uint32_t nodeId) {
    nodeCount = mesh.getNodeList().size();
    Serial.printf("{\"type\":\"node_connected\",\"node_id\":%u,\"total\":%u}\n", nodeId, nodeCount);
}

void droppedConnectionCallback(uint32_t nodeId) {
    nodeCount = mesh.getNodeList().size();
    Serial.printf("{\"type\":\"node_disconnected\",\"node_id\":%u,\"total\":%u}\n", nodeId, nodeCount);
}

void sendHeartbeat() {
    Serial.printf("{\"type\":\"heartbeat\",\"node_id\":%u,\"nodes\":%u,\"heap\":%u,\"uptime\":%lu,\"temp\":%.1f,\"hum\":%.1f}\n",
        mesh.getNodeId(), nodeCount, ESP.getFreeHeap(), millis(), lastTemp, lastHum);
}

void handleCommand(const char* cmd, JsonVariant value) {
    if (strcmp(cmd, "reboot") == 0) {
        Serial.println("{\"type\":\"ack\",\"cmd\":\"reboot\",\"status\":\"rebooting\"}");
        delay(100);
        ESP.restart();
    } else if (strcmp(cmd, "status") == 0) {
        sendHeartbeat();
    } else if (strcmp(cmd, "toggle_dht") == 0) {
        // Toggle DHT reading (optional)
        Serial.println("{\"type\":\"ack\",\"cmd\":\"toggle_dht\"}");
    }
}

void processSerial() {
    if (!Serial.available()) return;
    String cmd = Serial.readStringUntil('\n');
    cmd.trim();
    if (cmd.length() == 0) return;
    
    StaticJsonDocument<256> doc;
    if (deserializeJson(doc, cmd)) return;
    
    const char* type = doc["type"];
    if (!type) return;
    
    if (strcmp(type, "broadcast") == 0) {
        JsonObject data = doc["data"];
        const char* cmdName = data["cmd"];
        
        // Execute command locally first (bridge is also a node)
        if (cmdName) {
            handleCommand(cmdName, data["value"]);
        }
        
        // Then broadcast to mesh
        String payload;
        serializeJson(data, payload);
        mesh.sendBroadcast(payload);
        Serial.printf("{\"type\":\"ack\",\"cmd\":\"broadcast\",\"target\":\"all\",\"payload\":\"%s\"}\n", cmdName);
    } else if (strcmp(type, "send") == 0) {
        uint32_t target = doc["target"];
        JsonObject data = doc["data"];
        String payload;
        serializeJson(data, payload);
        mesh.sendSingle(target, payload);
        Serial.println("{\"type\":\"ack\",\"cmd\":\"send\"}");
    } else if (strcmp(type, "status") == 0) {
        sendHeartbeat();
    } else if (strcmp(type, "cmd") == 0) {
        // Direct command to bridge
        const char* cmdName = doc["cmd"];
        if (cmdName) {
            handleCommand(cmdName, doc["value"]);
        }
    }
}

void setup() {
    Serial.begin(SERIAL_BAUD);
    delay(1000);
    
    pinMode(LED_PIN, OUTPUT);
    dht.begin();
    
    mesh.setDebugMsgTypes(ERROR | STARTUP);
    mesh.init(MESH_PREFIX, MESH_PASSWORD, MESH_PORT, WIFI_AP, MESH_CHANNEL);
    mesh.setRoot(true);
    mesh.setContainsRoot(true);
    
    mesh.onReceive(&receivedCallback);
    mesh.onNewConnection(&newConnectionCallback);
    mesh.onDroppedConnection(&droppedConnectionCallback);
    
    Serial.printf("{\"type\":\"ready\",\"node_id\":%u,\"firmware\":\"%s\"}\n", 
        mesh.getNodeId(), FIRMWARE_VERSION);
    
    lastMetrics = millis();
    lastHeartbeat = millis();
}

void loop() {
    mesh.update();
    processSerial();
    
    // Send own metrics every 30 sec
    if (millis() - lastMetrics > METRICS_INTERVAL) {
        sendOwnMetrics();
        lastMetrics = millis();
    }
    
    // Heartbeat every 30 sec
    if (millis() - lastHeartbeat > HEARTBEAT_INTERVAL) {
        sendHeartbeat();
        lastHeartbeat = millis();
    }
}
