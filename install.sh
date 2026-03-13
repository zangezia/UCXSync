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

INSTALL_MODE="single"
MODE_EXPLICIT=0

usage() {
    cat <<EOF
Usage:
  sudo ./install.sh [--single | --dual | --mode single|dual]

Modes:
  --single           Install the main single-instance deployment (default)
  --dual             Install the dual deployment (ucxsync@a + ucxsync@b)
  --mode <value>     Explicitly choose 'single' or 'dual'
  -h, --help         Show this help

Behavior:
  - In interactive mode, the script asks which version to install if no flag is given.
  - In non-interactive mode, single-instance installation is used by default.
EOF
}

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --single)
                INSTALL_MODE="single"
                MODE_EXPLICIT=1
                ;;
            --dual)
                INSTALL_MODE="dual"
                MODE_EXPLICIT=1
                ;;
            --mode)
                shift
                if [ $# -eq 0 ]; then
                    echo -e "${RED}Error: --mode requires a value (single or dual)${NC}"
                    exit 1
                fi
                case "$1" in
                    single|dual)
                        INSTALL_MODE="$1"
                        MODE_EXPLICIT=1
                        ;;
                    *)
                        echo -e "${RED}Error: invalid mode '$1'. Use 'single' or 'dual'${NC}"
                        exit 1
                        ;;
                esac
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                echo -e "${RED}Error: unknown argument '$1'${NC}"
                echo ""
                usage
                exit 1
                ;;
        esac
        shift
    done
}

prompt_install_mode() {
    if [ "$MODE_EXPLICIT" -eq 1 ] || [ ! -t 0 ]; then
        return
    fi

    echo -e "${BLUE}Select installation mode:${NC}"
    echo "  1) Main version (single instance)"
    echo "  2) Dual version (two instances: ucxsync@a + ucxsync@b)"
    printf "Choose [1/2] (default: 1): "
    read -r MODE_CHOICE

    case "$MODE_CHOICE" in
        2|dual|Dual|DUAL)
            INSTALL_MODE="dual"
            ;;
        ""|1|single|Single|SINGLE)
            INSTALL_MODE="single"
            ;;
        *)
            echo -e "${YELLOW}Unknown selection, using single-instance installation${NC}"
            INSTALL_MODE="single"
            ;;
    esac
}

copy_if_missing() {
    local src="$1"
    local dst="$2"
    local description="$3"

    if [ ! -f "$src" ]; then
        echo -e "${YELLOW}⚠${NC}  Source file not found, skipping: $src"
        return
    fi

    if [ -f "$dst" ]; then
        echo -e "${YELLOW}⚠${NC}  $description already exists, skipping: $dst"
    else
        cp "$src" "$dst"
        echo -e "${GREEN}✓${NC} $description installed: $dst"
    fi
}

print_mode_summary() {
    if [ "$INSTALL_MODE" = "dual" ]; then
        echo -e "${BLUE}Installation mode: dual${NC}"
    else
        echo -e "${BLUE}Installation mode: single${NC}"
    fi
}

parse_args "$@"
prompt_install_mode

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}       UCXSync Installation${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
print_mode_summary
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

if [ "$INSTALL_MODE" = "dual" ]; then
    mkdir -p /ucmount-a
    mkdir -p /ucmount-b
    echo -e "${GREEN}✓${NC} Created /ucmount-a and /ucmount-b (dual UCX network mount points)"
else
    mkdir -p /ucmount
    echo -e "${GREEN}✓${NC} Created /ucmount (UCX network mount points)"
fi

# Create storage directory for USB-SSD
mkdir -p /ucdata
echo -e "${GREEN}✓${NC} Created /ucdata (USB-SSD mount point)"

# Check if USB-SSD is already mounted
if mountpoint -q /ucdata; then
    echo -e "${GREEN}✓${NC} /ucdata is already mounted"

    USER_NAME=${SUDO_USER:-$(whoami)}
    chown -R $USER_NAME:$USER_NAME /ucdata 2>/dev/null || true
    echo -e "${GREEN}✓${NC} Permissions set for /ucdata"
else
    echo -e "${YELLOW}⚠${NC}  /ucdata is not mounted"
    echo -e "${YELLOW}⚠${NC}  You need to mount your USB-SSD to /ucdata"
    echo ""
    echo "Option 1 - Manual mount (simple):"
    echo "  1. Find your device: lsblk"
    echo "  2. Mount it: sudo mount /dev/sdX1 /ucdata"
    echo ""
    echo "Option 2 - Auto-mount (recommended):"
    echo "  Run: sudo ./setup-usb-automount.sh"
    echo ""
    echo "See USB-SSD-GUIDE.md for detailed instructions"
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

echo ""
echo "Installing binary..."
cp ucxsync /opt/ucxsync/
chmod +x /opt/ucxsync/ucxsync
echo -e "${GREEN}✓${NC} Binary installed to /opt/ucxsync/ucxsync"

echo ""
echo "Installing web assets..."
rm -rf /opt/ucxsync/web
cp -r web /opt/ucxsync/
echo -e "${GREEN}✓${NC} Web assets installed to /opt/ucxsync/web"

echo ""
echo "Installing configuration..."
if [ "$INSTALL_MODE" = "dual" ]; then
    copy_if_missing config.instance-a.yaml /etc/ucxsync/a.yaml "Instance A configuration"
    copy_if_missing config.instance-b.yaml /etc/ucxsync/b.yaml "Instance B configuration"
    copy_if_missing config.example.yaml /etc/ucxsync/config.yaml "Legacy single-instance configuration"

    if [ -f setup-dualnic-routing.sh ]; then
        cp setup-dualnic-routing.sh /opt/ucxsync/setup-dualnic-routing.sh
        chmod +x /opt/ucxsync/setup-dualnic-routing.sh
        echo -e "${GREEN}✓${NC} Dual-NIC helper installed to /opt/ucxsync/setup-dualnic-routing.sh"
    else
        echo -e "${YELLOW}⚠${NC}  setup-dualnic-routing.sh not found, helper was not installed"
    fi
else
    if [ ! -f /etc/ucxsync/config.yaml ]; then
        cp config.example.yaml /etc/ucxsync/config.yaml
        echo -e "${GREEN}✓${NC} Configuration installed to /etc/ucxsync/config.yaml"
        echo -e "${YELLOW}⚠${NC}  Please edit /etc/ucxsync/config.yaml with your settings"
    else
        echo -e "${YELLOW}⚠${NC}  Configuration already exists, skipping"
    fi
fi

echo ""
echo "Installing systemd service..."
cp ucxsync.service /etc/systemd/system/
if [ "$INSTALL_MODE" = "dual" ]; then
    cp ucxsync@.service /etc/systemd/system/
    echo -e "${GREEN}✓${NC} Template service installed: /etc/systemd/system/ucxsync@.service"
fi
systemctl daemon-reload
echo -e "${GREEN}✓${NC} Service installed"

echo ""
echo "Setting permissions..."
chown -R root:root /opt/ucxsync
chown -R root:root /etc/ucxsync
chown -R root:root /var/log/ucxsync
chmod 700 /etc/ucxsync
if [ -f /etc/ucxsync/config.yaml ]; then
    chmod 600 /etc/ucxsync/config.yaml
fi
if [ -f /etc/ucxsync/a.yaml ]; then
    chmod 600 /etc/ucxsync/a.yaml
fi
if [ -f /etc/ucxsync/b.yaml ]; then
    chmod 600 /etc/ucxsync/b.yaml
fi
echo -e "${GREEN}✓${NC} Permissions set"

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}   Installation complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}Binary installed:${NC} /opt/ucxsync/ucxsync"
if [ "$INSTALL_MODE" = "dual" ]; then
    echo -e "${BLUE}Configurations:${NC} /etc/ucxsync/a.yaml and /etc/ucxsync/b.yaml"
    echo -e "${BLUE}Service files:${NC} /etc/systemd/system/ucxsync.service and /etc/systemd/system/ucxsync@.service"
else
    echo -e "${BLUE}Configuration:${NC} /etc/ucxsync/config.yaml"
    echo -e "${BLUE}Service file:${NC} /etc/systemd/system/ucxsync.service"
fi
echo ""
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}   IMPORTANT: USB-SSD Setup${NC}"
echo -e "${YELLOW}========================================${NC}"

if ! mountpoint -q /ucdata; then
    echo ""
    echo -e "${RED}⚠ USB-SSD is NOT mounted!${NC}"
    echo ""
    echo "UCXSync requires an external USB-SSD mounted at /ucdata"
    echo ""
    echo -e "${BLUE}Quick setup:${NC}"
    echo "  1. Connect your USB-SSD"
    echo "  2. Find device:    lsblk"
    echo "  3. Mount:          sudo mount /dev/sdX1 /ucdata"
    echo "  4. Set owner:      sudo chown -R \$USER:\$USER /ucdata"
    echo ""
    echo -e "${BLUE}See detailed guide:${NC} USB-SSD-GUIDE.md"
    echo ""
else
    echo ""
    echo -e "${GREEN}✓ USB-SSD is mounted at /ucdata${NC}"

    STORAGE_INFO=$(df -h /ucdata | tail -1 | awk '{print $2 " total, " $4 " free"}')
    echo -e "${BLUE}Storage:${NC} $STORAGE_INFO"
    echo ""
fi

echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}   Next Steps${NC}"
echo -e "${YELLOW}========================================${NC}"
echo ""

if [ "$INSTALL_MODE" = "dual" ]; then
    echo "1. ${BLUE}Edit dual-instance configurations:${NC}"
    echo "   sudo nano /etc/ucxsync/a.yaml"
    echo "   sudo nano /etc/ucxsync/b.yaml"
    echo ""
    echo "   Verify these settings:"
    echo "   - node lists do not overlap"
    echo "   - network.mount_root is /ucmount-a and /ucmount-b"
    echo "   - web.port is 8080 and 8081"
    echo "   - logging.file is different in each config"
    echo ""
    echo "2. ${BLUE}Setup USB-SSD auto-mount (recommended):${NC}"
    echo "   sudo ./setup-usb-automount.sh"
    echo ""
    echo "3. ${BLUE}Install dual-NIC routing helper (recommended):${NC}"
    echo "   sudo END0_IFACE=end0 END1_IFACE=end1 END0_SRC_IP=192.168.200.101 END1_SRC_IP=192.168.200.102 END0_HOSTS=\"1 2 3 4 5 6 7\" END1_HOSTS=\"8 9 10 11 12 13 201\" /opt/ucxsync/setup-dualnic-routing.sh --install"
    echo ""
    echo "4. ${BLUE}Enable auto-start:${NC}"
    echo "   sudo systemctl enable ucxsync@a ucxsync@b"
    echo ""
    echo "5. ${BLUE}Start both instances:${NC}"
    echo "   sudo systemctl start ucxsync@a ucxsync@b"
    echo ""
    echo "6. ${BLUE}Check status:${NC}"
    echo "   sudo systemctl status ucxsync@a"
    echo "   sudo systemctl status ucxsync@b"
    echo ""
    echo "7. ${BLUE}View logs:${NC}"
    echo "   sudo journalctl -u ucxsync@a -f"
    echo "   sudo journalctl -u ucxsync@b -f"
    echo ""
    echo "8. ${BLUE}Access web interfaces:${NC}"
    echo "   http://localhost:8080"
    echo "   http://localhost:8081"
else
    echo "1. ${BLUE}Edit configuration:${NC}"
    echo "   sudo nano /etc/ucxsync/config.yaml"
    echo ""
    echo "   Update these settings:"
    echo "   - sync.project (your project name)"
    echo "   - sync.destination (/ucdata)"
    echo "   - credentials.username and password"
    echo ""
    echo "2. ${BLUE}Setup USB-SSD auto-mount (recommended):${NC}"
    echo "   sudo ./setup-usb-automount.sh"
    echo ""
    echo "3. ${BLUE}Enable auto-start:${NC}"
    echo "   sudo systemctl enable ucxsync"
    echo ""
    echo "4. ${BLUE}Start service:${NC}"
    echo "   sudo systemctl start ucxsync"
    echo ""
    echo "5. ${BLUE}Check status:${NC}"
    echo "   sudo systemctl status ucxsync"
    echo ""
    echo "6. ${BLUE}View logs:${NC}"
    echo "   sudo journalctl -u ucxsync -f"
    echo ""
    echo "7. ${BLUE}Access web interface:${NC}"
    echo "   http://localhost:8080"
fi

echo ""
echo -e "${YELLOW}========================================${NC}"
echo ""
echo -e "${BLUE}Documentation:${NC}"
echo "  - Quick start:        README.md"
echo "  - USB-SSD setup:      USB-SSD-GUIDE.md"
echo "  - Storage explained:  STORAGE-ARCHITECTURE.md"
echo "  - Testing guide:      TESTING-ON-LAPTOP.md"
echo ""
