// ===== ESP32 Mesh Bridge v1.0.0 =====
// Connects painlessMesh to Raspberry Pi via Serial
// Role: Mesh Root + Serial Gateway

#include <painlessMesh.h>
#include <ArduinoJson.h>

// ---------------------------
// CONFIG
// ---------------------------
#define FIRMWARE_VERSION "bridge-1.0.0"

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
Scheduler userScheduler;

// Stats
uint32_t nodeCount = 0;
uint32_t messagesReceived = 0;
uint32_t messagesSent = 0;

// ---------------------------
// MESH CALLBACKS
// ---------------------------
void receivedCallback(uint32_t from, String &msg) {
    messagesReceived++;
    
    // Forward to RPi via Serial
    // Format: {"from": nodeId, "data": {...}}
    StaticJsonDocument<1024> wrapper;
    wrapper["type"] = "mesh_data";
    wrapper["from"] = from;
    wrapper["timestamp"] = mesh.getNodeTime();
    
    // Parse incoming message
    StaticJsonDocument<768> incoming;
    DeserializationError err = deserializeJson(incoming, msg);
    if (err) {
        wrapper["data"] = msg;  // Raw string if not JSON
    } else {
        wrapper["data"] = incoming;
    }
    
    // Send to RPi
    String output;
    serializeJson(wrapper, output);
    Serial.println(output);
}

void newConnectionCallback(uint32_t nodeId) {
    nodeCount = mesh.getNodeList().size();
    
    // Notify RPi
    StaticJsonDocument<256> doc;
    doc["type"] = "node_connected";
    doc["node_id"] = nodeId;
    doc["total_nodes"] = nodeCount;
    doc["timestamp"] = mesh.getNodeTime();
    
    String output;
    serializeJson(doc, output);
    Serial.println(output);
}

void droppedConnectionCallback(uint32_t nodeId) {
    nodeCount = mesh.getNodeList().size();
    
    // Notify RPi
    StaticJsonDocument<256> doc;
    doc["type"] = "node_disconnected";
    doc["node_id"] = nodeId;
    doc["total_nodes"] = nodeCount;
    doc["timestamp"] = mesh.getNodeTime();
    
    String output;
    serializeJson(doc, output);
    Serial.println(output);
}

void changedConnectionCallback() {
    nodeCount = mesh.getNodeList().size();
}

// ---------------------------
// SERIAL COMMANDS FROM RPI
// ---------------------------
void processSerialCommand(String &cmd) {
    StaticJsonDocument<512> doc;
    DeserializationError err = deserializeJson(doc, cmd);
    
    if (err) {
        Serial.println("{\"type\":\"error\",\"msg\":\"Invalid JSON\"}");
        return;
    }
    
    const char* type = doc["type"];
    
    if (strcmp(type, "broadcast") == 0) {
        // Broadcast to all nodes
        String payload;
        serializeJson(doc["data"], payload);
        mesh.sendBroadcast(payload);
        messagesSent++;
        Serial.println("{\"type\":\"ack\",\"cmd\":\"broadcast\"}");
        
    } else if (strcmp(type, "send") == 0) {
        // Send to specific node
        uint32_t target = doc["target"];
        String payload;
        serializeJson(doc["data"], payload);
        mesh.sendSingle(target, payload);
        messagesSent++;
        Serial.println("{\"type\":\"ack\",\"cmd\":\"send\"}");
        
    } else if (strcmp(type, "status") == 0) {
        // Report bridge status
        StaticJsonDocument<512> status;
        status["type"] = "bridge_status";
        status["node_id"] = mesh.getNodeId();
        status["node_count"] = nodeCount;
        status["messages_rx"] = messagesReceived;
        status["messages_tx"] = messagesSent;
        status["free_heap"] = ESP.getFreeHeap();
        status["uptime_ms"] = millis();
        status["firmware"] = FIRMWARE_VERSION;
        
        // List connected nodes
        JsonArray nodes = status.createNestedArray("nodes");
        auto nodeList = mesh.getNodeList();
        for (auto &node : nodeList) {
            nodes.add(node);
        }
        
        String output;
        serializeJson(status, output);
        Serial.println(output);
        
    } else if (strcmp(type, "get_nodes") == 0) {
        // List all nodes
        StaticJsonDocument<1024> response;
        response["type"] = "node_list";
        JsonArray nodes = response.createNestedArray("nodes");
        auto nodeList = mesh.getNodeList();
        for (auto &node : nodeList) {
            nodes.add(node);
        }
        response["count"] = nodeList.size();
        
        String output;
        serializeJson(response, output);
        Serial.println(output);
        
    } else {
        Serial.println("{\"type\":\"error\",\"msg\":\"Unknown command\"}");
    }
}

// ---------------------------
// PERIODIC STATUS
// ---------------------------
Task taskStatus(30000, TASK_FOREVER, []() {
    StaticJsonDocument<256> doc;
    doc["type"] = "heartbeat";
    doc["node_id"] = mesh.getNodeId();
    doc["node_count"] = nodeCount;
    doc["free_heap"] = ESP.getFreeHeap();
    doc["uptime_ms"] = millis();
    
    String output;
    serializeJson(doc, output);
    Serial.println(output);
});

// ---------------------------
// SETUP
// ---------------------------
void setup() {
    Serial.begin(SERIAL_BAUD);
    delay(1000);
    
    // Startup message
    Serial.println();
    Serial.println("{\"type\":\"boot\",\"msg\":\"ESP32 Mesh Bridge starting\",\"firmware\":\"" FIRMWARE_VERSION "\"}");
    
    // Initialize mesh
    mesh.setDebugMsgTypes(ERROR | STARTUP);
    mesh.init(MESH_PREFIX, MESH_PASSWORD, &userScheduler, MESH_PORT, WIFI_AP_STA, MESH_CHANNEL);
    
    // This node is the root (connected to RPi)
    mesh.setRoot(true);
    mesh.setContainsRoot(true);
    
    // Set callbacks
    mesh.onReceive(&receivedCallback);
    mesh.onNewConnection(&newConnectionCallback);
    mesh.onDroppedConnection(&droppedConnectionCallback);
    mesh.onChangedConnections(&changedConnectionCallback);
    
    // Start periodic status
    userScheduler.addTask(taskStatus);
    taskStatus.enable();
    
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
}

// ---------------------------
// LOOP
// ---------------------------
void loop() {
    mesh.update();
    
    // Check for Serial commands from RPi
    if (Serial.available()) {
        String cmd = Serial.readStringUntil('\n');
        cmd.trim();
        if (cmd.length() > 0) {
            processSerialCommand(cmd);
        }
    }
}

