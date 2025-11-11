#!/bin/bash

# UCXSync Installation Script for Orange Pi RV2 (Ubuntu Server 24.04)
# Run with: sudo ./install-orangepi.sh

set -e

INSTALL_DIR="/opt/ucxsync"
CONFIG_DIR="/etc/ucxsync"
LOG_DIR="/var/log/ucxsync"
MOUNT_DIR="/mnt/ucx"
BINARY_NAME="ucxsync"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}UCXSync Installation for Orange Pi RV2${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo -e "${RED}Error: This script must be run as root${NC}"
   echo "Please run: sudo $0"
   exit 1
fi

# Detect architecture
ARCH=$(uname -m)
echo -e "${YELLOW}Detected architecture: $ARCH${NC}"

if [[ "$ARCH" == "riscv64" ]]; then
    BINARY_SUFFIX="riscv64"
    echo -e "${GREEN}RISC-V 64-bit detected (Orange Pi RV2)${NC}"
elif [[ "$ARCH" == "aarch64" ]]; then
    BINARY_SUFFIX="arm64"
elif [[ "$ARCH" == "x86_64" ]]; then
    BINARY_SUFFIX="amd64"
else
    echo -e "${RED}Unsupported architecture: $ARCH${NC}"
    exit 1
fi

# Check Ubuntu version
if [ -f /etc/os-release ]; then
    . /etc/os-release
    echo -e "${YELLOW}OS: $NAME $VERSION${NC}"
    if [[ "$ID" != "ubuntu" ]]; then
        echo -e "${YELLOW}Warning: This script is optimized for Ubuntu Server 24.04${NC}"
        read -p "Continue anyway? (y/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
fi

# Update package list
echo -e "${GREEN}[1/8] Updating package list...${NC}"
apt-get update

# Install dependencies
echo -e "${GREEN}[2/8] Installing dependencies...${NC}"
apt-get install -y cifs-utils

# Create directories
echo -e "${GREEN}[3/8] Creating directories...${NC}"
mkdir -p "$INSTALL_DIR"
mkdir -p "$CONFIG_DIR"
mkdir -p "$LOG_DIR"
mkdir -p "$MOUNT_DIR"

# Build binary for detected architecture (if Go is installed)
if command -v go &> /dev/null; then
    echo -e "${GREEN}[4/8] Building UCXSync for $ARCH ($BINARY_SUFFIX)...${NC}"
    GOOS=linux GOARCH=$BINARY_SUFFIX go build -ldflags "-X main.Version=1.1.0 -X main.BuildTime=$(date -u '+%Y-%m-%d_%H:%M:%S')" -o "$INSTALL_DIR/$BINARY_NAME" ./cmd/ucxsync
else
    echo -e "${YELLOW}[4/8] Go not found. Please copy pre-built binary to $INSTALL_DIR/$BINARY_NAME${NC}"
    echo "You can build on another machine with: make build-$BINARY_SUFFIX"
    exit 1
fi

# Set permissions
echo -e "${GREEN}[5/8] Setting permissions...${NC}"
chmod +x "$INSTALL_DIR/$BINARY_NAME"
chown -R root:root "$INSTALL_DIR"
chmod 755 "$LOG_DIR"

# Copy configuration
echo -e "${GREEN}[6/8] Installing configuration...${NC}"
if [ -f "config.orangepi.yaml" ]; then
    cp config.orangepi.yaml "$CONFIG_DIR/config.yaml"
    echo -e "${YELLOW}Configuration copied. Please edit $CONFIG_DIR/config.yaml with your credentials${NC}"
else
    cp config.example.yaml "$CONFIG_DIR/config.yaml"
    echo -e "${YELLOW}Example configuration copied. Please edit $CONFIG_DIR/config.yaml${NC}"
fi

# Copy web assets
echo -e "${GREEN}[7/8] Copying web assets...${NC}"
cp -r web "$INSTALL_DIR/"

# Install systemd service
echo -e "${GREEN}[8/8] Installing systemd service...${NC}"
cat > /etc/systemd/system/ucxsync.service <<EOF
[Unit]
Description=UCXSync File Synchronization Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/$BINARY_NAME --config $CONFIG_DIR/config.yaml
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security settings
NoNewPrivileges=false
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=$LOG_DIR $MOUNT_DIR

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
systemctl daemon-reload

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Installation completed successfully!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo ""
echo "1. Edit configuration:"
echo "   sudo nano $CONFIG_DIR/config.yaml"
echo ""
echo "2. Test mount (optional):"
echo "   sudo $INSTALL_DIR/$BINARY_NAME mount"
echo ""
echo "3. Enable service to start on boot:"
echo "   sudo systemctl enable ucxsync"
echo ""
echo "4. Start the service:"
echo "   sudo systemctl start ucxsync"
echo ""
echo "5. Check status:"
echo "   sudo systemctl status ucxsync"
echo ""
echo "6. View logs:"
echo "   sudo journalctl -u ucxsync -f"
echo ""
echo "7. Access web interface:"
echo "   http://$(hostname -I | awk '{print $1}'):8080"
echo ""
echo -e "${YELLOW}Orange Pi optimization tips:${NC}"
echo "- Keep max_parallelism at 4 for better stability"
echo "- Monitor CPU temperature: cat /sys/class/thermal/thermal_zone0/temp"
echo "- Use external USB 3.0 drive for destination"
echo "- Consider active cooling for 24/7 operation"
echo ""
