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

The script will:
- Check and install prerequisites
- Build the application
- Create necessary directories
- Install binary to `/opt/ucxsync/`
- Install systemd service
- Set up configuration

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
sudo mkdir -p /mnt/ucx

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
/mnt/ucx/
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

### View Logs

```bash
# Follow logs in real-time
sudo journalctl -u ucxsync -f

# View recent logs
sudo journalctl -u ucxsync -n 100

# Application logs
sudo tail -f /var/log/ucxsync/ucxsync.log
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

### Cannot mount shares

```bash
# Check if cifs-utils is installed
dpkg -l | grep cifs-utils

# Try manual mount
sudo mount -t cifs //WU01/E /mnt/test -o username=Administrator,password=ultracam,vers=3.0

# Check network connectivity
ping WU01
```

### Permission denied errors

```bash
# Ensure running as root
sudo /opt/ucxsync/ucxsync

# Check file permissions
ls -la /opt/ucxsync
ls -la /etc/ucxsync
ls -la /mnt/ucx
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

# Run uninstallation
sudo ./uninstall.sh
```

Or manually:

```bash
# Stop and disable service
sudo systemctl stop ucxsync
sudo systemctl disable ucxsync

# Unmount shares
sudo umount /mnt/ucx/*/[EF]

# Remove files
sudo rm -rf /opt/ucxsync
sudo rm -rf /var/log/ucxsync
sudo rm /etc/systemd/system/ucxsync.service
sudo systemctl daemon-reload

# Optionally remove config and mounts
sudo rm -rf /etc/ucxsync
sudo rm -rf /mnt/ucx
```

## Security Considerations

1. **Credentials file**: Protected at 0600 permissions
2. **Network**: Restrict web interface to localhost or use firewall
3. **SMB**: Uses SMB3.0 by default (more secure than SMB1)
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
  buffer_size: 65536  # 64 KB

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

## Support

- Issues: https://github.com/zangezia/UCXSync/issues
- Documentation: https://github.com/zangezia/UCXSync/wiki
