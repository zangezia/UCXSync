#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME=$(basename "$0")
INSTALL_PATH="/usr/local/sbin/ucxsync-dualnic-routing.sh"
SERVICE_PATH="/etc/systemd/system/ucxsync-dualnic-routing.service"
SYSCTL_PATH="/etc/sysctl.d/99-ucxsync-dualnic.conf"

# Override via environment if needed.
END0_IFACE="${END0_IFACE:-end0}"
END1_IFACE="${END1_IFACE:-end1}"
END0_SRC_IP="${END0_SRC_IP:-192.168.200.101}"
END1_SRC_IP="${END1_SRC_IP:-192.168.200.102}"
END0_HOSTS_RAW="${END0_HOSTS:-1 2 3 4 5 6 7}"
END1_HOSTS_RAW="${END1_HOSTS:-8 9 10 11 12 13 201}"
IP_PREFIX="${IP_PREFIX:-192.168.200}"

DRY_RUN=0
MODE="apply"

log() {
    printf '[%s] %s\n' "$SCRIPT_NAME" "$*"
}

warn() {
    printf '[%s] WARNING: %s\n' "$SCRIPT_NAME" "$*" >&2
}

die() {
    printf '[%s] ERROR: %s\n' "$SCRIPT_NAME" "$*" >&2
    exit 1
}

usage() {
    cat <<EOF
Usage:
  sudo ./$SCRIPT_NAME [--apply] [--dry-run]
  sudo ./$SCRIPT_NAME --install [--dry-run]
  sudo ./$SCRIPT_NAME --remove [--dry-run]
  ./$SCRIPT_NAME --print

What it does:
  --apply    Apply runtime sysctl tuning and host routes now (default)
  --install  Install persistent sysctl + systemd oneshot service and apply now
  --remove   Remove persistent config/service and delete configured host routes
  --print    Print the computed configuration without changing the system

Environment overrides:
  END0_IFACE=end0
  END1_IFACE=end1
  END0_SRC_IP=192.168.200.101
  END1_SRC_IP=192.168.200.102
  END0_HOSTS="1 2 3 4 5 6 7"
  END1_HOSTS="8 9 10 11 12 13 201"
  IP_PREFIX=192.168.200

Host values may be either last octets (for example 7 or 201) or full IPv4 addresses.
EOF
}

run_cmd() {
    if [[ "$DRY_RUN" -eq 1 ]]; then
        printf '[dry-run]'
        printf ' %q' "$@"
        printf '\n'
        return 0
    fi

    "$@"
}

need_root() {
    if [[ "${EUID}" -ne 0 ]]; then
        die "run as root (sudo)"
    fi
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --apply)
                MODE="apply"
                ;;
            --install)
                MODE="install"
                ;;
            --remove)
                MODE="remove"
                ;;
            --print)
                MODE="print"
                ;;
            --dry-run)
                DRY_RUN=1
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                die "unknown argument: $1"
                ;;
        esac
        shift
    done
}

split_hosts() {
    local raw="$1"
    read -r -a HOSTS <<< "$raw"
    printf '%s\n' "${HOSTS[@]}"
}

normalize_ip() {
    local value="$1"
    if [[ "$value" == *.* ]]; then
        printf '%s\n' "$value"
    else
        printf '%s.%s\n' "$IP_PREFIX" "$value"
    fi
}

require_interface() {
    local iface="$1"
    ip link show dev "$iface" >/dev/null 2>&1 || die "interface not found: $iface"
}

print_config() {
    cat <<EOF
Configuration:
  end0 interface : $END0_IFACE
  end0 source IP : $END0_SRC_IP
  end0 hosts     : $END0_HOSTS_RAW
  end1 interface : $END1_IFACE
  end1 source IP : $END1_SRC_IP
  end1 hosts     : $END1_HOSTS_RAW
  subnet prefix  : $IP_PREFIX
EOF
}

apply_runtime_sysctl() {
    log "Applying runtime sysctl tuning"
    run_cmd sysctl -w net.ipv4.conf.all.rp_filter=2
    run_cmd sysctl -w net.ipv4.conf.default.rp_filter=2
    run_cmd sysctl -w "net.ipv4.conf.${END0_IFACE}.rp_filter=2"
    run_cmd sysctl -w "net.ipv4.conf.${END1_IFACE}.rp_filter=2"
    run_cmd sysctl -w net.ipv4.conf.all.arp_ignore=1
    run_cmd sysctl -w net.ipv4.conf.all.arp_announce=2
    run_cmd sysctl -w "net.ipv4.conf.${END0_IFACE}.arp_filter=1"
    run_cmd sysctl -w "net.ipv4.conf.${END1_IFACE}.arp_filter=1"
}

write_persistent_sysctl() {
    log "Writing persistent sysctl config to $SYSCTL_PATH"
    if [[ "$DRY_RUN" -eq 1 ]]; then
        cat <<EOF
[dry-run] would write $SYSCTL_PATH
net.ipv4.conf.all.rp_filter=2
net.ipv4.conf.default.rp_filter=2
net.ipv4.conf.${END0_IFACE}.rp_filter=2
net.ipv4.conf.${END1_IFACE}.rp_filter=2
net.ipv4.conf.all.arp_ignore=1
net.ipv4.conf.all.arp_announce=2
net.ipv4.conf.${END0_IFACE}.arp_filter=1
net.ipv4.conf.${END1_IFACE}.arp_filter=1
EOF
        return 0
    fi

    cat > "$SYSCTL_PATH" <<EOF
net.ipv4.conf.all.rp_filter=2
net.ipv4.conf.default.rp_filter=2
net.ipv4.conf.${END0_IFACE}.rp_filter=2
net.ipv4.conf.${END1_IFACE}.rp_filter=2
net.ipv4.conf.all.arp_ignore=1
net.ipv4.conf.all.arp_announce=2
net.ipv4.conf.${END0_IFACE}.arp_filter=1
net.ipv4.conf.${END1_IFACE}.arp_filter=1
EOF
}

apply_route_group() {
    local iface="$1"
    local src_ip="$2"
    shift 2
    local hosts=("$@")
    local host
    local ip

    for host in "${hosts[@]}"; do
        [[ -n "$host" ]] || continue
        ip=$(normalize_ip "$host")
        log "Routing $ip/32 via $iface (src $src_ip)"
        run_cmd ip route replace "$ip/32" dev "$iface" src "$src_ip"
    done
}

delete_route_group() {
    local iface="$1"
    shift
    local hosts=("$@")
    local host
    local ip

    for host in "${hosts[@]}"; do
        [[ -n "$host" ]] || continue
        ip=$(normalize_ip "$host")
        log "Removing route $ip/32 from $iface"
        if [[ "$DRY_RUN" -eq 1 ]]; then
            printf '[dry-run] %q %q %q %q %q\n' ip route del "$ip/32" dev "$iface"
            continue
        fi
        ip route del "$ip/32" dev "$iface" >/dev/null 2>&1 || true
    done
}

verify_routes() {
    local hosts_to_check=(1 7 8 13 201)
    local host
    local ip

    log "Computed route checks"
    for host in "${hosts_to_check[@]}"; do
        ip=$(normalize_ip "$host")
        if [[ "$DRY_RUN" -eq 1 ]]; then
            printf '[dry-run] %q %q %q\n' ip route get "$ip"
        else
            ip route get "$ip" || true
        fi
    done
}

install_service() {
    log "Installing helper script to $INSTALL_PATH"
    run_cmd install -m 0755 "$0" "$INSTALL_PATH"

    log "Writing systemd service to $SERVICE_PATH"
    if [[ "$DRY_RUN" -eq 1 ]]; then
        cat <<EOF
[dry-run] would write $SERVICE_PATH
[Unit]
Description=UCXSync dual-NIC routing setup
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
Environment=END0_IFACE=$END0_IFACE
Environment=END1_IFACE=$END1_IFACE
Environment=END0_SRC_IP=$END0_SRC_IP
Environment=END1_SRC_IP=$END1_SRC_IP
Environment=END0_HOSTS=$END0_HOSTS_RAW
Environment=END1_HOSTS=$END1_HOSTS_RAW
Environment=IP_PREFIX=$IP_PREFIX
ExecStart=$INSTALL_PATH --apply
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF
        return 0
    fi

    cat > "$SERVICE_PATH" <<EOF
[Unit]
Description=UCXSync dual-NIC routing setup
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
Environment=END0_IFACE=$END0_IFACE
Environment=END1_IFACE=$END1_IFACE
Environment=END0_SRC_IP=$END0_SRC_IP
Environment=END1_SRC_IP=$END1_SRC_IP
Environment=END0_HOSTS=$END0_HOSTS_RAW
Environment=END1_HOSTS=$END1_HOSTS_RAW
Environment=IP_PREFIX=$IP_PREFIX
ExecStart=$INSTALL_PATH --apply
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

    run_cmd systemctl daemon-reload
    run_cmd systemctl enable --now ucxsync-dualnic-routing.service
}

remove_installation() {
    log "Removing persistent dual-NIC routing setup"

    if [[ "$DRY_RUN" -eq 0 ]]; then
        systemctl disable --now ucxsync-dualnic-routing.service >/dev/null 2>&1 || true
    else
        printf '[dry-run] %q %q %q\n' systemctl disable --now ucxsync-dualnic-routing.service
    fi

    if [[ -f "$SERVICE_PATH" ]]; then
        run_cmd rm -f "$SERVICE_PATH"
    fi
    if [[ -f "$INSTALL_PATH" ]]; then
        run_cmd rm -f "$INSTALL_PATH"
    fi
    if [[ -f "$SYSCTL_PATH" ]]; then
        run_cmd rm -f "$SYSCTL_PATH"
    fi

    if [[ "$DRY_RUN" -eq 0 ]]; then
        systemctl daemon-reload || true
        sysctl --system >/dev/null 2>&1 || true
    else
        printf '[dry-run] %q %q\n' systemctl daemon-reload
        printf '[dry-run] %q %q\n' sysctl --system
    fi
}

main() {
    parse_args "$@"

    case "$MODE" in
        print)
            print_config
            exit 0
            ;;
        apply|install|remove)
            need_root
            require_interface "$END0_IFACE"
            require_interface "$END1_IFACE"
            ;;
    esac

    mapfile -t END0_HOSTS_ARRAY < <(split_hosts "$END0_HOSTS_RAW")
    mapfile -t END1_HOSTS_ARRAY < <(split_hosts "$END1_HOSTS_RAW")

    print_config

    case "$MODE" in
        apply)
            apply_runtime_sysctl
            apply_route_group "$END0_IFACE" "$END0_SRC_IP" "${END0_HOSTS_ARRAY[@]}"
            apply_route_group "$END1_IFACE" "$END1_SRC_IP" "${END1_HOSTS_ARRAY[@]}"
            verify_routes
            ;;
        install)
            apply_runtime_sysctl
            apply_route_group "$END0_IFACE" "$END0_SRC_IP" "${END0_HOSTS_ARRAY[@]}"
            apply_route_group "$END1_IFACE" "$END1_SRC_IP" "${END1_HOSTS_ARRAY[@]}"
            write_persistent_sysctl
            if [[ "$DRY_RUN" -eq 0 ]]; then
                sysctl --system >/dev/null
            else
                printf '[dry-run] %q %q\n' sysctl --system
            fi
            install_service
            verify_routes
            ;;
        remove)
            delete_route_group "$END0_IFACE" "${END0_HOSTS_ARRAY[@]}"
            delete_route_group "$END1_IFACE" "${END1_HOSTS_ARRAY[@]}"
            remove_installation
            ;;
    esac

    log "Done"
}

main "$@"