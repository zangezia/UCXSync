#!/bin/bash
#
# UCXSync Installation Script for Linux
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}UCXSync Installation${NC}"
echo "======================================"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    echo "Please run: sudo ./install.sh"
    exit 1
fi

# Check prerequisites
echo "Checking prerequisites..."

# Check Go
if ! command -v go &> /dev/null; then
    echo -e "${YELLOW}Warning: Go is not installed${NC}"
    echo "Install Go from: https://go.dev/dl/"
    echo "Or run: wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz && tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz"
    exit 1
fi

# Check cifs-utils
if ! dpkg -l | grep -q cifs-utils; then
    echo -e "${YELLOW}Installing cifs-utils...${NC}"
    apt-get update
    apt-get install -y cifs-utils
fi

echo -e "${GREEN}✓${NC} Prerequisites satisfied"

# Build application
echo ""
echo "Building UCXSync..."
go build -o ucxsync ./cmd/ucxsync
echo -e "${GREEN}✓${NC} Build complete"

# Create directories
echo ""
echo "Creating directories..."
mkdir -p /opt/ucxsync
mkdir -p /etc/ucxsync
mkdir -p /var/log/ucxsync
mkdir -p /mnt/ucx
echo -e "${GREEN}✓${NC} Directories created"

# Install binary
echo ""
echo "Installing binary..."
cp ucxsync /opt/ucxsync/
chmod +x /opt/ucxsync/ucxsync
echo -e "${GREEN}✓${NC} Binary installed to /opt/ucxsync/ucxsync"

# Install web assets
echo ""
echo "Installing web assets..."
cp -r web /opt/ucxsync/
echo -e "${GREEN}✓${NC} Web assets installed to /opt/ucxsync/web"

# Install config
if [ ! -f /etc/ucxsync/config.yaml ]; then
    echo ""
    echo "Installing default configuration..."
    cp config.example.yaml /etc/ucxsync/config.yaml
    echo -e "${GREEN}✓${NC} Configuration installed to /etc/ucxsync/config.yaml"
    echo -e "${YELLOW}⚠${NC}  Please edit /etc/ucxsync/config.yaml with your settings"
else
    echo -e "${YELLOW}⚠${NC}  Configuration already exists, skipping"
fi

# Install systemd service
echo ""
echo "Installing systemd service..."
cp ucxsync.service /etc/systemd/system/
systemctl daemon-reload
echo -e "${GREEN}✓${NC} Service installed"

# Set permissions
echo ""
echo "Setting permissions..."
chown -R root:root /opt/ucxsync
chown -R root:root /etc/ucxsync
chown -R root:root /var/log/ucxsync
chmod 700 /etc/ucxsync
chmod 600 /etc/ucxsync/config.yaml
echo -e "${GREEN}✓${NC} Permissions set"

echo ""
echo -e "${GREEN}Installation complete!${NC}"
echo ""
echo "Next steps:"
echo "  1. Edit configuration: nano /etc/ucxsync/config.yaml"
echo "  2. Enable service: systemctl enable ucxsync"
echo "  3. Start service: systemctl start ucxsync"
echo "  4. Check status: systemctl status ucxsync"
echo "  5. View logs: journalctl -u ucxsync -f"
echo "  6. Access web UI: http://localhost:8080"
echo ""
echo "To manually mount shares:"
echo "  /opt/ucxsync/ucxsync mount"
echo ""
echo "To unmount shares:"
echo "  /opt/ucxsync/ucxsync unmount"
