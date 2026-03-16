#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME=$(basename "$0")
INSTALL_PATH="/usr/local/sbin/ucxsync-dualnic-routing.sh"
SERVICE_PATH="/etc/systemd/system/ucxsync-dualnic-routing.service"
SYSCTL_PATH="/etc/sysctl.d/99-ucxsync-dualnic.conf"
UCXSYNC_SINGLE_DROPIN_DIR="/etc/systemd/system/ucxsync.service.d"
UCXSYNC_SINGLE_DROPIN_PATH="$UCXSYNC_SINGLE_DROPIN_DIR/10-dualnic-routing.conf"
UCXSYNC_TEMPLATE_DROPIN_DIR="/etc/systemd/system/ucxsync@.service.d"
UCXSYNC_TEMPLATE_DROPIN_PATH="$UCXSYNC_TEMPLATE_DROPIN_DIR/10-dualnic-routing.conf"

# Override via environment if needed.
END0_IFACE="${END0_IFACE:-end0}"
END1_IFACE="${END1_IFACE:-end1}"
END0_SRC_IP="${END0_SRC_IP:-192.168.200.101}"
END1_SRC_IP="${END1_SRC_IP:-192.168.200.102}"
END0_HOSTS_RAW="${END0_HOSTS:-1 2 3 4 5 6 7}"
END1_HOSTS_RAW="${END1_HOSTS:-8 9 10 11 12 13 201}"
IP_PREFIX="${IP_PREFIX:-192.168.200}"
IP_WAIT_TIMEOUT="${IP_WAIT_TIMEOUT:-30}"
NET_CORE_RMEM_MAX="${NET_CORE_RMEM_MAX:-134217728}"
NET_CORE_WMEM_MAX="${NET_CORE_WMEM_MAX:-134217728}"
TCP_RMEM="${TCP_RMEM:-4096 262144 67108864}"
TCP_WMEM="${TCP_WMEM:-4096 262144 67108864}"
NETDEV_MAX_BACKLOG="${NETDEV_MAX_BACKLOG:-250000}"
PIN_IRQS="${PIN_IRQS:-0}"
END0_IRQ_CORES="${END0_IRQ_CORES:-0}"
END1_IRQ_CORES="${END1_IRQ_CORES:-1}"
END0_IRQS_RAW="${END0_IRQS:-}"
END1_IRQS_RAW="${END1_IRQS:-}"

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

The install mode also wires routing to start before both:
    - ucxsync.service
    - ucxsync@.service (all template instances)

Environment overrides:
  END0_IFACE=end0
  END1_IFACE=end1
  END0_SRC_IP=192.168.200.101
  END1_SRC_IP=192.168.200.102
  END0_HOSTS="1 2 3 4 5 6 7"
  END1_HOSTS="8 9 10 11 12 13 201"
  IP_PREFIX=192.168.200
    NET_CORE_RMEM_MAX=134217728
    NET_CORE_WMEM_MAX=134217728
    TCP_RMEM="4096 262144 67108864"
    TCP_WMEM="4096 262144 67108864"
    NETDEV_MAX_BACKLOG=250000
    PIN_IRQS=1
    END0_IRQ_CORES=0
    END1_IRQ_CORES=1
    END0_IRQS="78"
    END1_IRQS="79"

Host values may be either last octets (for example 7 or 201) or full IPv4 addresses.
If PIN_IRQS=1, the script will try to pin all IRQs whose labels mention END0_IFACE/END1_IFACE.
Set END0_IRQS / END1_IRQS to explicit IRQ numbers if you want deterministic pinning.
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

write_text_file() {
    local path="$1"
    local value="$2"

    if [[ "$DRY_RUN" -eq 1 ]]; then
        printf '[dry-run] write %s <= %s\n' "$path" "$value"
        return 0
    fi

    printf '%s\n' "$value" > "$path"
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

split_words() {
    local raw="$1"
    read -r -a WORDS <<< "$raw"
    printf '%s\n' "${WORDS[@]}"
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

interface_has_ip() {
    local iface="$1"
    local ip_addr="$2"

    ip -4 -o addr show dev "$iface" | grep -Fq " $ip_addr/"
}

wait_for_interface_ip() {
    local iface="$1"
    local ip_addr="$2"
    local timeout="$3"
    local elapsed=0

    while (( elapsed < timeout )); do
        if interface_has_ip "$iface" "$ip_addr"; then
            log "Confirmed IP $ip_addr on $iface"
            return 0
        fi

        sleep 1
        elapsed=$((elapsed + 1))
    done

    return 1
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
    tcp rmem       : $TCP_RMEM
    tcp wmem       : $TCP_WMEM
    rmem_max       : $NET_CORE_RMEM_MAX
    wmem_max       : $NET_CORE_WMEM_MAX
    backlog        : $NETDEV_MAX_BACKLOG
    pin irqs       : $PIN_IRQS
    end0 irq cores : $END0_IRQ_CORES
    end1 irq cores : $END1_IRQ_CORES
    end0 irqs      : ${END0_IRQS_RAW:-auto}
    end1 irqs      : ${END1_IRQS_RAW:-auto}
EOF
}

apply_runtime_sysctl() {
    log "Applying runtime sysctl tuning"
        run_cmd sysctl -w "net.core.rmem_max=${NET_CORE_RMEM_MAX}"
        run_cmd sysctl -w "net.core.wmem_max=${NET_CORE_WMEM_MAX}"
        run_cmd sysctl -w "net.core.netdev_max_backlog=${NETDEV_MAX_BACKLOG}"
        run_cmd sysctl -w "net.ipv4.tcp_rmem=${TCP_RMEM}"
        run_cmd sysctl -w "net.ipv4.tcp_wmem=${TCP_WMEM}"
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
net.core.rmem_max=${NET_CORE_RMEM_MAX}
net.core.wmem_max=${NET_CORE_WMEM_MAX}
net.core.netdev_max_backlog=${NETDEV_MAX_BACKLOG}
net.ipv4.tcp_rmem=${TCP_RMEM}
net.ipv4.tcp_wmem=${TCP_WMEM}
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
net.core.rmem_max=${NET_CORE_RMEM_MAX}
net.core.wmem_max=${NET_CORE_WMEM_MAX}
net.core.netdev_max_backlog=${NETDEV_MAX_BACKLOG}
net.ipv4.tcp_rmem=${TCP_RMEM}
net.ipv4.tcp_wmem=${TCP_WMEM}
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

find_interface_irqs() {
    local iface="$1"
    grep -F "$iface" /proc/interrupts | awk -F: '{gsub(/^[[:space:]]+/, "", $1); print $1}'
}

resolve_irqs() {
    local explicit_raw="$1"
    local iface="$2"

    if [[ -n "$explicit_raw" ]]; then
        split_words "$explicit_raw"
        return 0
    fi

    find_interface_irqs "$iface"
}

warn_if_irqbalance_active() {
    if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet irqbalance; then
        warn "irqbalance is active and may override manual IRQ affinity; consider disabling or constraining irqbalance"
    fi
}

pin_interface_irqs() {
    local iface="$1"
    local core_list="$2"
    local explicit_irqs_raw="$3"
    local irq
    local found=0
    local irq_count=0

    while IFS= read -r irq; do
        [[ -n "$irq" ]] || continue
        found=1
        irq_count=$((irq_count + 1))
        log "Pinning IRQ $irq for $iface to CPU list $core_list"
        write_text_file "/proc/irq/${irq}/smp_affinity_list" "$core_list"
    done < <(resolve_irqs "$explicit_irqs_raw" "$iface")

    if [[ "$found" -eq 0 ]]; then
        warn "No IRQs found for interface $iface in /proc/interrupts"
    elif [[ -z "$explicit_irqs_raw" && "$irq_count" -gt 1 ]]; then
        warn "Auto-detected $irq_count IRQs for $iface; pinning all of them to the same CPU list can reduce throughput on multiqueue NICs. Consider explicit END0_IRQS/END1_IRQS values."
    fi
}

apply_irq_affinity() {
    if [[ "$PIN_IRQS" != "1" ]]; then
        return 0
    fi

    warn_if_irqbalance_active
    pin_interface_irqs "$END0_IFACE" "$END0_IRQ_CORES" "$END0_IRQS_RAW"
    pin_interface_irqs "$END1_IFACE" "$END1_IRQ_CORES" "$END1_IRQS_RAW"
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

        if interface_has_ip "$iface" "$src_ip"; then
            run_cmd ip route replace "$ip/32" dev "$iface" src "$src_ip"
        else
            warn "IP $src_ip is not assigned to $iface yet; applying route without explicit src"
            run_cmd ip route replace "$ip/32" dev "$iface"
        fi
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
Before=ucxsync.service ucxsync@.service

[Service]
Type=oneshot
Environment=END0_IFACE=$END0_IFACE
Environment=END1_IFACE=$END1_IFACE
Environment=END0_SRC_IP=$END0_SRC_IP
Environment=END1_SRC_IP=$END1_SRC_IP
Environment="END0_HOSTS=$END0_HOSTS_RAW"
Environment="END1_HOSTS=$END1_HOSTS_RAW"
Environment=IP_PREFIX=$IP_PREFIX
Environment=NET_CORE_RMEM_MAX=$NET_CORE_RMEM_MAX
Environment=NET_CORE_WMEM_MAX=$NET_CORE_WMEM_MAX
Environment="TCP_RMEM=$TCP_RMEM"
Environment="TCP_WMEM=$TCP_WMEM"
Environment=NETDEV_MAX_BACKLOG=$NETDEV_MAX_BACKLOG
Environment=PIN_IRQS=$PIN_IRQS
Environment=END0_IRQ_CORES=$END0_IRQ_CORES
Environment=END1_IRQ_CORES=$END1_IRQ_CORES
Environment="END0_IRQS=$END0_IRQS_RAW"
Environment="END1_IRQS=$END1_IRQS_RAW"
ExecStart=$INSTALL_PATH --apply
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

        cat <<EOF
[dry-run] would write $UCXSYNC_SINGLE_DROPIN_PATH
[Unit]
Wants=ucxsync-dualnic-routing.service
After=ucxsync-dualnic-routing.service
EOF

    cat <<EOF
[dry-run] would write $UCXSYNC_TEMPLATE_DROPIN_PATH
[Unit]
Wants=ucxsync-dualnic-routing.service
After=ucxsync-dualnic-routing.service
EOF
        return 0
    fi

    cat > "$SERVICE_PATH" <<EOF
[Unit]
Description=UCXSync dual-NIC routing setup
After=network-online.target
Wants=network-online.target
Before=ucxsync.service ucxsync@.service

[Service]
Type=oneshot
Environment=END0_IFACE=$END0_IFACE
Environment=END1_IFACE=$END1_IFACE
Environment=END0_SRC_IP=$END0_SRC_IP
Environment=END1_SRC_IP=$END1_SRC_IP
Environment="END0_HOSTS=$END0_HOSTS_RAW"
Environment="END1_HOSTS=$END1_HOSTS_RAW"
Environment=IP_PREFIX=$IP_PREFIX
Environment=NET_CORE_RMEM_MAX=$NET_CORE_RMEM_MAX
Environment=NET_CORE_WMEM_MAX=$NET_CORE_WMEM_MAX
Environment="TCP_RMEM=$TCP_RMEM"
Environment="TCP_WMEM=$TCP_WMEM"
Environment=NETDEV_MAX_BACKLOG=$NETDEV_MAX_BACKLOG
Environment=PIN_IRQS=$PIN_IRQS
Environment=END0_IRQ_CORES=$END0_IRQ_CORES
Environment=END1_IRQ_CORES=$END1_IRQ_CORES
Environment="END0_IRQS=$END0_IRQS_RAW"
Environment="END1_IRQS=$END1_IRQS_RAW"
ExecStart=$INSTALL_PATH --apply
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

    log "Writing UCXSync ordering drop-in to $UCXSYNC_SINGLE_DROPIN_PATH"
    run_cmd mkdir -p "$UCXSYNC_SINGLE_DROPIN_DIR"
    cat > "$UCXSYNC_SINGLE_DROPIN_PATH" <<EOF
[Unit]
Wants=ucxsync-dualnic-routing.service
After=ucxsync-dualnic-routing.service
EOF

    log "Writing UCXSync template ordering drop-in to $UCXSYNC_TEMPLATE_DROPIN_PATH"
    run_cmd mkdir -p "$UCXSYNC_TEMPLATE_DROPIN_DIR"
    cat > "$UCXSYNC_TEMPLATE_DROPIN_PATH" <<EOF
[Unit]
Wants=ucxsync-dualnic-routing.service
After=ucxsync-dualnic-routing.service
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
    if [[ -f "$UCXSYNC_SINGLE_DROPIN_PATH" ]]; then
        run_cmd rm -f "$UCXSYNC_SINGLE_DROPIN_PATH"
    fi
    if [[ -f "$UCXSYNC_TEMPLATE_DROPIN_PATH" ]]; then
        run_cmd rm -f "$UCXSYNC_TEMPLATE_DROPIN_PATH"
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
            if ! wait_for_interface_ip "$END0_IFACE" "$END0_SRC_IP" "$IP_WAIT_TIMEOUT"; then
                warn "Timed out waiting for $END0_SRC_IP on $END0_IFACE"
            fi
            if ! wait_for_interface_ip "$END1_IFACE" "$END1_SRC_IP" "$IP_WAIT_TIMEOUT"; then
                warn "Timed out waiting for $END1_SRC_IP on $END1_IFACE"
            fi
            apply_runtime_sysctl
            apply_irq_affinity
            apply_route_group "$END0_IFACE" "$END0_SRC_IP" "${END0_HOSTS_ARRAY[@]}"
            apply_route_group "$END1_IFACE" "$END1_SRC_IP" "${END1_HOSTS_ARRAY[@]}"
            verify_routes
            ;;
        install)
            if ! wait_for_interface_ip "$END0_IFACE" "$END0_SRC_IP" "$IP_WAIT_TIMEOUT"; then
                warn "Timed out waiting for $END0_SRC_IP on $END0_IFACE"
            fi
            if ! wait_for_interface_ip "$END1_IFACE" "$END1_SRC_IP" "$IP_WAIT_TIMEOUT"; then
                warn "Timed out waiting for $END1_SRC_IP on $END1_IFACE"
            fi
            apply_runtime_sysctl
            apply_irq_affinity
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