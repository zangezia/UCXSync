# Installation Guide - UCXSync for Linux

## Prerequisites

### System Requirements
- Ubuntu 20.04+ or compatible Linux distribution
- Root/sudo access
- Network connectivity to UCX nodes
- Minimum 4 GB RAM
- Minimum 50 GB free disk space

### Required Packages
```bash
sudo apt-get update
sudo apt-get install -y cifs-utils build-essential
```

### Go Installation (if building from source)
```bash
# Download and install Go 1.21+
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz

# Add to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify installation
go version
```

## Installation Methods

### Method 1: Automated Installation Script

```bash
# Clone repository
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync


# Make install script executable
chmod +x install.sh

# Run installation (requires sudo)
sudo ./install.sh
```

By default, the installer keeps backward compatibility and installs the **main single-instance version**.

You can explicitly choose the installation mode:

```bash
# Main version (single instance)
sudo ./install.sh --single

# Main version + dual-NIC routing helper
sudo ./install.sh --single --install-dualnic-routing

# Dual version (two instances: ucxsync@a + ucxsync@b)
sudo ./install.sh --dual

# Dual version + routing helper installed immediately
sudo ./install.sh --dual --install-dualnic-routing

# Equivalent explicit form
sudo ./install.sh --mode dual
```

For custom interface names and source IPs, pass environment overrides into the installer. Example:

```bash
sudo END0_IFACE=end0 \
  END1_IFACE=enx00e04c141b68 \
  END0_SRC_IP=192.168.200.101 \
  END1_SRC_IP=192.168.200.103 \
  END0_HOSTS="1 2 3 4 5 6 7" \
  END1_HOSTS="8 9 10 11 12 13 201" \
  ./install.sh --single --install-dualnic-routing
```

When run interactively without flags, the script will ask which version to install.

If you enable dual-NIC routing in interactive mode, the installer also launches a small wizard that asks for:

- primary interface name;
- secondary interface name;
- source IPv4 for each interface;
- which UCX hosts should be routed via each interface.

So for common cases like `end0 + enx00e04c141b68`, you can just answer the prompts instead of building a long command line.

The script will:
- Check and install prerequisites
- Build the application
- Create necessary directories
- Install binary to `/opt/ucxsync/`
- Install web assets to `/opt/ucxsync/web/`
- Configure network hosts mapping in `/etc/hosts` (192.168.200.1-13, 201)
- Install systemd service(s)
- Set up configuration for the selected mode

In **single** mode it installs:

- `/etc/ucxsync/config.yaml`
- `/etc/systemd/system/ucxsync.service`
- mount root `/ucmount`

In **dual** mode it installs:

- `/etc/ucxsync/a.yaml`
- `/etc/ucxsync/b.yaml`
- `/etc/systemd/system/ucxsync.service`
- `/etc/systemd/system/ucxsync@.service`
- mount roots `/ucmount-a` and `/ucmount-b`
- helper `/opt/ucxsync/setup-dualnic-routing.sh`

If `--install-dualnic-routing` is provided, the installer also runs the helper and enables:

- `/etc/systemd/system/ucxsync-dualnic-routing.service`
- `/etc/sysctl.d/99-ucxsync-dualnic.conf`
- service ordering drop-ins for `ucxsync.service` and `ucxsync@.service`

### Method 2: Manual Installation

```bash
# Clone repository
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync

# Build
go build -o ucxsync ./cmd/ucxsync

# Create directories
sudo mkdir -p /opt/ucxsync
sudo mkdir -p /etc/ucxsync
sudo mkdir -p /var/log/ucxsync
sudo mkdir -p /ucmount

# Install binary
sudo cp ucxsync /opt/ucxsync/
sudo chmod +x /opt/ucxsync/ucxsync

# Install configuration
sudo cp config.example.yaml /etc/ucxsync/config.yaml

# Install systemd service
sudo cp ucxsync.service /etc/systemd/system/
sudo systemctl daemon-reload

# Set permissions
sudo chown -R root:root /opt/ucxsync
sudo chown -R root:root /etc/ucxsync
sudo chmod 700 /etc/ucxsync
sudo chmod 600 /etc/ucxsync/config.yaml
```

## Configuration

### Edit Main Configuration

```bash
sudo nano /etc/ucxsync/config.yaml
```

Key settings to configure:

```yaml
# Network credentials
credentials:
  username: Administrator
  password: your_password_here

# Synchronization
sync:
  max_parallelism: 8  # Adjust based on system resources

# Web interface
web:
  host: 0.0.0.0  # Listen on all interfaces
  port: 8080

# Logging
logging:
  level: info  # debug, info, warn, error
  file: /var/log/ucxsync/ucxsync.log
```

### Network Share Mounting

UCXSync automatically mounts network shares on startup. The mount structure:

```
/ucmount/
├── WU01/
│   ├── E/  (mounted from //WU01/E)
│   └── F/  (mounted from //WU01/F)
├── WU02/
│   ├── E/
│   └── F/
...
└── CU/
    ├── E/
    └── F/
```

### Two-instance deployment scheme

For legacy SMB1 sources, the recommended scaling model is **two independent UCXSync instances**.

Recommended split:

- **Instance A**
  - nodes: `WU01`-`WU07`
  - interface: `end0`
  - mount root: `/ucmount-a`
  - web UI: `:8080` (**shared dashboard host**)
  - log file: `/var/log/ucxsync/ucxsync-a.log`
- **Instance B**
  - nodes: `WU08`-`WU13` and `CU`
  - interface: `end1`
  - mount root: `/ucmount-b`
  - web UI: `:8081` (direct fallback / instance-local UI)
  - log file: `/var/log/ucxsync/ucxsync-b.log`

Both instances may write to the same destination, for example `/ucdata`, but they must use different:

- `nodes`
- `network.mount_root`
- `web.port`
- `logging.file`

### Principle of operation

```text
WU01..WU07 ----> end0 ----> host routes ----> UCXSync A ----> /ucmount-a ---->
                                                                     copy ----> /ucdata

WU08..WU13 ----> end1 ----> host routes ----> UCXSync B ----> /ucmount-b ---->
CU -----------/                                                      copy ----> /ucdata
```

This model helps because:

1. each process mounts and scans only half of the UCX estate;
2. traffic is pinned to the intended NIC by host routes;
3. each process has its own worker pool and restart lifecycle;
4. mount trees are isolated, so the two instances do not fight over `/ucmount`.

### Shared dashboard

The dual deployment can now expose **one common dashboard**.

Recommended layout:

- `ucxsync@a` on `:8080` serves the **shared dashboard**
- `ucxsync@b` on `:8081` remains available as a direct instance UI and fallback endpoint

The shared dashboard on instance A:

- collects status from both instances;
- shows host performance once (CPU, RAM, disk, network);
- merges active tasks into one table;
- allows starting, stopping, remounting shares, and restarting services for both instances from one page.

This is enabled in `config.instance-a.yaml` through `web.dashboard.instances`.

### Recommended filesystem and service layout

```text
/opt/ucxsync/
├── ucxsync
└── web/

/etc/ucxsync/
├── a.yaml
├── b.yaml
└── config.yaml        # optional legacy single-instance config

/etc/systemd/system/
├── ucxsync@.service
├── ucxsync-dualnic-routing.service
├── ucxsync.service.d/
│   └── 10-dualnic-routing.conf
└── ucxsync@.service.d/
    └── 10-dualnic-routing.conf

/ucmount-a/
/ucmount-b/
/ucdata/
```

### Step-by-step dual-instance deployment

1. Install the dual version directly:

```bash
sudo ./install.sh --dual --install-dualnic-routing
```

If you already ran the default installation, you can re-run the installer in dual mode safely; existing configs are not overwritten.

If you prefer to install routing later, omit `--install-dualnic-routing` and run the helper manually.

2. Create separate mount roots:

```bash
sudo mkdir -p /ucmount-a /ucmount-b /var/log/ucxsync
```

3. Install the example instance configs:

```bash
sudo cp config.instance-a.yaml /etc/ucxsync/a.yaml
sudo cp config.instance-b.yaml /etc/ucxsync/b.yaml
sudo chmod 600 /etc/ucxsync/a.yaml /etc/ucxsync/b.yaml
```

4. Edit them and verify:

```bash
sudo nano /etc/ucxsync/a.yaml
sudo nano /etc/ucxsync/b.yaml
```

Check that:

- `a.yaml` uses `/ucmount-a` and port `8080`
- `b.yaml` uses `/ucmount-b` and port `8081`
- optional `network.mount_options` are consistent between instances if you tune CIFS
- `a.yaml` contains `web.dashboard.instances` for both `http://127.0.0.1:8080` and `http://127.0.0.1:8081`
- node lists do not overlap
- log files are different

5. Install the template unit:

```bash
sudo cp ucxsync@.service /etc/systemd/system/
sudo systemctl daemon-reload
```

6. Install dual-NIC routing before starting the instances (skip if you already used `--install-dualnic-routing`):

```bash
sudo END0_IFACE=end0 \
     END1_IFACE=end1 \
     END0_SRC_IP=192.168.200.101 \
     END1_SRC_IP=192.168.200.102 \
     END0_HOSTS="1 2 3 4 5 6 7" \
     END1_HOSTS="8 9 10 11 12 13 201" \
  /opt/ucxsync/setup-dualnic-routing.sh --install
```

The helper installs host routes and creates systemd ordering for both `ucxsync.service` and `ucxsync@.service`.

### SMB/CIFS throughput tuning

For legacy SMB1 sources, keep the default compatibility first and add tuning options gradually via `network.mount_options` in your config:

```yaml
network:
  mount_root: /ucmount-a
  mount_options:
    - nounix
    - noserverino
    - actimeo=1
    - rsize=65536
    - wsize=65536
```

Notes:

- `nounix` and `noserverino` are often helpful for old Windows/XP-style servers.
- `actimeo=1` reduces metadata round-trips without making cache staleness too aggressive.
- `rsize` / `wsize` should be tested on your specific UCX nodes; some SMB1 servers ignore them, others benefit from `65536`.
- Do **not** enable risky caching options blindly on capture sources that may still be changing.

### TCP buffer and IRQ affinity tuning

The routing helper can also persist host-side TCP buffer tuning and pin interface IRQs to specific CPU cores.

Example: keep `end0` IRQs on CPU 0 and `end1` IRQs on CPU 1:

```bash
sudo PIN_IRQS=1 \
     END0_IRQ_CORES=0 \
     END1_IRQ_CORES=1 \
     NET_CORE_RMEM_MAX=134217728 \
     NET_CORE_WMEM_MAX=134217728 \
     TCP_RMEM="4096 262144 67108864" \
     TCP_WMEM="4096 262144 67108864" \
     NETDEV_MAX_BACKLOG=250000 \
     END0_IFACE=end0 \
     END1_IFACE=end1 \
     END0_SRC_IP=192.168.200.101 \
     END1_SRC_IP=192.168.200.102 \
     END0_HOSTS="1 2 3 4 5 6 7" \
     END1_HOSTS="8 9 10 11 12 13 201" \
  /opt/ucxsync/setup-dualnic-routing.sh --install
```

The helper looks for IRQ labels containing the interface name in `/proc/interrupts` and writes the chosen CPU list into `/proc/irq/*/smp_affinity_list`.

If `irqbalance` is running, it may later override manual affinity, so either disable it or configure it not to fight your pinning.

7. Enable both instances:

```bash
sudo systemctl enable --now ucxsync@a
sudo systemctl enable --now ucxsync@b
```

8. Verify everything:

```bash
sudo systemctl status ucxsync-dualnic-routing.service
sudo systemctl status ucxsync@a
sudo systemctl status ucxsync@b

ip route get 192.168.200.1
ip route get 192.168.200.8
ip route get 192.168.200.201

mount | grep -E '/ucmount-a|/ucmount-b'
```

Expected result:

- nodes `1..7` route through `end0`
- nodes `8..13` and `201` route through `end1`
- instance A mounts only under `/ucmount-a`
- instance B mounts only under `/ucmount-b`

## Running the Application

### Start Service

```bash
# Enable service to start on boot
sudo systemctl enable ucxsync

# Start service now
sudo systemctl start ucxsync

# Check status
sudo systemctl status ucxsync
```

If you want interface pinning in single-instance mode, you can install the routing helper either during install or later:

```bash
sudo END0_IFACE=end0 \
  END1_IFACE=enx00e04c141b68 \
  END0_SRC_IP=192.168.200.101 \
  END1_SRC_IP=192.168.200.103 \
  END0_HOSTS="1 2 3 4 5 6 7" \
  END1_HOSTS="8 9 10 11 12 13 201" \
  /opt/ucxsync/setup-dualnic-routing.sh --install
```

### Start two-instance deployment

```bash
sudo systemctl enable --now ucxsync-dualnic-routing.service
sudo systemctl enable --now ucxsync@a
sudo systemctl enable --now ucxsync@b

sudo systemctl status ucxsync@a
sudo systemctl status ucxsync@b
```

Access the shared dashboard at:

```text
http://<server-ip>:8080
```

Direct per-instance fallback UI remains available at:

```text
http://<server-ip>:8081
```

### View Logs

```bash
# Follow logs in real-time
sudo journalctl -u ucxsync -f

# View recent logs
sudo journalctl -u ucxsync -n 100

# Application logs
sudo tail -f /var/log/ucxsync/ucxsync.log
```

For split deployment:

```bash
sudo journalctl -u ucxsync@a -f
sudo journalctl -u ucxsync@b -f

sudo tail -f /var/log/ucxsync/ucxsync-a.log
sudo tail -f /var/log/ucxsync/ucxsync-b.log
```

### Manual Operation

```bash
# Run once without service
sudo /opt/ucxsync/ucxsync

# Run with custom config
sudo /opt/ucxsync/ucxsync --config /path/to/config.yaml

# Debug mode
sudo /opt/ucxsync/ucxsync --debug

# Mount shares only
sudo /opt/ucxsync/ucxsync mount

# Unmount shares
sudo /opt/ucxsync/ucxsync unmount
```

Run a specific instance manually:

```bash
sudo /opt/ucxsync/ucxsync --config /etc/ucxsync/a.yaml
sudo /opt/ucxsync/ucxsync --config /etc/ucxsync/b.yaml
```

## Accessing the Web Interface

Once running, access the web interface at:

```
http://localhost:8080
```

Or from another machine (if host is set to 0.0.0.0):

```
http://<server-ip>:8080
```

## Troubleshooting

### Service won't start

```bash
# Check service status
sudo systemctl status ucxsync

# Check logs
sudo journalctl -u ucxsync -xe

# Verify configuration
sudo /opt/ucxsync/ucxsync --config /etc/ucxsync/config.yaml --debug
```

For template instances:

```bash
sudo systemctl status ucxsync@a
sudo systemctl status ucxsync@b
sudo journalctl -u ucxsync@a -n 100
sudo journalctl -u ucxsync@b -n 100
sudo systemctl status ucxsync-dualnic-routing.service
```

If `/ucdata` was mounted after a service had already started on an older installation, one instance may report that `/ucdata` is unavailable while another works. In that case, update the unit files, reload systemd, and restart the affected instances:

```bash
sudo cp ucxsync.service /etc/systemd/system/
sudo cp ucxsync@.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl restart ucxsync ucxsync@a ucxsync@b
```

### Cannot mount shares

```bash
# Check if cifs-utils is installed
dpkg -l | grep cifs-utils

# Try manual mount
sudo mount -t cifs //WU01/E /mnt/test -o username=Administrator,password=ultracam,vers=3.0

# Check network connectivity
ping WU01
```

For split deployment, also verify both mount roots:

```bash
ls -la /ucmount-a
ls -la /ucmount-b
```

### Permission denied errors

```bash
# Ensure running as root
sudo /opt/ucxsync/ucxsync

# Check file permissions
ls -la /opt/ucxsync
ls -la /etc/ucxsync
ls -la /ucmount
```

### High CPU usage

Reduce parallelism in configuration:

```yaml
sync:
  max_parallelism: 4  # Lower value
```

### Disk space issues

Check available space:

```bash
df -h /destination/path
```

Adjust minimum space requirements in config:

```yaml
sync:
  min_free_disk_space: 104857600  # 100 MB
```

## Updating

### Update via Git

```bash
cd UCXSync
git pull
sudo systemctl stop ucxsync
go build -o ucxsync ./cmd/ucxsync
sudo cp ucxsync /opt/ucxsync/
sudo systemctl start ucxsync
```

For two instances:

```bash
cd UCXSync
git pull
go build -o ucxsync ./cmd/ucxsync
sudo systemctl stop ucxsync@a ucxsync@b
sudo cp ucxsync /opt/ucxsync/
sudo cp -r web /opt/ucxsync/
sudo systemctl start ucxsync@a ucxsync@b
```

### Update Configuration

```bash
# Backup current config
sudo cp /etc/ucxsync/config.yaml /etc/ucxsync/config.yaml.backup

# Update with new options from example
sudo nano /etc/ucxsync/config.yaml

# Restart service
sudo systemctl restart ucxsync
```

## Uninstallation

```bash
cd UCXSync

# Make uninstall script executable
chmod +x uninstall.sh

# Remove only the main single-instance deployment (default)
sudo ./uninstall.sh

# Remove only the dual deployment
sudo ./uninstall.sh --dual

# Remove everything
sudo ./uninstall.sh --all
```

Available modes:

- `--single` — remove only the main single-instance deployment
- `--dual` — remove only the dual deployment (`ucxsync@a`, `ucxsync@b`, dual-NIC helper)
- `--all` — remove all deployments plus shared files in `/opt/ucxsync`

If you are **testing the dual version on a host that already has the single version**, you do **not** need a full uninstall first.

Usually it is enough to:

```bash
sudo systemctl stop ucxsync
sudo systemctl disable ucxsync
sudo ./install.sh --dual
```

Use a full uninstall only if you want a completely clean machine or want to remove old configs and mount roots.

Or manually:

```bash
# Stop and disable service
sudo systemctl stop ucxsync
sudo systemctl disable ucxsync

# Unmount shares
sudo umount /ucmount/*/[EF]

# Remove files
sudo rm -rf /opt/ucxsync
sudo rm -rf /var/log/ucxsync
sudo rm /etc/systemd/system/ucxsync.service
sudo systemctl daemon-reload

# Optionally remove config and mounts
sudo rm -rf /etc/ucxsync
sudo rm -rf /ucmount
```

For two instances:

```bash
sudo systemctl stop ucxsync@a ucxsync@b
sudo systemctl disable ucxsync@a ucxsync@b
sudo systemctl disable --now ucxsync-dualnic-routing.service
sudo rm -f /etc/systemd/system/ucxsync@.service
sudo rm -rf /etc/systemd/system/ucxsync@.service.d
sudo rm -rf /ucmount-a /ucmount-b
sudo systemctl daemon-reload
```

## Security Considerations

1. **Credentials file**: Protected at 0600 permissions
2. **Network**: Restrict web interface to localhost or use firewall
3. **SMB**: Current mount code uses `vers=1.0` for compatibility with legacy UCX nodes
4. **Logs**: Contains no sensitive information

### Firewall Configuration

If exposing web interface:

```bash
# Allow port 8080
sudo ufw allow 8080/tcp

# Or restrict to specific IP
sudo ufw allow from 192.168.1.0/24 to any port 8080
```

## Performance Tuning

### For high-speed networks (10 Gbps):

```yaml
sync:
  max_parallelism: 16

monitoring:
  max_disk_throughput_mbps: 1000.0
  network_speed_bps: 10000000000  # 10 Gbps
```

### For resource-constrained systems:

```yaml
sync:
  max_parallelism: 4

monitoring:
  performance_update_interval: 5s
  ui_update_interval: 5s
```

### For dual-instance SMB1 deployments

Use this as a starting point for each instance:

```yaml
sync:
  max_parallelism: 4

network:
  mount_root: /ucmount-a   # second instance uses /ucmount-b

web:
  port: 8080               # second instance uses 8081

logging:
  file: /var/log/ucxsync/ucxsync-a.log
```

Tune each instance separately instead of trying to force one process to consume both NICs efficiently over SMB1.

## Support

- Issues: https://github.com/zangezia/UCXSync/issues
- Documentation: https://github.com/zangezia/UCXSync/wiki
