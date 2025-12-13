// ===== ESP32 Mesh Bridge v1.3.0 =====
// painlessMesh + Serial to RPi Gateway
// Requires ESP32 Core 2.0.17!

#include <painlessMesh.h>
#include <ArduinoJson.h>

#define FIRMWARE_VERSION "bridge-1.3.0"
#define MESH_PREFIX   "iot_mesh"
#define MESH_PASSWORD "mesh12345"
#define MESH_PORT     5555
#define MESH_CHANNEL  6
#define SERIAL_BAUD   115200

painlessMesh mesh;
uint32_t nodeCount = 0;
unsigned long lastHeartbeat = 0;

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
    Serial.printf("{\"type\":\"heartbeat\",\"node_id\":%u,\"nodes\":%u,\"heap\":%u,\"uptime\":%lu}\n",
        mesh.getNodeId(), nodeCount, ESP.getFreeHeap(), millis());
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
        String payload;
        serializeJson(doc["data"], payload);
        mesh.sendBroadcast(payload);
        Serial.println("{\"type\":\"ack\",\"cmd\":\"broadcast\"}");
    } else if (strcmp(type, "send") == 0) {
        uint32_t target = doc["target"];
        String payload;
        serializeJson(doc["data"], payload);
        mesh.sendSingle(target, payload);
        Serial.println("{\"type\":\"ack\",\"cmd\":\"send\"}");
    } else if (strcmp(type, "status") == 0) {
        sendHeartbeat();
    }
}

void setup() {
    Serial.begin(SERIAL_BAUD);
    delay(1000);
    
    mesh.setDebugMsgTypes(ERROR | STARTUP);
    mesh.init(MESH_PREFIX, MESH_PASSWORD, MESH_PORT, WIFI_AP, MESH_CHANNEL);
    mesh.setRoot(true);
    mesh.setContainsRoot(true);
    
    mesh.onReceive(&receivedCallback);
    mesh.onNewConnection(&newConnectionCallback);
    mesh.onDroppedConnection(&droppedConnectionCallback);
    
    Serial.printf("{\"type\":\"ready\",\"node_id\":%u,\"firmware\":\"%s\"}\n", 
        mesh.getNodeId(), FIRMWARE_VERSION);
    
    lastHeartbeat = millis();
}

void loop() {
    mesh.update();
    processSerial();
    
    if (millis() - lastHeartbeat > 30000) {
        sendHeartbeat();
        lastHeartbeat = millis();
    }
}
