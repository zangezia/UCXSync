#!/bin/bash
#
# UCXSync Uninstall Script
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${RED}UCXSync Uninstallation${NC}"
echo "======================================"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
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
    for mount in $(mount | grep "/mnt/ucx" | cut -d' ' -f3); do
        umount "$mount" 2>/dev/null || true
    done
fi
echo -e "${GREEN}✓${NC} Shares unmounted"

# Remove files
echo ""
echo "Removing files..."
rm -f /etc/systemd/system/ucxsync.service
rm -rf /opt/ucxsync
rm -rf /var/log/ucxsync
systemctl daemon-reload
echo -e "${GREEN}✓${NC} Files removed"

# Ask about config
echo ""
read -p "Remove configuration from /etc/ucxsync? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -rf /etc/ucxsync
    echo -e "${GREEN}✓${NC} Configuration removed"
else
    echo -e "${YELLOW}⚠${NC}  Configuration preserved"
fi

# Ask about mount points
echo ""
read -p "Remove mount directory /mnt/ucx? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -rf /mnt/ucx
    echo -e "${GREEN}✓${NC} Mount directory removed"
else
    echo -e "${YELLOW}⚠${NC}  Mount directory preserved"
fi

# Ask about hosts entries
echo ""
read -p "Remove UCXSync entries from /etc/hosts? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    sed -i '/# UCXSync nodes/,/^192\.168\.200\.201.*CU/d' /etc/hosts
    # Remove empty lines left behind
    sed -i '/^$/N;/^\n$/D' /etc/hosts
    echo -e "${GREEN}✓${NC} Hosts entries removed"
else
    echo -e "${YELLOW}⚠${NC}  Hosts entries preserved"
fi

echo ""
echo -e "${GREEN}Uninstallation complete!${NC}"
