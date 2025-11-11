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

# Create mount points for UCX nodes (network shares)
mkdir -p "$MOUNT_DIR"
echo -e "${GREEN}✓${NC} Created $MOUNT_DIR (UCX network mount points)"

# Create storage directory for USB-SSD
mkdir -p /mnt/storage
echo -e "${GREEN}✓${NC} Created /mnt/storage (USB-SSD mount point)"

# Check if USB-SSD is already mounted
if mountpoint -q /mnt/storage; then
    echo -e "${GREEN}✓${NC} /mnt/storage is already mounted"
    
    # Set permissions for user access
    USER_NAME=${SUDO_USER:-$(whoami)}
    chown -R $USER_NAME:$USER_NAME /mnt/storage 2>/dev/null || true
    echo -e "${GREEN}✓${NC} Permissions set for /mnt/storage"
else
    echo -e "${YELLOW}⚠${NC}  /mnt/storage is not mounted"
    echo -e "${YELLOW}⚠${NC}  You need to mount your USB-SSD to /mnt/storage"
    echo ""
    echo "Option 1 - Manual mount:"
    echo "  1. Find your device: lsblk"
    echo "  2. Mount it: sudo mount /dev/sdX1 /mnt/storage"
    echo ""
    echo "Option 2 - Auto-mount (recommended):"
    echo "  Run: sudo ./setup-usb-automount.sh"
    echo ""
fi

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

# Check USB-SSD status
if ! mountpoint -q /mnt/storage; then
    echo -e "${RED}⚠ WARNING: USB-SSD is NOT mounted!${NC}"
    echo ""
    echo "UCXSync requires an external USB-SSD mounted at /mnt/storage"
    echo ""
    echo -e "${YELLOW}Quick setup:${NC}"
    echo "  1. Connect your USB-SSD"
    echo "  2. Find device:    lsblk"
    echo "  3. Mount:          sudo mount /dev/sdX1 /mnt/storage"
    echo "  4. Create dir:     sudo mkdir -p /mnt/storage/ucx"
    echo "  5. Set owner:      sudo chown -R \$USER:\$USER /mnt/storage/ucx"
    echo ""
else
    echo -e "${GREEN}✓ USB-SSD is mounted at /mnt/storage${NC}"
    STORAGE_INFO=$(df -h /mnt/storage 2>/dev/null | tail -1 | awk '{print $2 " total, " $4 " free"}')
    echo -e "${YELLOW}Storage:${NC} $STORAGE_INFO"
    echo ""
fi

echo -e "${YELLOW}Next steps:${NC}"
echo ""
echo "1. Edit configuration:"
echo "   sudo nano $CONFIG_DIR/config.yaml"
echo ""
echo "   Update these settings:"
echo "   - sync.project (your project name)"
echo "   - sync.destination (/mnt/storage)"
echo "   - credentials.username and password"
echo ""
echo "2. Setup USB-SSD auto-mount (recommended):"
echo "   sudo ./setup-usb-automount.sh"
echo ""
echo "3. Enable service to start on boot:"
echo "   sudo systemctl enable ucxsync"
echo ""
echo "3. Start the service:"
echo "   sudo systemctl start ucxsync"
echo ""
echo "4. Check status:"
echo "   sudo systemctl status ucxsync"
echo ""
echo "5. View logs:"
echo "   sudo journalctl -u ucxsync -f"
echo ""
echo "6. Access web interface:"
echo "   http://$(hostname -I | awk '{print $1}'):8080"
echo ""
echo -e "${YELLOW}Orange Pi RV2 optimization tips:${NC}"
echo "- Keep max_parallelism at 3-4 for RISC-V (see config.orangepi.yaml)"
echo "- Monitor CPU temperature: cat /sys/class/thermal/thermal_zone0/temp"
echo "- Use USB 3.0 SSD for best performance (/mnt/storage)"
echo "- Consider active cooling for 24/7 operation"
echo ""
echo -e "${YELLOW}Documentation:${NC}"
echo "- Orange Pi guide:    ORANGEPI.md"
echo "- USB-SSD setup:      USB-SSD-GUIDE.md"
echo "- Storage explained:  STORAGE-ARCHITECTURE.md"
echo ""
