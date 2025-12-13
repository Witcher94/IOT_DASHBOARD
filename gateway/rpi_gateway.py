#!/usr/bin/env python3
"""
Raspberry Pi Mesh Gateway v1.0.0

Connects ESP32 Bridge (via Serial) to Cloud Backend (via HTTP).
Bidirectional: 
  - Mesh metrics -> Cloud
  - Cloud commands -> Mesh

Usage:
    python3 rpi_gateway.py --port /dev/ttyUSB0 --backend https://chnu-iot.com

Requirements:
    pip3 install pyserial requests python-dotenv
"""

import argparse
import json
import logging
import os
import signal
import sys
import threading
import time
from collections import deque
from datetime import datetime
from typing import Optional

import requests
import serial

# ---------------------------
# CONFIGURATION
# ---------------------------
DEFAULT_SERIAL_PORT = "/dev/ttyUSB0"
DEFAULT_BAUD_RATE = 115200
DEFAULT_BACKEND_URL = "https://chnu-iot.com"
COMMAND_POLL_INTERVAL = 5  # seconds
RECONNECT_DELAY = 5  # seconds
MAX_BUFFER_SIZE = 1000  # max buffered messages if backend is down

# ---------------------------
# LOGGING
# ---------------------------
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S"
)
logger = logging.getLogger(__name__)

# ---------------------------
# GATEWAY CLASS
# ---------------------------
class MeshGateway:
    def __init__(
        self,
        serial_port: str,
        baud_rate: int,
        backend_url: str,
        gateway_token: str
    ):
        self.serial_port = serial_port
        self.baud_rate = baud_rate
        self.backend_url = backend_url.rstrip("/")
        self.gateway_token = gateway_token
        
        self.serial: Optional[serial.Serial] = None
        self.running = False
        self.buffer = deque(maxlen=MAX_BUFFER_SIZE)
        
        # Node ID to Device Token mapping (loaded from backend)
        self.node_tokens = {}
        
        # Stats
        self.stats = {
            "messages_received": 0,
            "messages_sent_to_backend": 0,
            "commands_sent_to_mesh": 0,
            "errors": 0,
            "start_time": None
        }
    
    # ---------------------------
    # SERIAL CONNECTION
    # ---------------------------
    def connect_serial(self) -> bool:
        """Connect to ESP32 Bridge via Serial"""
        try:
            self.serial = serial.Serial(
                port=self.serial_port,
                baudrate=self.baud_rate,
                timeout=1
            )
            logger.info(f"Serial connected: {self.serial_port} @ {self.baud_rate}")
            return True
        except serial.SerialException as e:
            logger.error(f"Serial connection failed: {e}")
            return False
    
    def disconnect_serial(self):
        """Disconnect Serial"""
        if self.serial and self.serial.is_open:
            self.serial.close()
            logger.info("Serial disconnected")
    
    # ---------------------------
    # HTTP TO BACKEND
    # ---------------------------
    def send_to_backend(self, node_id: int, data: dict) -> bool:
        """Send metrics from mesh node to backend"""
        try:
            # Find device token by node_id
            token = self.node_tokens.get(str(node_id))
            if not token:
                logger.warning(f"No token for node {node_id}, registering...")
                token = self.register_node(node_id, data)
                if not token:
                    return False
            
            # Transform mesh data to backend format
            payload = self.transform_metrics(data)
            
            url = f"{self.backend_url}/api/v1/metrics"
            headers = {
                "Content-Type": "application/json",
                "X-Device-Token": token
            }
            
            response = requests.post(url, json=payload, headers=headers, timeout=10)
            
            if response.status_code == 200:
                self.stats["messages_sent_to_backend"] += 1
                logger.debug(f"Metrics sent for node {node_id}")
                return True
            elif response.status_code == 401:
                logger.warning(f"Invalid token for node {node_id}")
                # Remove bad token, will re-register
                self.node_tokens.pop(str(node_id), None)
                return False
            else:
                logger.error(f"Backend error: {response.status_code}")
                return False
                
        except requests.RequestException as e:
            logger.error(f"HTTP error: {e}")
            self.stats["errors"] += 1
            return False
    
    def transform_metrics(self, data: dict) -> dict:
        """Transform mesh message to backend format"""
        payload = {
            "node_name": data.get("node_name", "Unknown"),
            "temperature": data.get("temperature"),
            "humidity": data.get("humidity"),
            "dht_enabled": data.get("dht_enabled", True)
        }
        
        # System info
        if "system" in data:
            payload["system"] = data["system"]
        
        # Current WiFi (mesh info)
        if "mesh" in data:
            payload["current_wifi"] = {
                "ssid": "mesh_network",
                "rssi": data["mesh"].get("rssi", 0),
                "bssid": "",
                "ip": "",
                "channel": 6
            }
        
        # Mesh status
        payload["mesh_status"] = {
            "enabled": True,
            "running": True,
            "node_id": data.get("node_id", 0),
            "node_count": data.get("mesh", {}).get("node_count", 0)
        }
        
        return payload
    
    def register_node(self, node_id: int, data: dict) -> Optional[str]:
        """Register new mesh node with backend (requires admin token)"""
        # This would require admin access to create device
        # For now, log and skip
        logger.warning(f"Node {node_id} not registered. Please add device manually in dashboard.")
        logger.info(f"Node info: {data.get('system', {})}")
        return None
    
    # ---------------------------
    # COMMAND POLLING
    # ---------------------------
    def poll_commands(self):
        """Poll backend for pending commands"""
        # For each registered node, check pending commands
        for node_id, token in self.node_tokens.items():
            try:
                url = f"{self.backend_url}/api/v1/devices/commands/pending"
                headers = {
                    "Authorization": f"Bearer {self.gateway_token}",
                    "X-Device-Token": token
                }
                
                response = requests.get(url, headers=headers, timeout=5)
                
                if response.status_code == 200:
                    commands = response.json()
                    for cmd in commands:
                        self.send_command_to_mesh(int(node_id), cmd)
                        
            except requests.RequestException as e:
                logger.debug(f"Command poll failed: {e}")
    
    def send_command_to_mesh(self, node_id: int, command: dict):
        """Send command to mesh node via bridge"""
        if not self.serial or not self.serial.is_open:
            return
        
        try:
            # Build mesh command
            mesh_cmd = {
                "type": "send",
                "target": node_id,
                "data": {
                    "cmd": command.get("type"),
                    "value": command.get("payload"),
                    "cmd_id": command.get("id")
                }
            }
            
            line = json.dumps(mesh_cmd) + "\n"
            self.serial.write(line.encode())
            self.stats["commands_sent_to_mesh"] += 1
            logger.info(f"Command sent to node {node_id}: {command.get('type')}")
            
        except Exception as e:
            logger.error(f"Failed to send command: {e}")
    
    # ---------------------------
    # SERIAL READER
    # ---------------------------
    def read_serial(self):
        """Read and process messages from bridge"""
        while self.running:
            try:
                if not self.serial or not self.serial.is_open:
                    time.sleep(RECONNECT_DELAY)
                    self.connect_serial()
                    continue
                
                line = self.serial.readline()
                if not line:
                    continue
                
                try:
                    line = line.decode("utf-8").strip()
                    if not line:
                        continue
                    
                    data = json.loads(line)
                    self.process_message(data)
                    
                except json.JSONDecodeError:
                    logger.debug(f"Non-JSON: {line}")
                except UnicodeDecodeError:
                    logger.debug("Unicode decode error")
                    
            except serial.SerialException as e:
                logger.error(f"Serial error: {e}")
                self.disconnect_serial()
                time.sleep(RECONNECT_DELAY)
            except Exception as e:
                logger.error(f"Reader error: {e}")
                time.sleep(1)
    
    def process_message(self, msg: dict):
        """Process message from bridge"""
        msg_type = msg.get("type")
        self.stats["messages_received"] += 1
        
        if msg_type == "mesh_data":
            # Data from mesh node
            from_node = msg.get("from")
            data = msg.get("data", {})
            
            # Only process metrics
            if isinstance(data, dict) and data.get("msg_type") == "metrics":
                logger.info(f"Metrics from node {from_node}: T={data.get('temperature')}, H={data.get('humidity')}")
                self.send_to_backend(from_node, data)
        
        elif msg_type == "node_connected":
            node_id = msg.get("node_id")
            total = msg.get("total_nodes")
            logger.info(f"Node connected: {node_id} (total: {total})")
        
        elif msg_type == "node_disconnected":
            node_id = msg.get("node_id")
            total = msg.get("total_nodes")
            logger.warning(f"Node disconnected: {node_id} (total: {total})")
        
        elif msg_type == "heartbeat":
            nodes = msg.get("node_count", 0)
            heap = msg.get("free_heap", 0)
            logger.debug(f"Bridge heartbeat: {nodes} nodes, {heap} bytes free")
        
        elif msg_type == "ready":
            logger.info(f"Bridge ready: {msg.get('firmware')} (ID: {msg.get('node_id')})")
        
        elif msg_type == "boot":
            logger.info(f"Bridge booting: {msg.get('msg')}")
        
        elif msg_type == "ack":
            logger.debug(f"Command acknowledged: {msg.get('cmd')}")
        
        elif msg_type == "error":
            logger.error(f"Bridge error: {msg.get('msg')}")
        
        else:
            logger.debug(f"Unknown message type: {msg_type}")
    
    # ---------------------------
    # COMMAND POLLER THREAD
    # ---------------------------
    def command_poller(self):
        """Background thread to poll commands"""
        while self.running:
            time.sleep(COMMAND_POLL_INTERVAL)
            if self.node_tokens:
                self.poll_commands()
    
    # ---------------------------
    # MAIN RUN
    # ---------------------------
    def run(self):
        """Main entry point"""
        self.running = True
        self.stats["start_time"] = datetime.now()
        
        logger.info("=" * 50)
        logger.info("Mesh Gateway Starting")
        logger.info(f"Serial: {self.serial_port}")
        logger.info(f"Backend: {self.backend_url}")
        logger.info("=" * 50)
        
        # Connect serial
        if not self.connect_serial():
            logger.error("Initial serial connection failed")
        
        # Start command poller thread
        poller_thread = threading.Thread(target=self.command_poller, daemon=True)
        poller_thread.start()
        
        # Main serial reader loop
        try:
            self.read_serial()
        except KeyboardInterrupt:
            logger.info("Shutting down...")
        finally:
            self.running = False
            self.disconnect_serial()
            self.print_stats()
    
    def print_stats(self):
        """Print runtime stats"""
        uptime = datetime.now() - self.stats["start_time"]
        logger.info("=" * 50)
        logger.info("Gateway Statistics")
        logger.info(f"Uptime: {uptime}")
        logger.info(f"Messages received: {self.stats['messages_received']}")
        logger.info(f"Sent to backend: {self.stats['messages_sent_to_backend']}")
        logger.info(f"Commands to mesh: {self.stats['commands_sent_to_mesh']}")
        logger.info(f"Errors: {self.stats['errors']}")
        logger.info("=" * 50)


# ---------------------------
# CLI
# ---------------------------
def main():
    parser = argparse.ArgumentParser(description="Mesh Gateway for Raspberry Pi")
    parser.add_argument(
        "--port", "-p",
        default=os.getenv("SERIAL_PORT", DEFAULT_SERIAL_PORT),
        help=f"Serial port (default: {DEFAULT_SERIAL_PORT})"
    )
    parser.add_argument(
        "--baud", "-b",
        type=int,
        default=int(os.getenv("BAUD_RATE", DEFAULT_BAUD_RATE)),
        help=f"Baud rate (default: {DEFAULT_BAUD_RATE})"
    )
    parser.add_argument(
        "--backend", "-u",
        default=os.getenv("BACKEND_URL", DEFAULT_BACKEND_URL),
        help=f"Backend URL (default: {DEFAULT_BACKEND_URL})"
    )
    parser.add_argument(
        "--token", "-t",
        default=os.getenv("GATEWAY_TOKEN", ""),
        help="Gateway authentication token"
    )
    parser.add_argument(
        "--debug", "-d",
        action="store_true",
        help="Enable debug logging"
    )
    
    args = parser.parse_args()
    
    if args.debug:
        logging.getLogger().setLevel(logging.DEBUG)
    
    gateway = MeshGateway(
        serial_port=args.port,
        baud_rate=args.baud,
        backend_url=args.backend,
        gateway_token=args.token
    )
    
    # Handle signals
    def signal_handler(sig, frame):
        logger.info("Received shutdown signal")
        gateway.running = False
    
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    gateway.run()


if __name__ == "__main__":
    main()

