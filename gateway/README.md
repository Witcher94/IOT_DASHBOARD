# Raspberry Pi Mesh Gateway

Connects ESP32 Bridge (painlessMesh) to Cloud Backend via HTTP.

## Quick Start

### 1. Copy files to RPi
```bash
scp -r gateway/ pi@<RPI_IP>:/home/pi/
```

### 2. Install dependencies
```bash
ssh pi@<RPI_IP>
cd ~/gateway
pip3 install -r requirements.txt
```

### 3. Connect ESP32 Bridge
- Plug ESP32 (with bridge firmware) into RPi USB port
- Check port: `ls /dev/ttyUSB*` or `ls /dev/ttyACM*`

### 4. Test manually
```bash
python3 rpi_gateway.py --port /dev/ttyUSB0 --backend https://chnu-iot.com --debug
```

### 5. Install as service (auto-start)
```bash
sudo cp mesh-gateway.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable mesh-gateway
sudo systemctl start mesh-gateway
```

### 6. View logs
```bash
sudo journalctl -u mesh-gateway -f
```

## Architecture

```
[ESP Nodes] <--mesh--> [ESP32-Bridge] <--USB/Serial--> [RPi Gateway] <--HTTP--> [Backend]
```

## Commands

| Command | Description |
|---------|-------------|
| `--port` | Serial port (default: /dev/ttyUSB0) |
| `--baud` | Baud rate (default: 115200) |
| `--backend` | Backend URL |
| `--token` | Admin token for device registration |
| `--debug` | Enable debug logging |

## Node Registration

Mesh nodes need to be registered in the dashboard first.
When a new node appears, the gateway logs its info:
```
WARNING - Node 1234567890 not registered. Please add device manually in dashboard.
INFO - Node info: {'chip_id': 'ABC123', 'mac': 'AA:BB:CC:DD:EE:FF', ...}
```

Use this info to create the device in the web UI.

