#!/bin/bash
#
# Auto-mount USB-SSD Script for UCXSync
# This script creates udev rules to automatically mount the first USB storage device to /mnt/storage
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  USB-SSD Auto-Mount Setup${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    echo "Please run: sudo $0"
    exit 1
fi

# Create mount point
echo -e "${BLUE}[1/4] Creating mount point...${NC}"
mkdir -p /mnt/storage
echo -e "${GREEN}✓${NC} Created /mnt/storage"

# Create systemd mount unit
echo -e "${BLUE}[2/4] Creating systemd mount service...${NC}"

cat > /etc/systemd/system/mnt-storage.mount << 'EOF'
[Unit]
Description=USB-SSD Storage for UCXSync
After=local-fs.target

[Mount]
What=/dev/disk/by-label/UCX-Storage
Where=/mnt/storage
Type=auto
Options=defaults,nofail,x-systemd.device-timeout=5

[Install]
WantedBy=multi-user.target
EOF

# Create automount unit
cat > /etc/systemd/system/mnt-storage.automount << 'EOF'
[Unit]
Description=Auto-mount USB-SSD for UCXSync
Before=ucxsync.service

[Automount]
Where=/mnt/storage
TimeoutIdleSec=0

[Install]
WantedBy=multi-user.target
EOF

echo -e "${GREEN}✓${NC} Systemd units created"

# Create udev rule for auto-mount first USB storage
echo -e "${BLUE}[3/4] Creating udev rule...${NC}"

cat > /etc/udev/rules.d/99-usb-storage-automount.rules << 'EOF'
# Auto-mount USB/SCSI storage device to /mnt/storage
# Samsung T7 and similar USB-SSD devices appear as SCSI, not USB

ACTION=="add", SUBSYSTEM=="block", ENV{DEVTYPE}=="partition", \
  KERNEL=="sd[b-z][0-9]", \
  RUN+="/bin/systemd-run --no-block /usr/local/bin/ucxsync-mount-usb.sh %k"

ACTION=="remove", SUBSYSTEM=="block", ENV{DEVTYPE}=="partition", \
  KERNEL=="sd[b-z][0-9]", \
  RUN+="/bin/systemd-run --no-block /usr/local/bin/ucxsync-unmount-usb.sh %k"
EOF

# Create mount helper script
cat > /usr/local/bin/ucxsync-mount-usb.sh << 'EOF'
#!/bin/bash
# Auto-mount USB device to /mnt/storage

DEVICE="/dev/$1"
MOUNT_POINT="/mnt/storage"
LOCK_FILE="/var/lock/ucxsync-usb-mount"

# Full paths for udev/systemd context
MOUNT="/bin/mount"
MKDIR="/bin/mkdir"
MOUNTPOINT="/bin/mountpoint"
CHMOD="/bin/chmod"
CHOWN="/bin/chown"
ID="/usr/bin/id"
LOGGER="/usr/bin/logger"
SLEEP="/bin/sleep"
RMDIR="/bin/rmdir"

# Only mount if nothing is mounted yet
if ! $MOUNTPOINT -q "$MOUNT_POINT"; then
    # Create lock to prevent multiple mounts
    if $MKDIR "$LOCK_FILE" 2>/dev/null; then
        # Wait for device to be ready
        $SLEEP 2
        
        # Create mount point if it doesn't exist
        $MKDIR -p "$MOUNT_POINT"
        
        # Mount the device
        $MOUNT "$DEVICE" "$MOUNT_POINT" 2>&1 | $LOGGER -t UCXSync
        
        if $MOUNTPOINT -q "$MOUNT_POINT"; then
            $LOGGER -t UCXSync "USB storage $DEVICE auto-mounted to $MOUNT_POINT"
            
            # Set permissions for user access
            $CHMOD 755 "$MOUNT_POINT"
            
            # If ucxsync user exists, set ownership
            if $ID ucxsync >/dev/null 2>&1; then
                $CHOWN ucxsync:ucxsync "$MOUNT_POINT"
            fi
        else
            $LOGGER -t UCXSync "Failed to mount USB storage $DEVICE"
        fi
        
        # Remove lock
        $RMDIR "$LOCK_FILE"
    fi
fi
EOF

# Create unmount helper script
cat > /usr/local/bin/ucxsync-unmount-usb.sh << 'EOF'
#!/bin/bash
# Auto-unmount USB device from /mnt/storage

DEVICE="/dev/$1"
MOUNT_POINT="/mnt/storage"

# Full paths for udev/systemd context
MOUNTPOINT="/bin/mountpoint"
FINDMNT="/bin/findmnt"
UMOUNT="/bin/umount"
LOGGER="/usr/bin/logger"

# Check if our device is mounted
if $MOUNTPOINT -q "$MOUNT_POINT"; then
    MOUNTED_DEVICE=$($FINDMNT -n -o SOURCE "$MOUNT_POINT")
    if [ "$MOUNTED_DEVICE" = "$DEVICE" ]; then
        # Try to unmount
        $UMOUNT "$MOUNT_POINT" 2>/dev/null
        if [ $? -eq 0 ]; then
            $LOGGER -t UCXSync "USB storage $DEVICE auto-unmounted from $MOUNT_POINT"
        else
            $LOGGER -t UCXSync "Failed to unmount USB storage $DEVICE (device busy)"
        fi
    fi
fi
EOF

chmod +x /usr/local/bin/ucxsync-mount-usb.sh
chmod +x /usr/local/bin/ucxsync-unmount-usb.sh

echo -e "${GREEN}✓${NC} Udev rule and helper scripts created"

# Reload udev rules
echo -e "${BLUE}[4/4] Reloading udev rules...${NC}"
udevadm control --reload-rules
udevadm trigger
echo -e "${GREEN}✓${NC} Udev rules reloaded"

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Auto-mount setup complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo ""
echo "1. Label your USB-SSD (optional but recommended):"
echo "   sudo e2label /dev/sdX1 UCX-Storage"
echo ""
echo "2. Test auto-mount:"
echo "   - Disconnect your USB-SSD"
echo "   - Reconnect it"
echo "   - Check: mountpoint /mnt/storage"
echo ""
echo "3. Update UCXSync config:"
echo "   sudo nano /etc/ucxsync/config.yaml"
echo "   Change: destination: \"/mnt/storage\""
echo ""
echo "4. If you have existing data in /mnt/storage/ucx:"
echo "   sudo mv /mnt/storage/ucx/* /mnt/storage/"
echo "   sudo rmdir /mnt/storage/ucx"
echo ""
echo -e "${YELLOW}Notes:${NC}"
echo "- First connected USB storage will be auto-mounted to /mnt/storage"
echo "- Device must be formatted (ext4, NTFS, exFAT, etc.)"
echo "- Auto-mount happens within 2-3 seconds of connection"
echo "- Check logs: journalctl | grep UCXSync"
echo ""
