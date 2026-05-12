# UCXSync Cheat Sheet

## Paths

- network shares mount under: `/ucmount`
- USB-SSD destination mounts under: `/ucdata`

## Quick install

```bash
git clone https://github.com/zangezia/UCXSync.git
cd UCXSync
sudo ./install.sh
```

## Manual disk setup

```bash
sudo mkdir -p /ucdata
sudo mount /dev/sdX1 /ucdata
df -h /ucdata
```

## Manual share mount root

```bash
sudo mkdir -p /ucmount
```

## Common commands

```bash
sudo /opt/ucxsync/ucxsync check --config /etc/ucxsync/config.yaml
sudo /opt/ucxsync/ucxsync mount --config /etc/ucxsync/config.yaml
sudo /opt/ucxsync/ucxsync unmount --config /etc/ucxsync/config.yaml
sudo /opt/ucxsync/ucxsync --config /etc/ucxsync/config.yaml
sudo systemctl restart ucxsync
```

## Typical config fragment

```yaml
sync:
  destination: "/ucdata"
  max_parallelism: 8
```

## Diagnostics

```bash
mount | grep /ucmount
mountpoint /ucdata
df -h /ucdata
sudo journalctl -u ucxsync -f
```

## Cleanup

```bash
sudo umount /ucmount/* 2>/dev/null || true
sudo umount /ucdata 2>/dev/null || true
```
