# UCXSync

Linux-only file synchronization service for UCX projects with a browser-based monitoring UI.

## What the application does

`UCXSync` connects to multiple UCX worker nodes over CIFS/SMB, scans project folders on mounted shares, and incrementally copies capture files to local storage.

The application combines three subsystems:

- **network mounting** — mount/unmount CIFS shares from UCX nodes;
- **synchronization** — detect new or changed files and copy them in parallel;
- **web monitoring** — show status, logs, and host metrics in real time.

## Runtime environment

### Supported environment

- **OS**: Linux
- **Architectures**: AMD64, ARM64, RISC-V 64
- **Privileges**: root / `sudo` required for mount operations
- **External tools**:
  - `cifs-utils` (`mount.cifs`)
  - `mount` / `umount`
  - `lsblk`
- **Go**: 1.21+ for building from source

### Important limitation

The codebase can be built on non-Linux hosts for development, but the full runtime feature set depends on Linux-specific interfaces such as `/proc/mounts`, CIFS mounting, and block-device management.

## Core features

- Incremental synchronization from 14 nodes (`WU01`-`WU13` + `CU`)
- Multiple shares per node (default: `E$`, `F$`)
- Global configurable parallelism for file-copy operations
- Capture completion tracking:
  - normal capture = 13 RAW + 1 XML;
  - test capture = 13 RAW, XML optional
- Web UI with real-time status and metrics
- Removable-storage discovery and mount/unmount helpers in the web API

## How it works

1. Load config from `config.yaml` or built-in defaults.
2. Optionally mount remote shares under `/ucmount`.
3. Start the web server.
4. When synchronization starts, scan mounted project directories.
5. Copy only missing or modified files into the target destination.
6. Broadcast status, logs, CPU, memory, disk, and network metrics to the UI.

## Commands

```bash
ucxsync
ucxsync mount
ucxsync unmount
ucxsync check
```

Common flags:

```bash
ucxsync --config /path/to/config.yaml
ucxsync --debug
ucxsync --project MyProject --dest /ucdata
ucxsync --port 9090
ucxsync --parallelism 8
```

## Quick start

### Build from source

```bash
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync
go build -o ucxsync ./cmd/ucxsync
```

### Check prerequisites

```bash
sudo ./ucxsync check
```

### Run the service

```bash
sudo ./ucxsync --config /etc/ucxsync/config.yaml
```

Open the UI at:

```text
http://localhost:8080
```

## Configuration

Use `config.example.yaml` as the source of truth. The currently supported top-level sections are:

- `nodes`
- `shares`
- `credentials`
- `sync`
- `web`
- `monitoring`
- `logging`

Minimal example:

```yaml
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

credentials:
  username: Administrator
  password: ultracam

sync:
  project: Arh2k_mezen_200725
  destination: /ucdata
  max_parallelism: 8
  service_loop_interval: 10s
  min_free_disk_space: 52428800
  disk_space_safety_margin: 104857600

web:
  host: 0.0.0.0
  port: 8080

monitoring:
  performance_update_interval: 1s
  ui_update_interval: 2s
  cpu_smoothing_samples: 3
  max_disk_throughput_mbps: 200.0
  network_speed_bps: 1000000000

logging:
  level: info
  file: /var/log/ucxsync/ucxsync.log
  max_size: 100
  max_backups: 5
  max_age: 30
```

## HTTP and WebSocket API

### REST endpoints

- `GET /api/projects`
- `GET /api/destinations`
- `GET /api/devices`
- `POST /api/devices/mount`
- `GET /api/status`
- `POST /api/sync/start`
- `POST /api/sync/stop`

### WebSocket endpoint

- `GET /ws`

Current message types:

- `status`
- `metrics`
- `log`

## Capture naming rules

### RAW files

Examples:

```text
Lvl00-00001-Arh2k_mezen_200725-06-00-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.raw
Lvl0X-00002-T-Arh2k_mezen_200725-06-00-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.raw
```

### XML metadata files

```text
EAD-00001-Arh2k_mezen_200725-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.xml
```

Completion rules:

- **normal capture**: 13 RAW + 1 XML
- **test capture**: 13 RAW, XML optional

## Project layout

```text
cmd/ucxsync/      CLI and process startup
internal/config/  config loading and validation
internal/network/ Linux CIFS mount management
internal/sync/    synchronization engine
internal/monitor/ host metrics collection
internal/web/     HTTP API and WebSocket server
pkg/models/       shared API models
web/              frontend assets
cpp/              experimental Linux-only C++ port scaffold
```

See also: [`ARCHITECTURE.md`](ARCHITECTURE.md).

## Documentation map

- [`README.ru.md`](README.ru.md) — quick start in Russian
- [`LINUX.md`](LINUX.md) — Linux deployment notes
- [`ORANGEPI.md`](ORANGEPI.md) — Orange Pi / RISC-V deployment
- [`BUILD.md`](BUILD.md) — build instructions
- [`TEST.md`](TEST.md) — testing guide
- [`PARALLELISM.md`](PARALLELISM.md) — concurrency notes
- [`USB-SSD-GUIDE.md`](USB-SSD-GUIDE.md) — external storage setup
- [`STORAGE-ARCHITECTURE.md`](STORAGE-ARCHITECTURE.md) — mount and data layout

## Testing and verification

Automated tests currently cover part of the sync package.

Typical local checks:

```bash
go test ./...
go vet ./...
go build ./cmd/ucxsync
./ucxsync --help
```

## Current limitations

- mount root is hard-coded to `/ucmount` in several places;
- free-space enforcement is not implemented yet in the sync service;
- block-device and mount operations are Linux-specific and shell out to system tools;
- the C++ port is experimental and not part of the runtime path.

## Experimental C++ port

The `cpp/` directory is a Linux-only sandbox for porting selected components, starting with configuration loading. It is intentionally isolated from the production Go runtime.

## License

MIT License.
