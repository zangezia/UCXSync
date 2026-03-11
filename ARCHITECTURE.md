# UCXSync Architecture

Technical overview of the current codebase, runtime model, and integration points.

## Purpose

`UCXSync` is a Linux-only service for pulling capture files from multiple UCX nodes over CIFS/SMB shares, copying them to local storage, and exposing runtime status through a small web UI.

At runtime the application does three main things:

1. mounts network shares from UCX nodes;
2. scans project directories and incrementally copies new files;
3. publishes status and system metrics over HTTP/WebSocket.

## Runtime model

### High-level flow

```text
CLI / systemd
    ↓
load config + apply CLI overrides
    ↓
start web server
    ↓
check mount requirements and mount CIFS shares
    ↓
start monitoring loop
    ↓
on user request start sync loop
    ↓
scan mounted shares → copy files → update status
    ↓
broadcast status + metrics to browser clients
```

### Deployment assumptions

- OS: Linux
- Privileges: root or `sudo` required for CIFS mount/unmount operations
- System tools: `mount`, `umount`, `mount.cifs`, `lsblk`
- Filesystem interfaces used directly:
  - `/proc/mounts`
  - `/ucmount`
  - `/ucdata` (default removable-storage mount target in the web UI)

The Go application can be built on Windows, but the operational feature set is Linux-only because network mounting and block-device management depend on Linux system interfaces.

## Code layout

```text
UCXSync/
├── cmd/ucxsync/            # CLI entry point and subcommands
├── internal/
│   ├── config/             # Config loading, defaults, validation
│   ├── monitor/            # Runtime system metrics
│   ├── network/            # CIFS mount / unmount management
│   ├── sync/               # File discovery, copy, capture tracking
│   └── web/                # HTTP API, WebSocket, storage-device actions
├── pkg/models/             # Shared API / websocket models
├── web/                    # HTML, JS, CSS assets
├── cpp/                    # Experimental Linux-only C++ port scaffold
├── config.example.yaml     # Reference configuration
└── ucxsync.service         # systemd unit
```

## Components

### `cmd/ucxsync`

Responsible for process startup and CLI wiring.

- `main.go`
  - defines root command and persistent flags;
  - loads configuration;
  - applies CLI overrides (`--project`, `--dest`, `--port`, `--parallelism`);
  - starts the web server;
  - handles shutdown signals.
- `commands.go`
  - `mount` — mount all configured CIFS shares;
  - `unmount` — unmount tracked shares;
  - `check` — validate config and required Linux dependencies.

### `internal/config`

Configuration layer backed by `viper`.

Responsibilities:

- load `config.yaml` from the working directory, `$HOME/.ucxsync`, or `/etc/ucxsync`;
- apply built-in defaults;
- support `UCXSYNC_*` environment overrides;
- validate essential fields (`nodes`, `shares`, `web.port`, `sync.max_parallelism`);
- persist lightweight UI settings via `SaveSettings()` / `LoadSettings()`.

Important detail: the current runtime configuration does **not** expose a configurable `network.base_mount_dir`; code still uses `/ucmount` directly.

### `internal/network`

Linux-specific mount manager for worker-node shares.

Responsibilities:

- create local mount directory layout under `/ucmount/{node}/{share}`;
- write `/etc/ucxsync/credentials` when possible;
- mount shares using `mount -t cifs` with SMB1 compatibility (`vers=1.0`);
- track mounted shares in memory for later unmount;
- verify prerequisites with `CheckRequirements()`.

Hard dependency on:

- `mount.cifs` from `cifs-utils`;
- root privileges.

### `internal/sync`

Core incremental synchronization engine.

Responsibilities:

- find project directories on mounted shares;
- periodically scan source trees;
- copy only missing or changed files;
- cap concurrent copy operations via a global semaphore;
- aggregate per-task statistics for the UI;
- detect completed captures from file naming conventions.

Capture logic:

- normal capture = 13 RAW + 1 XML;
- test capture = 13 RAW, XML optional.

Current limitation:

- `checkDiskSpace()` is still a stub and always returns `true`.

### `internal/monitor`

Collects system metrics using `gopsutil`.

Publishes:

- CPU usage (smoothed);
- memory usage;
- disk throughput and free space;
- network throughput.

Metrics are broadcast to connected browsers through the web server.

### `internal/web`

HTTP server plus WebSocket broadcaster.

Routes currently exposed:

- `GET /` — web UI;
- `GET /api/projects` — discover available projects on mounted shares;
- `GET /api/destinations` — list mounted external destinations;
- `GET /api/devices` — list block devices via `lsblk`;
- `POST /api/devices/mount` — mount/unmount a block device to `/ucdata`;
- `GET /api/status` — current sync state;
- `POST /api/sync/start` — start synchronization;
- `POST /api/sync/stop` — stop synchronization;
- `GET /ws` — real-time websocket stream.

WebSocket message types currently sent by the backend:

- `status`
- `metrics`
- `log`

### `pkg/models`

Shared models used by HTTP and WebSocket layers:

- sync task status;
- aggregate sync status;
- system metrics;
- project / destination / block-device descriptors;
- websocket payload envelopes.

### `cpp/`

Experimental Linux-only porting workspace for C++.

Scope today:

- scaffolding only;
- config-port prototype area;
- not used by the Go runtime.

## Data flow

### Starting synchronization

```text
Browser UI
  ↓ POST /api/sync/start
internal/web.Server.handleStartSync
  ↓
monitor target disk set
  ↓
internal/sync.Service.Start
  ↓
background sync loop ticks every service_loop_interval
  ↓
scan project directories on mounted shares
  ↓
copy changed files with global parallelism limit
  ↓
aggregate status → broadcast over WebSocket
```

### Metrics broadcast

```text
internal/monitor.Service.Start
  ↓
collect metrics on interval
  ↓
channel to web server
  ↓
periodic broadcast to all websocket clients
```

## Current gaps and technical debt

- mount root (`/ucmount`) is hard-coded in multiple places;
- disk-space enforcement is not implemented yet;
- Linux assumptions are spread through `network` and `web` code;
- mount and block-device actions call external commands directly, which makes unit testing harder;
- C++ port is still experimental and not feature-complete.

## Testing status

Automated tests currently exist for part of the sync package (`internal/sync/sync_test.go`).

Typical checks:

```bash
go test ./...
go vet ./...
go build ./cmd/ucxsync
```

## Recommended refactoring directions

1. Introduce a configurable mount root in `internal/config` and consume it in `network`, `sync`, and `web`.
2. Implement real free-space checks in `internal/sync`.
3. Wrap `mount`, `umount`, and `lsblk` behind interfaces to simplify testing.
4. Separate Linux-only adapters from platform-neutral business logic.
5. Grow the C++ port in isolation until config loading and validation match the Go implementation.
