# Quick Start Guide - UCXSync

## Installation

```bash
# 1. Clone repository
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync

# 2. Install (automated)
chmod +x install.sh
sudo ./install.sh
```

## Configuration

Edit configuration file:

```bash
sudo nano /etc/ucxsync/config.yaml
```

Minimal required changes:

```yaml
credentials:
  username: YourUsername
  password: YourPassword

sync:
  max_parallelism: 8  # Adjust based on CPU cores
```

## First Run

### 1. Check Requirements

```bash
sudo ucxsync check
```

Expected output:
```
✓ Configuration loaded
✓ Network requirements met
✓ CIFS utilities installed
✓ Running with required privileges
```

### 2. Mount Network Shares

```bash
sudo ucxsync mount
```

This mounts all configured shares to `/mnt/ucx/`:
- `/mnt/ucx/WU01/E/`
- `/mnt/ucx/WU01/F/`
- etc.

### 3. Start Server

```bash
sudo ucxsync
```

Or as a service:

```bash
sudo systemctl start ucxsync
sudo systemctl status ucxsync
```

### 4. Access Web Interface

Open browser: `http://localhost:8080`

## Basic Usage

### Web Interface

1. **Click "🔄 Обновить"** to scan for available projects
2. **Select project** from dropdown
3. **Enter destination path** (e.g., `/data/ucx-projects`)
4. **Set parallelism** (default: 8 threads)
5. **Click "▶️ Запустить"** to start synchronization

### Monitor Progress

The interface shows real-time:
- **Completed captures** counter
- **Performance metrics** (CPU, Memory, Disk, Network)
- **Active tasks** per node/share
- **Live log** of events

### Stop Synchronization

Click "⏹️ Остановить" button or:

```bash
sudo systemctl stop ucxsync
```

## Command Line Usage

### Mount/Unmount Manually

```bash
# Mount all shares
sudo ucxsync mount

# Unmount all shares
sudo ucxsync unmount
```

### Run with Custom Config

```bash
sudo ucxsync --config /path/to/config.yaml
```

### Debug Mode

```bash
sudo ucxsync --debug
```

### Specify Project at Startup

```bash
sudo ucxsync --project MyProject --dest /data/output
```

## Viewing Logs

### Application Logs

```bash
# Real-time
sudo journalctl -u ucxsync -f

# Last 100 lines
sudo journalctl -u ucxsync -n 100

# Application log file
sudo tail -f /var/log/ucxsync/ucxsync.log
```

### Web Interface Logs

Check the **Журнал событий** (Event Log) panel in the web interface for real-time updates.

## Common Tasks

### Check Mounted Shares

```bash
mount | grep /mnt/ucx
```

Or:

```bash
ls -la /mnt/ucx/WU01/E/
```

### Change Parallelism

Edit config:

```bash
sudo nano /etc/ucxsync/config.yaml
```

Change:
```yaml
sync:
  max_parallelism: 16  # More threads
```

Restart:
```bash
sudo systemctl restart ucxsync
```

### Check Disk Space

```bash
df -h /destination/path
```

### Monitor System Resources

```bash
# CPU and memory
htop

# Disk I/O
iotop

# Network
iftop
```

## Troubleshooting

### Service won't start

```bash
# Check status
sudo systemctl status ucxsync

# Check logs
sudo journalctl -u ucxsync -xe

# Test manually
sudo /opt/ucxsync/ucxsync --debug
```

### Cannot mount shares

```bash
# Install cifs-utils
sudo apt-get install cifs-utils

# Test manual mount
sudo mount -t cifs //WU01/E /mnt/test \
  -o username=Administrator,password=ultracam,vers=3.0

# Check connectivity
ping WU01
```

### Web interface not accessible

```bash
# Check if service is running
sudo systemctl status ucxsync

# Check port
sudo netstat -tlnp | grep 8080

# Check firewall
sudo ufw status
sudo ufw allow 8080/tcp
```

### High CPU usage

Reduce parallelism:
```yaml
sync:
  max_parallelism: 4
```

### Out of disk space

Check space:
```bash
df -h
```

Increase minimum in config:
```yaml
sync:
  min_free_disk_space: 104857600  # 100 MB
```

## Performance Tips

### For High-Speed Networks (10 Gbps)

```yaml
sync:
  max_parallelism: 16
  
monitoring:
  max_disk_throughput_mbps: 1000.0
  network_speed_bps: 10000000000
```

### For Low-End Hardware

```yaml
sync:
  max_parallelism: 4
  
monitoring:
  performance_update_interval: 5s
  ui_update_interval: 5s
```

## Systemd Service Management

```bash
# Enable on boot
sudo systemctl enable ucxsync

# Start service
sudo systemctl start ucxsync

# Stop service
sudo systemctl stop ucxsync

# Restart service
sudo systemctl restart ucxsync

# Check status
sudo systemctl status ucxsync

# Disable on boot
sudo systemctl disable ucxsync
```

## Backup and Restore

### Backup Configuration

```bash
sudo cp /etc/ucxsync/config.yaml ~/ucxsync-config-backup.yaml
```

### Restore Configuration

```bash
sudo cp ~/ucxsync-config-backup.yaml /etc/ucxsync/config.yaml
sudo systemctl restart ucxsync
```

## Updating

```bash
cd UCXSync
git pull
sudo systemctl stop ucxsync
make build
sudo cp ucxsync /opt/ucxsync/
sudo systemctl start ucxsync
```

## Getting Help

- Check logs: `sudo journalctl -u ucxsync -f`
- Run diagnostics: `sudo ucxsync check`
- View README: `cat /opt/ucxsync/README.md`
- Issues: https://github.com/zangezia/UCXSync/issues

## Next Steps

- Configure automatic startup: `sudo systemctl enable ucxsync`
- Set up log rotation
- Configure firewall rules
- Set up monitoring (Grafana/Prometheus)
- Create backup scripts
