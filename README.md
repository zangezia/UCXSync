# UCXSync

High-performance file synchronization tool for UCX projects with web-based monitoring (Linux-only).

## Overview

UCXSync is a Linux Go application that synchronizes files from multiple network nodes to a local destination with real-time performance monitoring through a web interface.

### Key Features

- **Multi-node synchronization**: Parallel copying from 14 nodes (WU01-WU13 worker nodes + CU control unit)
- **Multiple sources**: Each node has 2 network shares (E$, F$) = 28 sources total
- **Incremental sync**: Only copies new or modified files
- **Configurable parallelism**: Adjustable concurrent file operations
- **Capture tracking**: Automatic detection and tracking of completed captures
  - Normal captures: 13 RAW + 1 XML = 14 files
  - Test captures: 13 RAW files (XML optional)
- **File type recognition**: Distinguishes verified (Lvl00) and unverified (Lvl0X) captures
- **Test capture support**: Separate tracking for test captures (marked with "T")
- **Metadata handling**: EAD XML files from CU node (may be missing for test captures)
- **Web interface**: Real-time monitoring via browser
- **Performance metrics**: Live CPU, disk, network, and memory monitoring
- **CIFS/SMB mounting**: Automatic mounting of network shares

## Architecture

```
UCXSync/
├── cmd/
│   └── ucxsync/           # Main application entry point
├── internal/
│   ├── config/            # Configuration management
│   ├── sync/              # File synchronization engine
│   ├── monitor/           # Performance monitoring
│   ├── network/           # Network credentials & SMB mounting
│   ├── storage/           # Settings persistence
│   └── web/               # Web server & WebSocket handlers
├── pkg/
│   └── models/            # Shared data models
├── web/
│   ├── static/            # CSS, JS, images
│   └── templates/         # HTML templates
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## System Requirements

- **OS**: Ubuntu 20.04+ (tested on Ubuntu Server 24.04, Orange Pi compatible)
- **Architecture**: RISC-V 64-bit / ARM64 / AMD64
- **Hardware**: Orange Pi RV2 (RISC-V) or compatible SBC / Laptop / Server
- **Go**: 1.21 or higher (for building from source)
- **CIFS**: `cifs-utils` package for SMB mounting
- **Permissions**: sudo/root for mounting network shares
- **Storage**: 
  - Internal: Minimum 50 MB for application
  - **External USB-SSD**: 500GB - 2TB for data storage (recommended)
- **Network**: Access to UCX worker nodes (WU01-WU13, CU)

## 📚 Documentation

### Quick Start Guides
- **[🚀 Quick Start (Russian)](README.ru.md)** - Быстрый старт на русском
- **[📋 Cheat Sheet](CHEATSHEET.md)** - Quick reference commands
- **[🧪 Testing Guide](TESTING-ON-LAPTOP.md)** - Laptop testing instructions

### Platform-Specific Guides
- **[🐧 Linux AMD64](LINUX.md)** - Standard server installation
- **[🍊 Orange Pi RV2](ORANGEPI.md)** - RISC-V specific guide
- **[⚙️ RISC-V Details](RISCV.md)** - Architecture information

### Architecture & Setup
- **[📁 Storage Architecture](STORAGE-ARCHITECTURE.md)** - Understanding `/mnt/ucx` vs `/mnt/storage/ucx`
- **[💾 USB-SSD Guide](USB-SSD-GUIDE.md)** - External storage setup and troubleshooting
- **[🔧 Build Instructions](BUILD.md)** - Multi-architecture build guide
- **[⚡ Parallelism](PARALLELISM.md)** - Understanding concurrency control

### Maintenance
- **[🧹 Uninstall Guide](UNINSTALL-GUIDE.md)** - Complete removal instructions
- **[📊 Testing](TEST.md)** - Comprehensive testing procedures

## Installation

### Linux AMD64/x86_64 Servers

For standard Linux servers (Ubuntu, Debian, CentOS, RHEL), see:
**[📘 Linux Installation Guide](LINUX.md)**

Quick install:
```bash
sudo chmod +x install.sh
sudo ./install.sh
```

### Orange Pi RV2 (RISC-V)

For Orange Pi RV2 with Ubuntu Server 24.04, see detailed guide:
**[📘 Orange Pi Installation Guide](ORANGEPI.md)**

Quick install:
```bash
sudo chmod +x install-orangepi.sh
sudo ./install-orangepi.sh
```

### From Source

```bash
# Clone the repository
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync

# Build
go build -o ucxsync ./cmd/ucxsync

# Run
./ucxsync
```

### Using Go Install

```bash
go install github.com/zangezia/UCXSync/cmd/ucxsync@latest
```

## Configuration

Create `config.yaml` in the application directory:

```yaml
# Network configuration
nodes:
  - WU01
  - WU02
  - WU03
  - WU04
  - WU05
  - WU06
  - WU07
  - WU08
  - WU09
  - WU10
  - WU11
  - WU12
  - WU13
  - CU

shares:
  - E$
  - F$

# Authentication (Windows)
credentials:
  username: Administrator
  password: ultracam

# Synchronization settings
sync:
  max_parallelism: 8
  service_loop_interval: 10s
  min_free_disk_space: 52428800  # 50 MB
  disk_space_safety_margin: 104857600  # 100 MB

# Web server
web:
  host: localhost
  port: 8080

# Monitoring
monitoring:
  performance_update_interval: 1s
  ui_update_interval: 2s
  cpu_smoothing_samples: 3
  max_disk_throughput_mbps: 200.0
  network_speed_bps: 1000000000  # 1 Gbps

# Logging
logging:
  level: info
  file: logs/ucxsync.log
  max_size: 100  # MB
  max_backups: 5
  max_age: 30  # days
```

## Usage

### Start the application

```bash
# Start with default configuration
./ucxsync

# Start with custom config file
./ucxsync --config /path/to/config.yaml

# Start with custom web port
./ucxsync --port 9090
```

### Access the web interface

Open your browser and navigate to:
```
http://localhost:8080
```

### Command-line options

```bash
# Show help
./ucxsync --help

# Show version
./ucxsync --version

# Start with specific project and destination
./ucxsync --project MyProject --dest /path/to/destination

# Enable debug logging
./ucxsync --debug
```

## Web Interface

The web interface provides:

1. **Control Panel**
   - Project selection (auto-discovered from network)
   - Destination path selection
   - Parallelism configuration
   - Start/Stop controls

2. **Real-time Monitoring**
   - Live log stream
   - Completed captures counter
   - Test captures counter
   - Last capture number

3. **Performance Metrics**
   - CPU usage (with smoothing)
   - Disk throughput
   - Network throughput
   - Free disk space

4. **Activity Table**
   - Per-node synchronization status
   - Files downloaded per node
   - Progress percentage
   - Last update time

## API Endpoints

### REST API

- `GET /api/projects` - List available projects
- `GET /api/status` - Get current sync status
- `POST /api/sync/start` - Start synchronization
- `POST /api/sync/stop` - Stop synchronization

### WebSocket

- `ws://localhost:8080/ws` - Real-time updates
  - `log` - Log messages
  - `status` - Status updates
  - `progress` - Progress updates
  - `metrics` - Performance metrics

## Capture File Format

UCXSync automatically detects and tracks capture files from 14 nodes:

### File Naming Convention

**RAW files** (13 files from WU01-WU13 nodes):
```
Lvl00-00001-Arh2k_mezen_200725-06-00-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.raw
Lvl0X-00002-T-Arh2k_mezen_200725-06-00-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.raw
```

**XML metadata file** (1 file from CU node):
```
EAD-00001-Arh2k_mezen_200725-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.xml
```

### Format Structure

**RAW file format:**
```
Lvl00-00001-T-Arh2k_mezen_200725-06-00-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.raw
│     │     │ │                  │     └──────────────────────── Session GUID
│     │     │ │                  └────────────────────────────── Sensor Code (XX-YY)
│     │     │ └───────────────────────────────────────────────── Project Name
│     │     └─────────────────────────────────────────────────── Test Marker (optional)
│     └───────────────────────────────────────────────────────── Capture Number (5 digits)
└─────────────────────────────────────────────────────────────── Data Type
```

**XML metadata format:**
```
EAD-00001-Arh2k_mezen_200725-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.xml
│   │     │                  └──────────────────────────────────── Session GUID
│   │     └─────────────────────────────────────────────────────── Project Name
│   └───────────────────────────────────────────────────────────── Capture Number
└───────────────────────────────────────────────────────────────── Metadata prefix (EAD)
```

### Field Descriptions

**RAW files:**
- **Data Type**: 
  - `Lvl00` - Verified capture
  - `Lvl0X` - Unverified capture (X can be any digit)
  
- **Capture Number**: 5-digit sequential number (00001, 00002, etc.)

- **Test Marker**: Optional `T-` after capture number indicates test capture

- **Project Name**: Unique project identifier (e.g., `Arh2k_mezen_200725`)

- **Sensor Code**: Two-part code in format `XX-YY` (e.g., `00-00`, `06-00`, `01-02`)

- **Session GUID**: Unique identifier for the capture session

**XML files:**
- **EAD Prefix**: Metadata file identifier
- **Capture Number**: Matches corresponding RAW files
- **Project Name**: Matches corresponding RAW files
- **Session GUID**: Matches corresponding RAW files

### Capture Completion

A capture is considered **complete** based on type:

**Normal (non-test) captures:** 14 files required
- **13 RAW files** - One from each worker node (WU01-WU13)
- **1 XML file** - Metadata from control unit (CU)

**Test captures:** 13 files required
- **13 RAW files** - One from each worker node (WU01-WU13)
- **XML file may be missing** - Test captures might not have metadata

The system tracks each capture by number and marks it complete only when all required components are present.

## Development

### Project Structure

- **cmd/ucxsync/** - Main application
- **internal/config/** - Configuration loading and validation
- **internal/sync/** - Core synchronization logic
- **internal/monitor/** - System performance monitoring
- **internal/network/** - Network authentication and mounting
- **internal/storage/** - Settings persistence
- **internal/web/** - HTTP server and WebSocket handlers
- **pkg/models/** - Shared data structures
- **web/** - Frontend assets

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Run with live reload (requires air)
make dev
```

### Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/sync/...
```

## Linux Setup

### Installing CIFS utilities

```bash
sudo apt-get update
sudo apt-get install -y cifs-utils
```

### Network share mounting

UCXSync automatically mounts network shares using CIFS. On first run:

```bash
# Run with sudo for initial setup
sudo ./ucxsync

# Creates mount points at /mnt/ucx/{node}/{share}
# Stores credentials in /etc/ucxsync/credentials
```

### Running as systemd service

```bash
# Copy service file
sudo cp ucxsync.service /etc/systemd/system/

# Enable and start
sudo systemctl enable ucxsync
sudo systemctl start ucxsync

# Check status
sudo systemctl status ucxsync
```

### Mount points structure

```
/mnt/ucx/
├── WU01/
│   ├── E$/
│   └── F$/
├── WU02/
│   ├── E$/
│   └── F$/
...
└── CU/
    ├── E$/
    └── F$/
```

## Performance

- **Throughput**: Up to 200 MB/s disk I/O
- **Network**: Supports 1 Gbps links
- **Parallelism**: Default 8 concurrent operations (configurable)
- **Memory**: ~50-100 MB typical usage
- **CPU**: Low overhead, <5% on modern systems

## Troubleshooting

### Cannot connect to network shares

**Windows**: Ensure SMB1 is enabled
```powershell
Enable-WindowsOptionalFeature -Online -FeatureName SMB1Protocol
```

**Linux**: Install CIFS utilities
```bash
sudo apt-get install cifs-utils
```

### Insufficient disk space

Ensure at least 150 MB free space (50 MB minimum + 100 MB safety margin)

### High CPU usage

Reduce parallelism in configuration:
```yaml
sync:
  max_parallelism: 4
```

## License

MIT License - see LICENSE file for details

## Contributing

Contributions welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## 🗑️ Uninstallation

To remove UCXSync from your system:

```bash
cd UCXSync
chmod +x uninstall.sh
sudo ./uninstall.sh
```

Or manually:

```bash
# Stop and disable service
sudo systemctl stop ucxsync
sudo systemctl disable ucxsync

# Remove files
sudo rm -f /usr/local/bin/ucxsync
sudo rm -rf /etc/ucxsync
sudo rm -f /etc/systemd/system/ucxsync.service
sudo systemctl daemon-reload

# Optionally remove data
sudo rm -rf /mnt/storage/ucx
sudo rm -rf /mnt/ucx
```

## Support

- Issues: https://github.com/zangezia/UCXSync/issues
- Documentation: https://github.com/zangezia/UCXSync/wiki

## Roadmap

- [ ] Docker container support
- [ ] Systemd service integration
- [ ] Email notifications on capture completion
- [ ] Grafana/Prometheus metrics export
- [ ] REST API for external integrations
- [ ] Multiple destination support
- [ ] Bandwidth throttling
- [ ] Resume interrupted transfers
