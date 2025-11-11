# Building UCXSync for Orange Pi RV2 (RISC-V)

## Prerequisites

- Go 1.21 or higher
- Git

## Build on Orange Pi RV2 directly

```bash
# Clone repository
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync

# Build for RISC-V 64-bit
make build-riscv64

# Or manually:
GOOS=linux GOARCH=riscv64 go build -o ucxsync ./cmd/ucxsync
```

## Cross-compile from x86_64 Linux

```bash
# Build for RISC-V 64-bit
GOOS=linux GOARCH=riscv64 go build -o ucxsync-riscv64 ./cmd/ucxsync

# Build for ARM64 (if needed)
GOOS=linux GOARCH=arm64 go build -o ucxsync-arm64 ./cmd/ucxsync

# Transfer to Orange Pi
scp ucxsync-riscv64 user@orangepi:/tmp/
```

## Cross-compile from Windows

```powershell
# Using PowerShell - Build for RISC-V
$env:GOOS='linux'
$env:GOARCH='riscv64'
go build -o ucxsync-riscv64 ./cmd/ucxsync
$env:GOOS=''
$env:GOARCH=''

# Or for ARM64
$env:GOOS='linux'
$env:GOARCH='arm64'
go build -o ucxsync-arm64 ./cmd/ucxsync
$env:GOOS=''
$env:GOARCH=''

# Transfer using WinSCP or similar
```

## Verify binary

```bash
# On Orange Pi RV2
file ucxsync-riscv64
# Should output: ELF 64-bit LSB executable, UCB RISC-V, ...

# Test run
./ucxsync-riscv64 --version
```

## Makefile targets

```bash
make build          # Build for Linux AMD64
make build-riscv64  # Build for Linux RISC-V 64-bit (Orange Pi RV2)
make build-arm64    # Build for Linux ARM64
make build-all      # Build all architectures
make test           # Run tests
make clean          # Clean build artifacts
```
