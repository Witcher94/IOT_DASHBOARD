# Mesh Gateway (Go)

Compiled Go binary for Raspberry Pi - connects ESP32 Bridge (painlessMesh) to Cloud Backend.

## Quick Start

### 1. Build on your machine
```bash
cd gateway-go
chmod +x build.sh
./build.sh
```

### 2. Copy to Raspberry Pi
```bash
# For Raspberry Pi 4 (64-bit):
scp dist/mesh-gateway-linux-arm64 pi@<IP>:~/mesh-gateway

# For Raspberry Pi 3/Zero (32-bit):
scp dist/mesh-gateway-linux-arm pi@<IP>:~/mesh-gateway
```

### 3. Run on Raspberry Pi
```bash
ssh pi@<IP>

# Make executable
chmod +x mesh-gateway

# Run with debug
./mesh-gateway --port /dev/ttyUSB0 --backend https://chnu-iot.com --debug
```

### 4. Run as service (auto-start)
```bash
# Create service file
sudo nano /etc/systemd/system/mesh-gateway.service

# Paste:
[Unit]
Description=IoT Mesh Gateway
After=network.target

[Service]
Type=simple
User=pi
ExecStart=/home/pi/mesh-gateway --port /dev/ttyUSB0 --backend https://chnu-iot.com
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable mesh-gateway
sudo systemctl start mesh-gateway

# View logs
sudo journalctl -u mesh-gateway -f
```

## Command Line Options

| Option | Default | Description |
|--------|---------|-------------|
| `--port` | `/dev/ttyUSB0` | Serial port for ESP32 bridge |
| `--baud` | `115200` | Baud rate |
| `--backend` | `https://chnu-iot.com` | Backend URL |
| `--debug` | `false` | Enable debug logging |

## Node Token Registration

When a new mesh node connects, the gateway logs:
```
[WARN] No token for node 1234567890. Add device in dashboard and set token here.
[INFO] Node info: ChipID=ABC123, MAC=AA:BB:CC:DD:EE:FF, Platform=ESP32
```

To register:
1. Create device in web dashboard
2. Copy the device token
3. The gateway will match by ChipID/MAC automatically (or you can manually map node IDs to tokens in config)

## Architecture

```
[ESP Nodes] <--mesh--> [ESP32-Bridge] <--USB--> [RPi + mesh-gateway] <--HTTPS--> [Backend]
```

## Why Go instead of Python?

✅ Single compiled binary - no dependencies
✅ Better performance and memory usage
✅ Easy cross-compilation for ARM
✅ Type safety and reliability


