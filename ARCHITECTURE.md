# UCXSync Project Structure

Complete overview of the project organization and file purposes.

## Directory Structure

```
UCXSync/
├── cmd/
│   └── ucxsync/              # Main application entry point
│       ├── main.go           # Application initialization, CLI setup
│       └── commands.go       # CLI subcommands (mount, unmount, check)
│
├── internal/                 # Private application code
│   ├── config/              
│   │   └── config.go         # Configuration loading (Viper), validation
│   │
│   ├── sync/
│   │   └── sync.go           # Core file synchronization engine
│   │                         # - Multi-node parallel copying
│   │                         # - Capture tracking (13 nodes)
│   │                         # - Incremental sync logic
│   │
│   ├── monitor/
│   │   └── monitor.go        # System performance monitoring
│   │                         # - CPU usage (with smoothing)
│   │                         # - Memory stats
│   │                         # - Disk I/O
│   │                         # - Network throughput
│   │
│   ├── network/
│   │   └── network.go        # Network share management
│   │                         # - CIFS mounting
│   │                         # - Credentials handling
│   │                         # - Mount/unmount operations
│   │
│   ├── storage/              # (Reserved for future use)
│   │   └── settings.go       # User settings persistence
│   │
│   └── web/
│       └── server.go         # HTTP server and WebSocket handler
│                             # - REST API endpoints
│                             # - WebSocket real-time updates
│                             # - Client connection management
│
├── pkg/
│   └── models/
│       └── models.go         # Shared data structures
│                             # - SyncTask, SyncStatus
│                             # - PerformanceMetrics
│                             # - CaptureInfo, LogMessage
│                             # - WebSocket message types
│
├── web/                      # Frontend assets
│   ├── templates/
│   │   └── index.html        # Main web interface HTML
│   │
│   └── static/
│       ├── css/
│       │   └── style.css     # Dark theme styling
│       │
│       └── js/
│           └── app.js        # Frontend application logic
│                             # - WebSocket client
│                             # - UI updates
│                             # - API calls
│
├── config.example.yaml       # Example configuration file
├── ucxsync.service          # Systemd service definition
├── install.sh               # Automated installation script
├── uninstall.sh             # Removal script
├── go.mod                   # Go module definition
├── go.sum                   # Dependency checksums
├── Makefile                 # Build automation
├── README.md                # Main documentation
├── INSTALL.md               # Installation guide
├── QUICKSTART.md            # Quick start guide
└── .gitignore              # Git ignore rules
```

## Key Files Description

### Application Core

**cmd/ucxsync/main.go**
- Entry point for the application
- CLI argument parsing with Cobra
- Context and signal handling
- Logging setup (zerolog)
- Web server initialization

**cmd/ucxsync/commands.go**
- `mount` - Mount all network shares
- `unmount` - Unmount all shares
- `check` - Verify system requirements

### Business Logic

**internal/sync/sync.go**
- Core synchronization engine
- Functions:
  - `New()` - Create sync service
  - `Start()` - Begin synchronization loop
  - `Stop()` - Graceful shutdown
  - `GetStatus()` - Current sync state
  - `FindProjects()` - Scan network for projects
  - `syncLoop()` - Main coordination loop
  - `syncDirectory()` - Directory scanning and copying
  - `copyFile()` - File copy with retries
  - `trackCaptureCompletion()` - Capture completion detection

**internal/monitor/monitor.go**
- System performance monitoring using gopsutil
- Functions:
  - `New()` - Create monitor service
  - `Start()` - Begin monitoring loop
  - `GetMetrics()` - Get current metrics snapshot
  - `collectMetrics()` - Gather CPU/Memory/Disk/Network stats
  - `SetTargetDisk()` - Set disk path for monitoring

**internal/network/network.go**
- Linux CIFS/SMB mounting
- Functions:
  - `New()` - Create network service
  - `MountAll()` - Mount all configured shares
  - `UnmountAll()` - Unmount all shares
  - `GetMountPoint()` - Get local mount path
  - `CheckRequirements()` - Verify cifs-utils installed
  - `mountShare()` - Execute mount command
  - `isMounted()` - Check if path is mounted

**internal/config/config.go**
- Configuration management with Viper
- YAML config file parsing
- Environment variable support
- Validation and defaults
- Settings persistence

**internal/web/server.go**
- HTTP server and WebSocket handler
- REST API:
  - `GET /api/projects` - List available projects
  - `GET /api/status` - Current sync status
  - `POST /api/sync/start` - Start synchronization
  - `POST /api/sync/stop` - Stop synchronization
  - `GET /ws` - WebSocket connection
- Real-time broadcasting to connected clients

### Frontend

**web/templates/index.html**
- Single-page application structure
- Sections:
  - Control panel (project selection, settings)
  - Status cards (captures, last capture)
  - Performance metrics (CPU, memory, disk, network)
  - Activity table (per-node status)
  - Event log (real-time messages)

**web/static/css/style.css**
- Dark theme design
- Responsive layout
- CSS Grid for metrics
- Progress bars and animations
- Smooth transitions

**web/static/js/app.js**
- Frontend application class (UCXSyncApp)
- WebSocket connection management
- Auto-reconnection logic
- DOM manipulation and updates
- LocalStorage for settings persistence
- API calls with fetch()

### Data Models

**pkg/models/models.go**
- `SyncTask` - Active sync task info
- `SyncStatus` - Overall sync state
- `PerformanceMetrics` - System metrics
- `CaptureInfo` - Capture file information
- `LogMessage` - Log entry structure
- `WSMessage` - WebSocket message wrapper
- `ProjectInfo` - Available project details

### Configuration & Deployment

**config.example.yaml**
- Complete configuration template
- All available options documented
- Default values provided

**ucxsync.service**
- Systemd unit file
- Service configuration
- Auto-restart on failure
- Logging to journald

**install.sh**
- Automated installation
- Prerequisites checking
- Directory creation
- Binary installation
- Service setup
- Permission configuration

**Makefile**
- `make build` - Build application
- `make build-all` - Cross-platform builds
- `make test` - Run tests
- `make clean` - Clean build artifacts
- `make install` - Install to system

## Data Flow

### Synchronization Flow

```
1. User starts sync via Web UI
   ↓
2. POST /api/sync/start → internal/web/server.go
   ↓
3. syncService.Start() → internal/sync/sync.go
   ↓
4. Mount points checked → /mnt/ucx/{node}/{share}
   ↓
5. For each node/share:
   - Scan source directory
   - Filter files needing copy
   - Parallel copy with retries
   - Track capture completion
   ↓
6. Status updates broadcast via WebSocket
   ↓
7. Frontend updates UI in real-time
```

### Monitoring Flow

```
1. Monitor service starts → internal/monitor/monitor.go
   ↓
2. Collect metrics every 1 second:
   - gopsutil CPU, Memory, Disk, Network
   ↓
3. Metrics sent to channel
   ↓
4. Web server broadcasts to clients → internal/web/server.go
   ↓
5. WebSocket sends to all connected clients
   ↓
6. Frontend updates progress bars and values
```

### Network Mounting Flow

```
1. Application starts or 'ucxsync mount' command
   ↓
2. CheckRequirements() → verify cifs-utils, root
   ↓
3. Create /mnt/ucx directory structure
   ↓
4. Create credentials file → /etc/ucxsync/credentials
   ↓
5. For each node/share:
   - Create mount point directory
   - Execute: mount -t cifs //node/share /mnt/ucx/node/share
   - Track mounted status
   ↓
6. Log results and errors
```

## Dependencies

### Go Packages

- `github.com/gorilla/websocket` - WebSocket support
- `github.com/shirou/gopsutil/v3` - System metrics
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `github.com/rs/zerolog` - Structured logging

### System Requirements

- `cifs-utils` - CIFS/SMB mounting
- `systemd` - Service management
- Root privileges - For mounting

## Build Process

1. `go mod download` - Download dependencies
2. `go build` - Compile binary
3. Embed version and build time via ldflags
4. Output: `ucxsync` executable

## Deployment Locations

- Binary: `/opt/ucxsync/ucxsync`
- Config: `/etc/ucxsync/config.yaml`
- Logs: `/var/log/ucxsync/ucxsync.log`
- Mounts: `/mnt/ucx/`
- Service: `/etc/systemd/system/ucxsync.service`

## Extension Points

### Adding New Metrics

1. Add field to `models.PerformanceMetrics`
2. Collect in `monitor.collectMetrics()`
3. Update frontend to display

### Adding New API Endpoints

1. Add handler function in `internal/web/server.go`
2. Register route in `Server.Start()`
3. Add frontend API call in `web/static/js/app.js`

### Adding New Configuration Options

1. Add field to `config.Config` struct
2. Set default in `config.setDefaults()`
3. Add to `config.example.yaml`
4. Use in relevant service

## Testing

Currently no automated tests. Future additions:

- Unit tests for sync logic
- Integration tests for mounting
- API endpoint tests
- WebSocket communication tests

Run tests:
```bash
go test ./...
```

## Maintenance

### Updating Dependencies

```bash
go get -u ./...
go mod tidy
```

### Adding New Nodes

Edit config:
```yaml
nodes:
  - WU01
  - ...
  - NEW_NODE
```

### Changing Mount Location

Edit config:
```yaml
network:
  base_mount_dir: /custom/mount/path
```

Update code in `internal/sync/sync.go` to use config value.
