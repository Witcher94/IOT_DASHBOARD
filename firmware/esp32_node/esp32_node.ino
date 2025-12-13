// ===== ESP32 Mesh Node v1.1.0 =====
// Requires ESP32 Core 2.0.17!

#include <painlessMesh.h>
#include <ArduinoJson.h>
#include <DHT.h>
#include <EEPROM.h>

#define FIRMWARE_VERSION "node-1.1.0"

// MUST MATCH BRIDGE!
#define MESH_PREFIX   "iot_mesh"
#define MESH_PASSWORD "mesh12345"
#define MESH_PORT     5555
#define MESH_CHANNEL  6

// Hardware
#define DHTPIN       15
#define DHTTYPE      DHT22
#define LED_PIN      2
#define EEPROM_SIZE  128
#define CONFIG_MAGIC 0xCAFE0002

struct NodeConfig {
    uint32_t magic;
    char nodeName[32];
    uint32_t intervalSec;
    uint8_t dhtEnabled;
};

painlessMesh mesh;
DHT dht(DHTPIN, DHTTYPE);
NodeConfig cfg;

float lastTemp = NAN;
float lastHum = NAN;
unsigned long lastSend = 0;

void loadConfig() {
    EEPROM.begin(EEPROM_SIZE);
    EEPROM.get(0, cfg);
    if (cfg.magic != CONFIG_MAGIC) {
        cfg.magic = CONFIG_MAGIC;
        strcpy(cfg.nodeName, "ESP32-Node");
        cfg.intervalSec = 30;
        cfg.dhtEnabled = 1;
        EEPROM.put(0, cfg);
        EEPROM.commit();
    }
}

void readSensors() {
    if (cfg.dhtEnabled) {
        float t = dht.readTemperature();
        float h = dht.readHumidity();
        if (!isnan(t)) lastTemp = t;
        if (!isnan(h)) lastHum = h;
    }
}

void sendMetrics() {
    readSensors();
    
    StaticJsonDocument<384> doc;
    doc["msg_type"] = "metrics";
    doc["node_name"] = cfg.nodeName;
    doc["node_id"] = mesh.getNodeId();
    
    if (!isnan(lastTemp)) doc["temperature"] = lastTemp;
    if (!isnan(lastHum)) doc["humidity"] = lastHum;
    
    doc["chip_id"] = String((uint32_t)ESP.getEfuseMac(), HEX);
    doc["mac"] = WiFi.macAddress();
    doc["firmware"] = FIRMWARE_VERSION;
    doc["platform"] = "ESP32";
    doc["free_heap"] = ESP.getFreeHeap();
    doc["rssi"] = WiFi.RSSI();
    
    String output;
    serializeJson(doc, output);
    mesh.sendBroadcast(output);
    
    digitalWrite(LED_PIN, !digitalRead(LED_PIN));
    Serial.printf("[SEND] %s\n", output.c_str());
}

void handleCommand(JsonDocument &doc) {
    const char* cmd = doc["cmd"];
    if (!cmd) return;
    
    Serial.printf("[CMD] %s\n", cmd);
    
    if (strcmp(cmd, "reboot") == 0) {
        ESP.restart();
    } else if (strcmp(cmd, "status") == 0) {
        sendMetrics();
    } else if (strcmp(cmd, "set_name") == 0) {
        const char* name = doc["value"];
        if (name) {
            strncpy(cfg.nodeName, name, 31);
            EEPROM.put(0, cfg);
            EEPROM.commit();
        }
    } else if (strcmp(cmd, "toggle_dht") == 0) {
        cfg.dhtEnabled = !cfg.dhtEnabled;
        EEPROM.put(0, cfg);
        EEPROM.commit();
    }
}

void receivedCallback(uint32_t from, String &msg) {
    Serial.printf("[RX] %u: %s\n", from, msg.c_str());
    
    StaticJsonDocument<256> doc;
    if (deserializeJson(doc, msg)) return;
    
    if (doc.containsKey("cmd")) {
        uint32_t target = doc["target"] | 0;
        if (target == 0 || target == mesh.getNodeId()) {
            handleCommand(doc);
        }
    }
}

void newConnectionCallback(uint32_t nodeId) {
    Serial.printf("[+] Node %u connected\n", nodeId);
}

void droppedConnectionCallback(uint32_t nodeId) {
    Serial.printf("[-] Node %u disconnected\n", nodeId);
}

void setup() {
    Serial.begin(115200);
    delay(1000);
    
    Serial.printf("\n=== ESP32 Mesh Node %s ===\n", FIRMWARE_VERSION);
    
    pinMode(LED_PIN, OUTPUT);
    loadConfig();
    
    Serial.printf("Name: %s, Interval: %ds, DHT: %s\n", 
        cfg.nodeName, cfg.intervalSec, cfg.dhtEnabled ? "ON" : "OFF");
    
    if (cfg.dhtEnabled) dht.begin();
    
    mesh.setDebugMsgTypes(ERROR | STARTUP);
    mesh.init(MESH_PREFIX, MESH_PASSWORD, MESH_PORT, WIFI_AP_STA, MESH_CHANNEL);
    mesh.setContainsRoot(true);
    
    mesh.onReceive(&receivedCallback);
    mesh.onNewConnection(&newConnectionCallback);
    mesh.onDroppedConnection(&droppedConnectionCallback);
    
    Serial.printf("Node ID: %u\n", mesh.getNodeId());
    Serial.println("[READY]");
    
    lastSend = millis();
}

void loop() {
    mesh.update();
    
    if (millis() - lastSend > cfg.intervalSec * 1000) {
        sendMetrics();
        lastSend = millis();
    }
}
