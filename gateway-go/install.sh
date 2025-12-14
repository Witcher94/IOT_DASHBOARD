#!/bin/bash
# Mesh Gateway Installer for Raspberry Pi
# Usage: curl -sSL https://your-url/install.sh | sudo bash

set -e

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/mesh-gateway"
SERVICE_FILE="/etc/systemd/system/mesh-gateway.service"
BINARY_NAME="mesh-gateway"

echo "╔══════════════════════════════════════════════════════════╗"
echo "║        🌐 Mesh Gateway Installer v2.0                    ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# Check root
if [ "$EUID" -ne 0 ]; then
    echo "❌ Please run as root: sudo $0"
    exit 1
fi

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    armv7l|armhf) ARCH="arm" ;;
    aarch64|arm64) ARCH="arm64" ;;
    x86_64) ARCH="amd64" ;;
    *) echo "❌ Unsupported architecture: $ARCH"; exit 1 ;;
esac
echo "✓ Detected architecture: $ARCH"

# Check if binary exists in current directory
if [ -f "./$BINARY_NAME" ]; then
    BINARY_PATH="./$BINARY_NAME"
elif [ -f "./$BINARY_NAME-linux-$ARCH" ]; then
    BINARY_PATH="./$BINARY_NAME-linux-$ARCH"
else
    echo "❌ Binary not found. Please build first: ./build.sh"
    exit 1
fi

echo "✓ Found binary: $BINARY_PATH"

# Stop existing service
if systemctl is-active --quiet mesh-gateway; then
    echo "⏹ Stopping existing service..."
    systemctl stop mesh-gateway
fi

# Install binary
echo "📦 Installing binary to $INSTALL_DIR..."
cp "$BINARY_PATH" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Create config directory
echo "📁 Creating config directory..."
mkdir -p "$CONFIG_DIR"

# Copy config if not exists
if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
    echo "📝 Creating default config..."
    if [ -f "./config.yaml" ]; then
        cp ./config.yaml "$CONFIG_DIR/config.yaml"
    else
        cat > "$CONFIG_DIR/config.yaml" << 'EOF'
# Mesh Gateway Configuration

serial:
  port: /dev/ttyUSB0
  baud: 115200

backend:
  url: https://chnu-iot.com
  token: ""  # REQUIRED: Set your gateway token here!
  batch_interval: 30

web:
  port: 8080
  enabled: true

logging:
  level: info
  file: ""

nodes:
  timeout: 120
  auto_register: true
EOF
    fi
    echo "⚠️  IMPORTANT: Edit /etc/mesh-gateway/config.yaml and set your token!"
else
    echo "✓ Config already exists, skipping..."
fi

# Install service
echo "🔧 Installing systemd service..."
cat > "$SERVICE_FILE" << 'EOF'
[Unit]
Description=IoT Mesh Gateway - Bridge ESP32 mesh to cloud
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple
User=root
Group=dialout
ExecStart=/usr/local/bin/mesh-gateway --config /etc/mesh-gateway/config.yaml
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=mesh-gateway

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
systemctl daemon-reload

# Enable service
echo "✓ Enabling service to start on boot..."
systemctl enable mesh-gateway

# Add user to dialout group
if [ -n "$SUDO_USER" ]; then
    usermod -a -G dialout "$SUDO_USER"
    echo "✓ Added $SUDO_USER to dialout group"
fi

echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║  ✅ Installation Complete!                               ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
echo "📋 Next steps:"
echo ""
echo "1. Edit config file:"
echo "   sudo nano /etc/mesh-gateway/config.yaml"
echo ""
echo "2. Set your gateway token in the config"
echo ""
echo "3. Start the service:"
echo "   sudo systemctl start mesh-gateway"
echo ""
echo "4. Check status:"
echo "   sudo systemctl status mesh-gateway"
echo "   sudo journalctl -u mesh-gateway -f"
echo ""
echo "5. Access Web UI:"
echo "   http://$(hostname -I | awk '{print $1}'):8080"
echo ""


