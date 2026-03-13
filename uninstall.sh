#!/bin/bash
#
# UCXSync Uninstall Script
# Supports removing single deployment, dual deployment, or all components.
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

UNINSTALL_MODE="single"
MODE_EXPLICIT=0

DUAL_ROUTING_SERVICE="/etc/systemd/system/ucxsync-dualnic-routing.service"
DUAL_ROUTING_SCRIPT="/opt/ucxsync/setup-dualnic-routing.sh"
DUAL_ROUTING_INSTALLER="/usr/local/sbin/ucxsync-dualnic-routing.sh"
DUAL_SINGLE_DROPIN_DIR="/etc/systemd/system/ucxsync.service.d"
DUAL_TEMPLATE_DROPIN_DIR="/etc/systemd/system/ucxsync@.service.d"
DUAL_SYSCTL_FILE="/etc/sysctl.d/99-ucxsync-dualnic.conf"

usage() {
    cat <<EOF
Usage:
  sudo ./uninstall.sh [--single | --dual | --all | --mode single|dual|all]

Modes:
  --single           Remove the main single-instance deployment only (default)
  --dual             Remove the dual-instance deployment only
  --all              Remove all UCXSync deployments and shared files
  --mode <value>     Explicitly choose 'single', 'dual', or 'all'
  -h, --help         Show this help

Behavior:
  - In interactive mode, the script asks which uninstall mode to use if no flag is given.
  - In non-interactive mode, single-instance uninstall is used by default.
  - Shared files under /opt/ucxsync are removed only in --all mode.
EOF
}

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --single)
                UNINSTALL_MODE="single"
                MODE_EXPLICIT=1
                ;;
            --dual)
                UNINSTALL_MODE="dual"
                MODE_EXPLICIT=1
                ;;
            --all)
                UNINSTALL_MODE="all"
                MODE_EXPLICIT=1
                ;;
            --mode)
                shift
                if [ $# -eq 0 ]; then
                    echo -e "${RED}Error: --mode requires a value (single, dual, or all)${NC}"
                    exit 1
                fi
                case "$1" in
                    single|dual|all)
                        UNINSTALL_MODE="$1"
                        MODE_EXPLICIT=1
                        ;;
                    *)
                        echo -e "${RED}Error: invalid mode '$1'. Use 'single', 'dual', or 'all'${NC}"
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

prompt_uninstall_mode() {
    if [ "$MODE_EXPLICIT" -eq 1 ] || [ ! -t 0 ]; then
        return
    fi

    echo -e "${BLUE}Select uninstall mode:${NC}"
    echo "  1) Remove main version (single instance only)"
    echo "  2) Remove dual version (ucxsync@a + ucxsync@b + dual routing)"
    echo "  3) Remove everything"
    printf "Choose [1/2/3] (default: 1): "
    read -r MODE_CHOICE

    case "$MODE_CHOICE" in
        2|dual|Dual|DUAL)
            UNINSTALL_MODE="dual"
            ;;
        3|all|All|ALL)
            UNINSTALL_MODE="all"
            ;;
        ""|1|single|Single|SINGLE)
            UNINSTALL_MODE="single"
            ;;
        *)
            echo -e "${YELLOW}Unknown selection, using single-instance uninstall${NC}"
            UNINSTALL_MODE="single"
            ;;
    esac
}

ask_yes_no() {
    local prompt="$1"
    local reply

    if [ ! -t 0 ]; then
        return 1
    fi

    read -p "$prompt [y/N] " -n 1 -r reply
    echo
    [[ "$reply" =~ ^[Yy]$ ]]
}

stop_disable_unit() {
    local unit="$1"
    systemctl stop "$unit" 2>/dev/null || true
    systemctl disable "$unit" 2>/dev/null || true
}

unmount_tree() {
    local root="$1"

    if [ ! -d "$root" ]; then
        return
    fi

    while IFS= read -r mount_path; do
        [ -n "$mount_path" ] || continue
        umount "$mount_path" 2>/dev/null || true
    done < <(mount | awk -v prefix="$root" '$3 ~ ("^" prefix) { print $3 }' | sort -r)
}

remove_dual_routing() {
    echo "Removing dual-NIC routing helper..."

    if [ -x "$DUAL_ROUTING_INSTALLER" ]; then
        "$DUAL_ROUTING_INSTALLER" --remove 2>/dev/null || true
    elif [ -x "$DUAL_ROUTING_SCRIPT" ]; then
        "$DUAL_ROUTING_SCRIPT" --remove 2>/dev/null || true
    else
        stop_disable_unit ucxsync-dualnic-routing.service
        rm -f "$DUAL_ROUTING_SERVICE"
        rm -f "$DUAL_SYSCTL_FILE"
        rm -rf "$DUAL_SINGLE_DROPIN_DIR"
        rm -rf "$DUAL_TEMPLATE_DROPIN_DIR"
        systemctl daemon-reload 2>/dev/null || true
        sysctl --system >/dev/null 2>&1 || true
    fi

    rm -f "$DUAL_ROUTING_SCRIPT"
    echo -e "${GREEN}✓${NC} Dual-NIC routing helper removed"
}

remove_empty_config_dir() {
    if [ -d /etc/ucxsync ] && [ -z "$(find /etc/ucxsync -mindepth 1 -maxdepth 1 2>/dev/null)" ]; then
        rmdir /etc/ucxsync 2>/dev/null || true
    fi
}

dual_routing_exists() {
    [ -f "$DUAL_ROUTING_SERVICE" ] || [ -f "$DUAL_ROUTING_SCRIPT" ] || [ -f "$DUAL_ROUTING_INSTALLER" ] || [ -d "$DUAL_SINGLE_DROPIN_DIR" ] || [ -d "$DUAL_TEMPLATE_DROPIN_DIR" ] || [ -f "$DUAL_SYSCTL_FILE" ]
}

parse_args "$@"
prompt_uninstall_mode

echo -e "${RED}UCXSync Uninstallation${NC}"
echo "======================================"
echo -e "${BLUE}Mode: ${UNINSTALL_MODE}${NC}"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    echo "Please run: sudo ./uninstall.sh"
    exit 1
fi

echo "Stopping services..."
case "$UNINSTALL_MODE" in
    single)
        stop_disable_unit ucxsync
        ;;
    dual)
        stop_disable_unit ucxsync@a
        stop_disable_unit ucxsync@b
        stop_disable_unit ucxsync-dualnic-routing.service
        ;;
    all)
        stop_disable_unit ucxsync
        stop_disable_unit ucxsync@a
        stop_disable_unit ucxsync@b
        stop_disable_unit ucxsync-dualnic-routing.service
        ;;
esac
echo -e "${GREEN}✓${NC} Services stopped"

echo ""
echo "Unmounting network shares..."
case "$UNINSTALL_MODE" in
    single)
        unmount_tree /ucmount
        ;;
    dual)
        unmount_tree /ucmount-a
        unmount_tree /ucmount-b
        ;;
    all)
        unmount_tree /ucmount
        unmount_tree /ucmount-a
        unmount_tree /ucmount-b
        ;;
esac
echo -e "${GREEN}✓${NC} Shares unmounted"

echo ""
echo "Removing service files..."
case "$UNINSTALL_MODE" in
    single)
        rm -f /etc/systemd/system/ucxsync.service
        ;;
    dual)
        rm -f /etc/systemd/system/ucxsync@.service
        ;;
    all)
        rm -f /etc/systemd/system/ucxsync.service
        rm -f /etc/systemd/system/ucxsync@.service
        ;;
esac
systemctl daemon-reload
echo -e "${GREEN}✓${NC} Service files removed"

if dual_routing_exists; then
    echo ""
    if [ "$UNINSTALL_MODE" = "all" ]; then
        if ask_yes_no "Remove host-wide dual-NIC routing helper/service too?"; then
            remove_dual_routing
        else
            echo -e "${YELLOW}⚠${NC}  Dual-NIC routing preserved"
        fi
    else
        if ask_yes_no "Remove host-wide dual-NIC routing helper/service?"; then
            remove_dual_routing
        else
            echo -e "${YELLOW}⚠${NC}  Dual-NIC routing preserved"
        fi
    fi
fi

echo ""
if [ "$UNINSTALL_MODE" = "single" ]; then
    if ask_yes_no "Remove single-instance configuration /etc/ucxsync/config.yaml?"; then
        rm -f /etc/ucxsync/config.yaml
        echo -e "${GREEN}✓${NC} Single-instance configuration removed"
    else
        echo -e "${YELLOW}⚠${NC}  Single-instance configuration preserved"
    fi
elif [ "$UNINSTALL_MODE" = "dual" ]; then
    if ask_yes_no "Remove dual-instance configurations /etc/ucxsync/a.yaml and /etc/ucxsync/b.yaml?"; then
        rm -f /etc/ucxsync/a.yaml /etc/ucxsync/b.yaml
        echo -e "${GREEN}✓${NC} Dual-instance configurations removed"
    else
        echo -e "${YELLOW}⚠${NC}  Dual-instance configurations preserved"
    fi
else
    if ask_yes_no "Remove all configuration from /etc/ucxsync?"; then
        rm -rf /etc/ucxsync
        echo -e "${GREEN}✓${NC} All configuration removed"
    else
        echo -e "${YELLOW}⚠${NC}  Configuration preserved at /etc/ucxsync"
    fi
fi
remove_empty_config_dir

echo ""
if [ "$UNINSTALL_MODE" = "single" ]; then
    if ask_yes_no "Remove mount directory /ucmount?"; then
        rm -rf /ucmount
        echo -e "${GREEN}✓${NC} Mount directory removed"
    else
        echo -e "${YELLOW}⚠${NC}  Mount directory preserved at /ucmount"
    fi
elif [ "$UNINSTALL_MODE" = "dual" ]; then
    if ask_yes_no "Remove dual mount directories /ucmount-a and /ucmount-b?"; then
        rm -rf /ucmount-a /ucmount-b
        echo -e "${GREEN}✓${NC} Dual mount directories removed"
    else
        echo -e "${YELLOW}⚠${NC}  Dual mount directories preserved"
    fi
else
    if ask_yes_no "Remove all mount directories (/ucmount, /ucmount-a, /ucmount-b)?"; then
        rm -rf /ucmount /ucmount-a /ucmount-b
        echo -e "${GREEN}✓${NC} All mount directories removed"
    else
        echo -e "${YELLOW}⚠${NC}  Mount directories preserved"
    fi
fi

echo ""
if [ "$UNINSTALL_MODE" = "all" ]; then
    if ask_yes_no "Remove application files from /opt/ucxsync and logs from /var/log/ucxsync?"; then
        rm -rf /opt/ucxsync
        rm -rf /var/log/ucxsync
        rm -f /usr/local/bin/ucxsync
        echo -e "${GREEN}✓${NC} Shared application files removed"
    else
        echo -e "${YELLOW}⚠${NC}  Shared application files preserved"
    fi
else
    echo -e "${YELLOW}⚠${NC}  Shared application files preserved (/opt/ucxsync, /var/log/ucxsync)"
fi

echo ""
if ask_yes_no "Remove data directory /ucdata (this deletes synchronized data)?"; then
    echo -e "${RED}⚠ WARNING: This will delete all synchronized data!${NC}"
    if ask_yes_no "Are you absolutely sure?"; then
        rm -rf /ucdata/ucx
        rm -rf /ucdata
        echo -e "${GREEN}✓${NC} Data directory removed"
    else
        echo -e "${YELLOW}⚠${NC}  Data directory preserved at /ucdata"
    fi
else
    echo -e "${YELLOW}⚠${NC}  Data directory preserved at /ucdata"
fi

echo ""
if [ "$UNINSTALL_MODE" = "all" ]; then
    if ask_yes_no "Remove dependencies (Go, cifs-utils)?"; then
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
else
    echo -e "${YELLOW}⚠${NC}  Dependencies preserved (only removed in --all mode)"
fi

echo ""
echo "======================================"
echo -e "${GREEN}Uninstallation complete!${NC}"
echo "======================================"
echo ""
echo "To reinstall UCXSync:"
echo "  git clone https://github.com/zangezia/UCXSync.git"
echo "  cd UCXSync"
echo "  chmod +x install.sh"
echo "  sudo ./install.sh --single   # or --dual"
echo ""
