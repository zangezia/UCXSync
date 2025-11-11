#!/bin/bash
#
# UCXSync Installation Script for Linux (All architectures)
# Supports: AMD64, ARM64, RISC-V 64
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}       UCXSync Installation${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Detect architecture
ARCH=$(uname -m)
echo -e "${BLUE}Detected architecture: $ARCH${NC}"

case "$ARCH" in
    x86_64)
        GOARCH="amd64"
        GO_DOWNLOAD="go1.21.5.linux-amd64.tar.gz"
        ;;
    aarch64)
        GOARCH="arm64"
        GO_DOWNLOAD="go1.21.5.linux-arm64.tar.gz"
        ;;
    riscv64)
        GOARCH="riscv64"
        GO_DOWNLOAD="go1.21.5.linux-riscv64.tar.gz"
        echo -e "${YELLOW}Note: RISC-V detected. For Orange Pi RV2, use ./install-orangepi.sh instead${NC}"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        echo "Supported: x86_64 (AMD64), aarch64 (ARM64), riscv64"
        exit 1
        ;;
esac

echo -e "${GREEN}Target architecture: $GOARCH${NC}"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    echo "Please run: sudo ./install.sh"
    exit 1
fi

# Detect OS
if [ -f /etc/os-release ]; then
    . /etc/os-release
    echo -e "${BLUE}OS: $NAME $VERSION${NC}"
else
    echo -e "${YELLOW}Warning: Cannot detect OS version${NC}"
fi
echo ""

# Check prerequisites
echo -e "${GREEN}[1/6] Checking prerequisites...${NC}"

# Check Go
if ! command -v go &> /dev/null; then
    echo -e "${YELLOW}Go is not installed. Installing...${NC}"
    echo "Downloading $GO_DOWNLOAD..."
    
    wget -q https://go.dev/dl/$GO_DOWNLOAD -O /tmp/$GO_DOWNLOAD || {
        echo -e "${RED}Failed to download Go${NC}"
        echo "Please install Go manually from: https://go.dev/dl/"
        exit 1
    }
    
    tar -C /usr/local -xzf /tmp/$GO_DOWNLOAD
    rm /tmp/$GO_DOWNLOAD
    
    # Add to PATH
    if ! grep -q '/usr/local/go/bin' /etc/profile; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    fi
    
    export PATH=$PATH:/usr/local/go/bin
    echo -e "${GREEN}✓ Go installed${NC}"
else
    GO_VERSION=$(go version | awk '{print $3}')
    echo -e "${GREEN}✓ Go already installed: $GO_VERSION${NC}"
fi

# Check cifs-utils
if ! dpkg -l | grep -q cifs-utils 2>/dev/null && ! rpm -q cifs-utils &>/dev/null; then
    echo -e "${YELLOW}Installing cifs-utils...${NC}"
    
    if command -v apt-get &> /dev/null; then
        apt-get update -qq
        apt-get install -y cifs-utils
    elif command -v yum &> /dev/null; then
        yum install -y cifs-utils
    elif command -v dnf &> /dev/null; then
        dnf install -y cifs-utils
    else
        echo -e "${RED}Cannot install cifs-utils automatically${NC}"
        echo "Please install manually: apt-get install cifs-utils"
        exit 1
    fi
    
    echo -e "${GREEN}✓ cifs-utils installed${NC}"
else
    echo -e "${GREEN}✓ cifs-utils already installed${NC}"
fi

echo ""
echo -e "${GREEN}[2/6] Building UCXSync for $GOARCH...${NC}"
GOOS=linux GOARCH=$GOARCH go build -ldflags "-X main.Version=1.1.0 -X main.BuildTime=$(date -u '+%Y-%m-%d_%H:%M:%S')" -o ucxsync ./cmd/ucxsync
echo -e "${GREEN}✓ Build complete${NC}"

echo ""
echo -e "${GREEN}[3/6] Creating directories...${NC}"
mkdir -p /opt/ucxsync
mkdir -p /etc/ucxsync
mkdir -p /var/log/ucxsync

# Create mount points for UCX nodes (network shares)
mkdir -p /mnt/ucx
echo -e "${GREEN}✓${NC} Created /mnt/ucx (UCX network mount points)"

# Create storage directory for USB-SSD
mkdir -p /mnt/storage
echo -e "${GREEN}✓${NC} Created /mnt/storage (USB-SSD mount point)"

# Check if USB-SSD is already mounted
if mountpoint -q /mnt/storage; then
    echo -e "${GREEN}✓${NC} /mnt/storage is already mounted"
    # Create UCX data directory on mounted storage
    mkdir -p /mnt/storage/ucx
    chown -R $SUDO_USER:$SUDO_USER /mnt/storage/ucx 2>/dev/null || chown -R $(whoami):$(whoami) /mnt/storage/ucx
    echo -e "${GREEN}✓${NC} Created /mnt/storage/ucx for data"
else
    echo -e "${YELLOW}⚠${NC}  /mnt/storage is not mounted"
    echo -e "${YELLOW}⚠${NC}  You need to mount your USB-SSD to /mnt/storage"
    echo ""
    echo "To mount USB-SSD:"
    echo "  1. Find your device: lsblk"
    echo "  2. Mount it: sudo mount /dev/sdX1 /mnt/storage"
    echo "  3. Create data dir: sudo mkdir -p /mnt/storage/ucx"
    echo "  4. Set permissions: sudo chown -R \$USER:\$USER /mnt/storage/ucx"
    echo ""
    echo "Or see USB-SSD-GUIDE.md for detailed instructions"
    echo ""
fi

echo -e "${GREEN}✓${NC} Directories created"

echo ""
echo -e "${GREEN}[4/6] Installing application...${NC}"
HOSTS_MARKER="# UCXSync nodes"
if ! grep -q "$HOSTS_MARKER" /etc/hosts; then
    echo "" >> /etc/hosts
    echo "$HOSTS_MARKER" >> /etc/hosts
    echo "192.168.200.1    WU01" >> /etc/hosts
    echo "192.168.200.2    WU02" >> /etc/hosts
    echo "192.168.200.3    WU03" >> /etc/hosts
    echo "192.168.200.4    WU04" >> /etc/hosts
    echo "192.168.200.5    WU05" >> /etc/hosts
    echo "192.168.200.6    WU06" >> /etc/hosts
    echo "192.168.200.7    WU07" >> /etc/hosts
    echo "192.168.200.8    WU08" >> /etc/hosts
    echo "192.168.200.9    WU09" >> /etc/hosts
    echo "192.168.200.10   WU10" >> /etc/hosts
    echo "192.168.200.11   WU11" >> /etc/hosts
    echo "192.168.200.12   WU12" >> /etc/hosts
    echo "192.168.200.13   WU13" >> /etc/hosts
    echo "192.168.200.201  CU" >> /etc/hosts
    echo -e "${GREEN}✓${NC} Network hosts mapping added to /etc/hosts"
else
    echo -e "${YELLOW}⚠${NC}  Network hosts mapping already exists in /etc/hosts"
fi

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
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}   Installation complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}Binary installed:${NC} /usr/local/bin/ucxsync"
echo -e "${BLUE}Configuration:${NC} /etc/ucxsync/config.yaml"
echo -e "${BLUE}Service file:${NC} /etc/systemd/system/ucxsync.service"
echo ""
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}   IMPORTANT: USB-SSD Setup${NC}"
echo -e "${YELLOW}========================================${NC}"

# Check if USB-SSD is mounted
if ! mountpoint -q /mnt/storage; then
    echo ""
    echo -e "${RED}⚠ USB-SSD is NOT mounted!${NC}"
    echo ""
    echo "UCXSync requires an external USB-SSD mounted at /mnt/storage"
    echo ""
    echo -e "${BLUE}Quick setup:${NC}"
    echo "  1. Connect your USB-SSD"
    echo "  2. Find device:    lsblk"
    echo "  3. Mount:          sudo mount /dev/sdX1 /mnt/storage"
    echo "  4. Create dir:     sudo mkdir -p /mnt/storage/ucx"
    echo "  5. Set owner:      sudo chown -R \$USER:\$USER /mnt/storage/ucx"
    echo ""
    echo -e "${BLUE}See detailed guide:${NC} USB-SSD-GUIDE.md"
    echo ""
else
    echo ""
    echo -e "${GREEN}✓ USB-SSD is mounted at /mnt/storage${NC}"
    
    # Show storage info
    STORAGE_INFO=$(df -h /mnt/storage | tail -1 | awk '{print $2 " total, " $4 " free"}')
    echo -e "${BLUE}Storage:${NC} $STORAGE_INFO"
    
    if [ -d /mnt/storage/ucx ]; then
        echo -e "${GREEN}✓ Data directory ready: /mnt/storage/ucx${NC}"
    fi
    echo ""
fi

echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}   Next Steps${NC}"
echo -e "${YELLOW}========================================${NC}"
echo ""
echo "1. ${BLUE}Edit configuration:${NC}"
echo "   sudo nano /etc/ucxsync/config.yaml"
echo ""
echo "   Update these settings:"
echo "   - sync.project (your project name)"
echo "   - sync.destination (/mnt/storage/ucx)"
echo "   - credentials.username and password"
echo ""
echo "2. ${BLUE}Enable auto-start:${NC}"
echo "   sudo systemctl enable ucxsync"
echo ""
echo "3. ${BLUE}Start service:${NC}"
echo "   sudo systemctl start ucxsync"
echo ""
echo "4. ${BLUE}Check status:${NC}"
echo "   sudo systemctl status ucxsync"
echo ""
echo "5. ${BLUE}View logs:${NC}"
echo "   sudo journalctl -u ucxsync -f"
echo ""
echo "6. ${BLUE}Access web interface:${NC}"
echo "   http://localhost:8080"
echo ""
echo -e "${YELLOW}========================================${NC}"
echo ""
echo -e "${BLUE}Documentation:${NC}"
echo "  - Quick start:        README.md"
echo "  - USB-SSD setup:      USB-SSD-GUIDE.md"
echo "  - Storage explained:  STORAGE-ARCHITECTURE.md"
echo "  - Testing guide:      TESTING-ON-LAPTOP.md"
echo ""
