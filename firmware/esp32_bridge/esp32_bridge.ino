// ===== ESP32 Mesh Bridge v1.1.0 =====
// Connects painlessMesh to Raspberry Pi via Serial
// SIMPLIFIED - no scheduler to avoid TCP conflicts

#include <painlessMesh.h>
#include <ArduinoJson.h>

// ---------------------------
// CONFIG
// ---------------------------
#define FIRMWARE_VERSION "bridge-1.1.0"

// Mesh settings
#define MESH_PREFIX     "iot_mesh_network"
#define MESH_PASSWORD   "mesh_secret_123"
#define MESH_PORT       5555
#define MESH_CHANNEL    6

// Serial to RPi
#define SERIAL_BAUD     115200

// ---------------------------
// GLOBALS
// ---------------------------
painlessMesh mesh;

// Stats
uint32_t nodeCount = 0;
uint32_t messagesReceived = 0;
uint32_t messagesSent = 0;
unsigned long lastHeartbeat = 0;

// ---------------------------
// SEND JSON TO SERIAL
// ---------------------------
void sendJson(const char* type, JsonDocument& doc) {
    doc["type"] = type;
    String output;
    serializeJson(doc, output);
    Serial.println(output);
}

// ---------------------------
// MESH CALLBACKS
// ---------------------------
void receivedCallback(uint32_t from, String &msg) {
    messagesReceived++;
    
    StaticJsonDocument<1024> wrapper;
    wrapper["type"] = "mesh_data";
    wrapper["from"] = from;
    
    // Try to parse as JSON
    StaticJsonDocument<768> incoming;
    if (deserializeJson(incoming, msg) == DeserializationError::Ok) {
        wrapper["data"] = incoming;
    } else {
        wrapper["data"] = msg;
    }
    
    String output;
    serializeJson(wrapper, output);
    Serial.println(output);
}

void newConnectionCallback(uint32_t nodeId) {
    nodeCount = mesh.getNodeList().size();
    
    StaticJsonDocument<256> doc;
    doc["type"] = "node_connected";
    doc["node_id"] = nodeId;
    doc["total_nodes"] = nodeCount;
    
    String output;
    serializeJson(doc, output);
    Serial.println(output);
}

void droppedConnectionCallback(uint32_t nodeId) {
    nodeCount = mesh.getNodeList().size();
    
    StaticJsonDocument<256> doc;
    doc["type"] = "node_disconnected";
    doc["node_id"] = nodeId;
    doc["total_nodes"] = nodeCount;
    
    String output;
    serializeJson(doc, output);
    Serial.println(output);
}

void changedConnectionCallback() {
    nodeCount = mesh.getNodeList().size();
}

// ---------------------------
// PROCESS SERIAL COMMANDS
// ---------------------------
void processSerial() {
    if (!Serial.available()) return;
    
    String cmd = Serial.readStringUntil('\n');
    cmd.trim();
    if (cmd.length() == 0) return;
    
    StaticJsonDocument<512> doc;
    if (deserializeJson(doc, cmd) != DeserializationError::Ok) {
        Serial.println("{\"type\":\"error\",\"msg\":\"Invalid JSON\"}");
        return;
    }
    
    const char* type = doc["type"];
    if (!type) return;
    
    if (strcmp(type, "broadcast") == 0) {
        String payload;
        serializeJson(doc["data"], payload);
        mesh.sendBroadcast(payload);
        messagesSent++;
        Serial.println("{\"type\":\"ack\",\"cmd\":\"broadcast\"}");
        
    } else if (strcmp(type, "send") == 0) {
        uint32_t target = doc["target"];
        String payload;
        serializeJson(doc["data"], payload);
        mesh.sendSingle(target, payload);
        messagesSent++;
        Serial.println("{\"type\":\"ack\",\"cmd\":\"send\"}");
        
    } else if (strcmp(type, "status") == 0) {
        sendHeartbeat();
    }
}

// ---------------------------
// HEARTBEAT
// ---------------------------
void sendHeartbeat() {
    StaticJsonDocument<512> doc;
    doc["type"] = "heartbeat";
    doc["node_id"] = mesh.getNodeId();
    doc["node_count"] = nodeCount;
    doc["messages_rx"] = messagesReceived;
    doc["messages_tx"] = messagesSent;
    doc["free_heap"] = ESP.getFreeHeap();
    doc["uptime_ms"] = millis();
    doc["firmware"] = FIRMWARE_VERSION;
    
    JsonArray nodes = doc.createNestedArray("nodes");
    for (auto &node : mesh.getNodeList()) {
        nodes.add(node);
    }
    
    String output;
    serializeJson(doc, output);
    Serial.println(output);
}

// ---------------------------
// SETUP
// ---------------------------
void setup() {
    Serial.begin(SERIAL_BAUD);
    delay(2000);  // Wait for serial
    
    Serial.println();
    Serial.println("{\"type\":\"boot\",\"msg\":\"ESP32 Mesh Bridge starting\",\"firmware\":\"" FIRMWARE_VERSION "\"}");
    
    // Disable debug to reduce conflicts
    mesh.setDebugMsgTypes(ERROR);
    
    // Initialize mesh WITHOUT scheduler
    mesh.init(MESH_PREFIX, MESH_PASSWORD, MESH_PORT, WIFI_AP_STA, MESH_CHANNEL);
    
    mesh.setRoot(true);
    mesh.setContainsRoot(true);
    
    mesh.onReceive(&receivedCallback);
    mesh.onNewConnection(&newConnectionCallback);
    mesh.onDroppedConnection(&droppedConnectionCallback);
    mesh.onChangedConnections(&changedConnectionCallback);
    
    // Ready
    StaticJsonDocument<256> ready;
    ready["type"] = "ready";
    ready["node_id"] = mesh.getNodeId();
    ready["mesh_prefix"] = MESH_PREFIX;
    ready["channel"] = MESH_CHANNEL;
    ready["firmware"] = FIRMWARE_VERSION;
    
    String output;
    serializeJson(ready, output);
    Serial.println(output);
    
    lastHeartbeat = millis();
}

// ---------------------------
// LOOP
// ---------------------------
void loop() {
    mesh.update();
    
    processSerial();
    
    // Heartbeat every 30 seconds
    if (millis() - lastHeartbeat > 30000) {
        sendHeartbeat();
        lastHeartbeat = millis();
    }
}
