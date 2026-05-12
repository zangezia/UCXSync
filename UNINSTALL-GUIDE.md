# Uninstalling UCXSync

Use the repository uninstall script whenever possible: it understands single-instance, dual-instance, and full cleanup modes.

## Preferred method

```bash
cd UCXSync
sudo ./uninstall.sh --single   # remove single-instance deployment
sudo ./uninstall.sh --dual     # remove ucxsync@a + ucxsync@b deployment
sudo ./uninstall.sh --all      # remove all deployments and shared files
```

The script stops services, unmounts share trees, removes systemd units, and then asks whether to delete configuration, mount directories, shared files, and synchronized data.

## Installed locations

Current installer-managed paths are:

- binary: `/opt/ucxsync/ucxsync`
- web assets and helpers: `/opt/ucxsync/`
- configuration: `/etc/ucxsync/`
- logs: `/var/log/ucxsync/`
- mount roots: `/ucmount`, `/ucmount-a`, `/ucmount-b`
- synchronized data: `/ucdata`

## What each uninstall mode removes

### `--single`

- stops and disables `ucxsync`
- removes `/etc/systemd/system/ucxsync.service`
- optionally removes `/etc/ucxsync/config.yaml`
- optionally removes `/ucmount`

### `--dual`

- stops and disables `ucxsync@a`, `ucxsync@b`, and optional routing service
- removes `/etc/systemd/system/ucxsync@.service`
- optionally removes `/etc/ucxsync/a.yaml` and `/etc/ucxsync/b.yaml`
- optionally removes `/ucmount-a` and `/ucmount-b`

### `--all`

- removes both single and dual deployments
- optionally removes `/opt/ucxsync` and `/var/log/ucxsync`
- optionally removes `/etc/ucxsync`
- optionally removes `/ucdata`

## Manual cleanup

If you need a manual reset, use the current install paths rather than older `/usr/local/bin` examples.

```bash
sudo systemctl stop ucxsync ucxsync@a ucxsync@b 2>/dev/null || true
sudo systemctl disable ucxsync ucxsync@a ucxsync@b 2>/dev/null || true
sudo rm -f /etc/systemd/system/ucxsync.service /etc/systemd/system/ucxsync@.service
sudo rm -rf /opt/ucxsync /etc/ucxsync /var/log/ucxsync
sudo rm -rf /ucmount /ucmount-a /ucmount-b
sudo systemctl daemon-reload
```

Delete `/ucdata` only if you intentionally want to remove synchronized data.

## Verify removal

```bash
systemctl status ucxsync
systemctl status ucxsync@a
systemctl status ucxsync@b
ls -la /opt/ucxsync
ls -la /etc/ucxsync
df -h /ucdata
```
