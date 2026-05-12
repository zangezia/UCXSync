# Quick Start Guide - UCXSync

This page is the shortest safe path from clone to first launch.

## 1. Install

```bash
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync
sudo ./install.sh
```

The installer places the runtime binary at `/opt/ucxsync/ucxsync`, installs web assets to `/opt/ucxsync/web`, and writes configuration to `/etc/ucxsync/`.

## 2. Configure

Edit the main config:

```bash
sudo nano /etc/ucxsync/config.yaml
```

At minimum, set:

```yaml
credentials:
  username: Administrator
  password: your_password_here

sync:
  project: YourProject
  destination: /ucdata
  max_parallelism: 8
```

If `/ucdata` is an external disk, mount it before starting the service.

## 3. Validate prerequisites

```bash
sudo /opt/ucxsync/ucxsync check --config /etc/ucxsync/config.yaml
```

## 4. Optional: mount shares manually

```bash
sudo /opt/ucxsync/ucxsync mount --config /etc/ucxsync/config.yaml
```

## 5. Start UCXSync

Run as a service:

```bash
sudo systemctl enable --now ucxsync
sudo systemctl status ucxsync
```

Or run the binary directly for a foreground session:

```bash
sudo /opt/ucxsync/ucxsync --config /etc/ucxsync/config.yaml
```

## 6. Open the web UI

```text
http://localhost:8080
```

## Daily commands

```bash
sudo /opt/ucxsync/ucxsync check --config /etc/ucxsync/config.yaml
sudo /opt/ucxsync/ucxsync mount --config /etc/ucxsync/config.yaml
sudo /opt/ucxsync/ucxsync unmount --config /etc/ucxsync/config.yaml
sudo systemctl restart ucxsync
sudo journalctl -u ucxsync -f
```

## If something misbehaves

```bash
sudo systemctl status ucxsync
sudo journalctl -u ucxsync -n 100 --no-pager
mount | grep /ucmount
df -h /ucdata
```

## Where to go next

- [`INSTALL.md`](INSTALL.md) — full installation and deployment guide
- [`ORANGEPI.md`](ORANGEPI.md) — Orange Pi RV2 / RISC-V specifics
- [`CHEATSHEET.md`](CHEATSHEET.md) — compact command reference
- [`USB-SSD-GUIDE.md`](USB-SSD-GUIDE.md) — storage setup
- [`UNINSTALL-GUIDE.md`](UNINSTALL-GUIDE.md) — clean removal
