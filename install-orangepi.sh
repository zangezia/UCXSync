#!/bin/bash

# UCXSync Installation Script for Orange Pi RV2 (Ubuntu Server 24.04)
# Run with: sudo ./install-orangepi.sh

set -e

INSTALL_DUALNIC_ROUTING=0
ROUTING_EXPLICIT=0
END0_IFACE_VALUE="${END0_IFACE:-end0}"
END1_IFACE_VALUE="${END1_IFACE:-end1}"
END0_SRC_IP_VALUE="${END0_SRC_IP:-192.168.200.101}"
END1_SRC_IP_VALUE="${END1_SRC_IP:-192.168.200.102}"
END0_HOSTS_VALUE="${END0_HOSTS:-1 2 3 4 5 6 7}"
END1_HOSTS_VALUE="${END1_HOSTS:-8 9 10 11 12 13 201}"

INSTALL_DIR="/opt/ucxsync"
CONFIG_DIR="/etc/ucxsync"
LOG_DIR="/var/log/ucxsync"
MOUNT_DIR="/ucmount"
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

usage() {
    cat <<EOF
Usage:
  sudo ./install-orangepi.sh [--install-dualnic-routing]

Options:
  --install-dualnic-routing  Install and enable the dual-NIC routing helper
  --skip-dualnic-routing     Skip routing setup even in interactive mode
  -h, --help                 Show this help

Environment overrides for routing:
  END0_IFACE / END1_IFACE / END0_SRC_IP / END1_SRC_IP / END0_HOSTS / END1_HOSTS
Interactive mode can prompt for these values before routing is installed.
EOF
}

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --install-dualnic-routing)
                INSTALL_DUALNIC_ROUTING=1
                ROUTING_EXPLICIT=1
                ;;
            --skip-dualnic-routing)
                INSTALL_DUALNIC_ROUTING=0
                ROUTING_EXPLICIT=1
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                echo -e "${RED}Unknown argument: $1${NC}"
                usage
                exit 1
                ;;
        esac
        shift
    done
}

prompt_dualnic_routing() {
    if [ "$ROUTING_EXPLICIT" -eq 1 ] || [ ! -t 0 ]; then
        return
    fi

    echo -e "${YELLOW}Install dual-NIC routing helper during setup?${NC}"
    printf "Install dual-NIC routing now? [y/N]: "
    read -r ROUTING_CHOICE

    case "$ROUTING_CHOICE" in
        y|Y|yes|YES)
            INSTALL_DUALNIC_ROUTING=1
            ;;
        *)
            INSTALL_DUALNIC_ROUTING=0
            ;;
    esac
}

prompt_value() {
    local prompt="$1"
    local default_value="$2"
    local result

    printf "%s [%s]: " "$prompt" "$default_value" >&2
    read -r result
    if [ -z "$result" ]; then
        result="$default_value"
    fi
    printf '%s' "$result"
}

normalize_hosts_value() {
    local raw="$1"
    raw="${raw//,/ }"
    printf '%s' "$raw" | awk '{$1=$1; print}'
}

show_detected_interfaces() {
    if ! command -v ip >/dev/null 2>&1; then
        return
    fi

    echo "  Available interfaces:"
    ip -o link show | awk -F': ' '{print $2}' | cut -d'@' -f1 | grep -v '^lo$' | sed 's/^/    - /'
}

prompt_routing_parameters() {
    if [ "$INSTALL_DUALNIC_ROUTING" -ne 1 ] || [ ! -t 0 ]; then
        return
    fi

    echo -e "${YELLOW}Dual-NIC routing wizard${NC}"
    echo "  Leave values empty to accept the defaults shown in brackets."
    show_detected_interfaces

    END0_IFACE_VALUE=$(prompt_value "  Primary interface for the first host group" "$END0_IFACE_VALUE")
    echo ""
    END0_SRC_IP_VALUE=$(prompt_value "  Source IPv4 address on $END0_IFACE_VALUE" "$END0_SRC_IP_VALUE")
    echo ""
    END0_HOSTS_VALUE=$(normalize_hosts_value "$(prompt_value "  Hosts routed via $END0_IFACE_VALUE (last octets or full IPs)" "$END0_HOSTS_VALUE")")
    echo ""
    END1_IFACE_VALUE=$(prompt_value "  Secondary interface for the second host group" "$END1_IFACE_VALUE")
    echo ""
    END1_SRC_IP_VALUE=$(prompt_value "  Source IPv4 address on $END1_IFACE_VALUE" "$END1_SRC_IP_VALUE")
    echo ""
    END1_HOSTS_VALUE=$(normalize_hosts_value "$(prompt_value "  Hosts routed via $END1_IFACE_VALUE (last octets or full IPs)" "$END1_HOSTS_VALUE")")
    echo ""
}

install_dualnic_routing() {
    if [ "$INSTALL_DUALNIC_ROUTING" -ne 1 ]; then
        return
    fi

    if [ ! -x "$INSTALL_DIR/setup-dualnic-routing.sh" ]; then
        echo -e "${YELLOW}⚠${NC}  Dual-NIC helper not found, skipping routing installation"
        return
    fi

    echo -e "${GREEN}Installing dual-NIC routing helper...${NC}"
    if END0_IFACE="$END0_IFACE_VALUE" \
        END1_IFACE="$END1_IFACE_VALUE" \
        END0_SRC_IP="$END0_SRC_IP_VALUE" \
        END1_SRC_IP="$END1_SRC_IP_VALUE" \
        END0_HOSTS="$END0_HOSTS_VALUE" \
        END1_HOSTS="$END1_HOSTS_VALUE" \
        "$INSTALL_DIR/setup-dualnic-routing.sh" --install; then
        echo -e "${GREEN}✓${NC} Dual-NIC routing installed and enabled"
    else
        echo -e "${YELLOW}⚠${NC}  Dual-NIC routing installation failed; run the helper manually after adjusting interface/IP overrides"
    fi
}

parse_args "$@"
prompt_dualnic_routing

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo -e "${RED}Error: This script must be run as root${NC}"
   echo "Please run: sudo $0"
   exit 1
fi

prompt_routing_parameters

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
mkdir -p /ucdata
echo -e "${GREEN}✓${NC} Created /ucdata (USB-SSD mount point)"

# Check if USB-SSD is already mounted
if mountpoint -q /ucdata; then
    echo -e "${GREEN}✓${NC} /ucdata is already mounted"
    
    # Set permissions for user access
    USER_NAME=${SUDO_USER:-$(whoami)}
    chown -R $USER_NAME:$USER_NAME /ucdata 2>/dev/null || true
    echo -e "${GREEN}✓${NC} Permissions set for /ucdata"
else
    echo -e "${YELLOW}⚠${NC}  /ucdata is not mounted"
    echo -e "${YELLOW}⚠${NC}  You need to mount your USB-SSD to /ucdata"
    echo ""
    echo "Option 1 - Manual mount:"
    echo "  1. Find your device: lsblk"
    echo "  2. Mount it: sudo mount /dev/sdX1 /ucdata"
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
if [ -f "setup-dualnic-routing.sh" ]; then
    cp setup-dualnic-routing.sh "$INSTALL_DIR/"
    chmod +x "$INSTALL_DIR/setup-dualnic-routing.sh"
    echo -e "${GREEN}✓${NC} Dual-NIC helper installed to $INSTALL_DIR/setup-dualnic-routing.sh"
fi

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
ProtectSystem=full
ProtectHome=true
ReadWritePaths=$LOG_DIR $MOUNT_DIR /ucdata

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
systemctl daemon-reload

install_dualnic_routing

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Installation completed successfully!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check USB-SSD status
if ! mountpoint -q /ucdata; then
    echo -e "${RED}⚠ WARNING: USB-SSD is NOT mounted!${NC}"
    echo ""
    echo "UCXSync requires an external USB-SSD mounted at /ucdata"
    echo ""
    echo -e "${YELLOW}Quick setup:${NC}"
    echo "  1. Connect your USB-SSD"
    echo "  2. Find device:    lsblk"
    echo "  3. Mount:          sudo mount /dev/sdX1 /ucdata"
    echo "  4. Set owner:      sudo chown -R \$USER:\$USER /ucdata"
    echo ""
else
    echo -e "${GREEN}✓ USB-SSD is mounted at /ucdata${NC}"
    STORAGE_INFO=$(df -h /ucdata 2>/dev/null | tail -1 | awk '{print $2 " total, " $4 " free"}')
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
    echo "   - sync.destination (/ucdata)"
echo "   - credentials.username and password"
echo ""
echo "2. Setup USB-SSD auto-mount (recommended):"
echo "   sudo ./setup-usb-automount.sh"
echo ""
echo "   Optional dual-NIC routing helper:"
echo "   sudo END0_IFACE=end0 END1_IFACE=enx00e04c141b68 END0_SRC_IP=192.168.200.101 END1_SRC_IP=192.168.200.103 $INSTALL_DIR/setup-dualnic-routing.sh --install"
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
echo "- Use USB 3.0 SSD for best performance (/ucdata)"
echo "- Consider active cooling for 24/7 operation"
echo ""
echo -e "${YELLOW}Documentation:${NC}"
echo "- Orange Pi guide:    ORANGEPI.md"
echo "- USB-SSD setup:      USB-SSD-GUIDE.md"
echo "- Storage explained:  STORAGE-ARCHITECTURE.md"
echo ""
