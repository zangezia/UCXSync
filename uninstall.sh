#!/bin/bash
#
# UCXSync Uninstall Script - Updated for new installation paths
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${RED}UCXSync Uninstallation${NC}"
echo "======================================"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    echo "Please run: sudo ./uninstall.sh"
    exit 1
fi

# Stop service
echo "Stopping service..."
systemctl stop ucxsync 2>/dev/null || true
systemctl disable ucxsync 2>/dev/null || true
echo -e "${GREEN}✓${NC} Service stopped"

# Unmount shares
echo ""
echo "Unmounting network shares..."
if [ -d /mnt/ucx ]; then
    for mount in $(mount | grep "/mnt/ucx" | cut -d' ' -f3 2>/dev/null); do
        umount "$mount" 2>/dev/null || true
    done
fi
echo -e "${GREEN}✓${NC} Shares unmounted"

# Remove files
echo ""
echo "Removing files..."

# Remove binary from /usr/local/bin
if [ -f /usr/local/bin/ucxsync ]; then
    rm -f /usr/local/bin/ucxsync
    echo -e "${GREEN}✓${NC} Removed /usr/local/bin/ucxsync"
fi

# Remove old installation paths (if they exist)
rm -f /etc/systemd/system/ucxsync.service
rm -rf /opt/ucxsync
rm -rf /var/log/ucxsync

systemctl daemon-reload
echo -e "${GREEN}✓${NC} System files removed"

# Ask about config
echo ""
read -p "Remove configuration from /etc/ucxsync? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -rf /etc/ucxsync
    echo -e "${GREEN}✓${NC} Configuration removed"
else
    echo -e "${YELLOW}⚠${NC}  Configuration preserved at /etc/ucxsync"
fi

# Ask about mount points
echo ""
read -p "Remove mount directory /mnt/ucx? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -rf /mnt/ucx
    echo -e "${GREEN}✓${NC} Mount directory removed"
else
    echo -e "${YELLOW}⚠${NC}  Mount directory preserved at /mnt/ucx"
fi

# Ask about data directory
echo ""
read -p "Remove data directory /mnt/storage/ucx? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${RED}⚠ WARNING: This will delete all synchronized data!${NC}"
    read -p "Are you absolutely sure? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf /mnt/storage/ucx
        echo -e "${GREEN}✓${NC} Data directory removed"
    else
        echo -e "${YELLOW}⚠${NC}  Data directory preserved at /mnt/storage/ucx"
    fi
else
    echo -e "${YELLOW}⚠${NC}  Data directory preserved at /mnt/storage/ucx"
fi

# Ask about dependencies
echo ""
read -p "Remove dependencies (Go, cifs-utils)? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    if command -v apt-get &> /dev/null; then
        apt-get remove -y golang cifs-utils || true
        apt-get autoremove -y || true
        echo -e "${GREEN}✓${NC} Dependencies removed (apt)"
    elif command -v yum &> /dev/null; then
        yum remove -y golang cifs-utils || true
        yum autoremove -y || true
        echo -e "${GREEN}✓${NC} Dependencies removed (yum)"
    elif command -v dnf &> /dev/null; then
        dnf remove -y golang cifs-utils || true
        dnf autoremove -y || true
        echo -e "${GREEN}✓${NC} Dependencies removed (dnf)"
    fi
else
    echo -e "${YELLOW}⚠${NC}  Dependencies preserved"
fi

echo ""
echo "======================================"
echo -e "${GREEN}Uninstallation complete!${NC}"
echo "======================================"
echo ""
echo "To reinstall UCXSync:"
echo "  git clone https://github.com/zangezia/UCXSync.git"
echo "  cd UCXSync"
echo "  chmod +x QUICK-TEST.sh"
echo "  sudo ./QUICK-TEST.sh"
echo ""
