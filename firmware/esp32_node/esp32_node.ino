// ===== ESP32 Mesh Node v1.0.0 =====
// Sensor node for painlessMesh network
// Sends metrics to Bridge, receives commands

#include <painlessMesh.h>
#include <ArduinoJson.h>
#include <DHT.h>
#include <EEPROM.h>

// ---------------------------
// CONFIG
// ---------------------------
#define FIRMWARE_VERSION "node-esp32-1.0.0"

// Mesh settings (MUST MATCH BRIDGE!)
#define MESH_PREFIX     "iot_mesh_network"
#define MESH_PASSWORD   "mesh_secret_123"
#define MESH_PORT       5555
#define MESH_CHANNEL    6

// Hardware
#define DHTPIN          15
#define DHTTYPE         DHT22
#define LED_PIN         2
#define EEPROM_SIZE     256
#define CONFIG_MAGIC    0xDEAD0001

// ---------------------------
// CONFIG STRUCTURE
// ---------------------------
struct NodeConfig {
    uint32_t magic;
    char nodeName[32];
    uint32_t intervalSec;
    uint8_t dhtEnabled;
};

// ---------------------------
// GLOBALS
// ---------------------------
painlessMesh mesh;
Scheduler userScheduler;
DHT dht(DHTPIN, DHTTYPE);
NodeConfig cfg;

float lastTemp = NAN;
float lastHum = NAN;
bool ledState = false;

// ---------------------------
// CONFIG
// ---------------------------
void loadConfig() {
    EEPROM.begin(EEPROM_SIZE);
    EEPROM.get(0, cfg);
    
    if (cfg.magic != CONFIG_MAGIC) {
        // Defaults
        cfg.magic = CONFIG_MAGIC;
        strcpy(cfg.nodeName, "ESP32-Node");
        cfg.intervalSec = 30;
        cfg.dhtEnabled = 1;
        saveConfig();
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
// MESH DATA
// ---------------------------
String buildMetricsJson() {
    StaticJsonDocument<512> doc;
    
    doc["msg_type"] = "metrics";
    doc["node_name"] = cfg.nodeName;
    doc["node_id"] = mesh.getNodeId();
    
    // Sensors
    if (!isnan(lastTemp)) doc["temperature"] = lastTemp;
    if (!isnan(lastHum)) doc["humidity"] = lastHum;
    doc["dht_enabled"] = cfg.dhtEnabled ? true : false;
    
    // System
    JsonObject sys = doc.createNestedObject("system");
    sys["chip_id"] = String((uint32_t)ESP.getEfuseMac(), HEX);
    sys["mac"] = WiFi.macAddress();
    sys["firmware"] = FIRMWARE_VERSION;
    sys["platform"] = "ESP32";
    sys["free_heap"] = ESP.getFreeHeap();
    sys["uptime_ms"] = millis();
    
    // Mesh info
    JsonObject meshInfo = doc.createNestedObject("mesh");
    meshInfo["node_count"] = mesh.getNodeList().size() + 1;  // +1 for self
    meshInfo["rssi"] = WiFi.RSSI();
    
    String output;
    serializeJson(doc, output);
    return output;
}

void sendMetrics() {
    readSensors();
    String data = buildMetricsJson();
    mesh.sendBroadcast(data);  // Bridge will receive
    
    // Blink LED
    ledState = !ledState;
    digitalWrite(LED_PIN, ledState);
}

// ---------------------------
// COMMAND HANDLING
// ---------------------------
void handleCommand(JsonDocument &doc) {
    const char* cmd = doc["cmd"];
    
    if (!cmd) return;
    
    if (strcmp(cmd, "reboot") == 0) {
        Serial.println("[CMD] Reboot requested");
        delay(100);
        ESP.restart();
        
    } else if (strcmp(cmd, "set_interval") == 0) {
        uint32_t interval = doc["value"] | 30;
        cfg.intervalSec = constrain(interval, 10, 300);
        saveConfig();
        Serial.printf("[CMD] Interval set to %d sec\n", cfg.intervalSec);
        
    } else if (strcmp(cmd, "set_name") == 0) {
        const char* name = doc["value"];
        if (name) {
            strncpy(cfg.nodeName, name, 31);
            saveConfig();
            Serial.printf("[CMD] Name set to %s\n", cfg.nodeName);
        }
        
    } else if (strcmp(cmd, "toggle_dht") == 0) {
        cfg.dhtEnabled = !cfg.dhtEnabled;
        saveConfig();
        Serial.printf("[CMD] DHT %s\n", cfg.dhtEnabled ? "enabled" : "disabled");
        
    } else if (strcmp(cmd, "led_on") == 0) {
        digitalWrite(LED_PIN, HIGH);
        ledState = true;
        
    } else if (strcmp(cmd, "led_off") == 0) {
        digitalWrite(LED_PIN, LOW);
        ledState = false;
        
    } else if (strcmp(cmd, "status") == 0) {
        // Send immediate status
        sendMetrics();
        
    } else {
        Serial.printf("[CMD] Unknown: %s\n", cmd);
    }
}

// ---------------------------
// MESH CALLBACKS
// ---------------------------
void receivedCallback(uint32_t from, String &msg) {
    Serial.printf("[MESH] From %u: %s\n", from, msg.c_str());
    
    StaticJsonDocument<512> doc;
    DeserializationError err = deserializeJson(doc, msg);
    if (err) return;
    
    // Check if this is a command
    if (doc.containsKey("cmd")) {
        // Check if targeted at us or broadcast
        uint32_t target = doc["target"] | 0;
        if (target == 0 || target == mesh.getNodeId()) {
            handleCommand(doc);
        }
    }
}

void newConnectionCallback(uint32_t nodeId) {
    Serial.printf("[MESH] New connection: %u\n", nodeId);
}

void droppedConnectionCallback(uint32_t nodeId) {
    Serial.printf("[MESH] Dropped: %u\n", nodeId);
}

void changedConnectionCallback() {
    Serial.printf("[MESH] Connections changed. Nodes: %d\n", mesh.getNodeList().size());
}

// ---------------------------
// PERIODIC TASKS
// ---------------------------
Task taskMetrics(TASK_SECOND * 30, TASK_FOREVER, &sendMetrics);

// ---------------------------
// SETUP
// ---------------------------
void setup() {
    Serial.begin(115200);
    delay(500);
    
    Serial.println("\n\n=== ESP32 Mesh Node v" FIRMWARE_VERSION " ===");
    
    pinMode(LED_PIN, OUTPUT);
    digitalWrite(LED_PIN, LOW);
    
    loadConfig();
    Serial.printf("Node name: %s\n", cfg.nodeName);
    Serial.printf("Interval: %d sec\n", cfg.intervalSec);
    Serial.printf("DHT: %s\n", cfg.dhtEnabled ? "enabled" : "disabled");
    
    if (cfg.dhtEnabled) {
        dht.begin();
    }
    
    // Initialize mesh
    mesh.setDebugMsgTypes(ERROR | STARTUP);
    mesh.init(MESH_PREFIX, MESH_PASSWORD, &userScheduler, MESH_PORT, WIFI_AP_STA, MESH_CHANNEL);
    mesh.setContainsRoot(true);  // Network has a root node
    
    // Callbacks
    mesh.onReceive(&receivedCallback);
    mesh.onNewConnection(&newConnectionCallback);
    mesh.onDroppedConnection(&droppedConnectionCallback);
    mesh.onChangedConnections(&changedConnectionCallback);
    
    // Update task interval from config
    taskMetrics.setInterval(cfg.intervalSec * TASK_SECOND);
    userScheduler.addTask(taskMetrics);
    taskMetrics.enable();
    
    Serial.printf("Mesh Node ID: %u\n", mesh.getNodeId());
    Serial.println("[READY]");
}

// ---------------------------
// LOOP
// ---------------------------
void loop() {
    mesh.update();
    
    // Optional: Debug via Serial
    if (Serial.available()) {
        String cmd = Serial.readStringUntil('\n');
        cmd.trim();
        if (cmd == "status") {
            sendMetrics();
        } else if (cmd == "reboot") {
            ESP.restart();
        } else if (cmd == "nodes") {
            auto nodes = mesh.getNodeList();
            Serial.printf("Connected nodes: %d\n", nodes.size());
            for (auto &n : nodes) {
                Serial.printf("  - %u\n", n);
            }
        }
    }
}

