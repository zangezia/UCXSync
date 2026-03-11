# 💾 USB-SSD Guide

This guide describes how to prepare the external destination disk for UCXSync.

## Expected mount point

The USB-SSD should be mounted at:

```text
/ucdata
```

The application writes synchronized files under that destination root.

## Quick setup

### 1. Find the device

```bash
lsblk
```

### 2. Create the mount point

```bash
sudo mkdir -p /ucdata
```

### 3. Mount the disk

```bash
sudo mount /dev/sdX1 /ucdata
df -h /ucdata
```

### 4. Fix ownership if needed

```bash
sudo chown -R $USER:$USER /ucdata
sudo chmod -R 755 /ucdata
```

## Persistent mount via `fstab`

Get the UUID:

```bash
sudo blkid /dev/sdX1
```

Add to `/etc/fstab`:

```bash
UUID=your-uuid /ucdata ext4 defaults,nofail 0 2
```

Apply and verify:

```bash
sudo mount -a
df -h /ucdata
```

## Monitoring available space

```bash
df -h /ucdata
watch -n 1 'df -h /ucdata'
```

## Safe removal

```bash
sudo systemctl stop ucxsync
sudo umount /ucdata
```

## Troubleshooting

### Device busy

```bash
sudo lsof | grep /ucdata
sudo fuser -m /ucdata
sudo umount /ucdata
```

### Wrong filesystem type

```bash
sudo blkid /dev/sdX1
sudo mount -t ntfs-3g /dev/sdX1 /ucdata
```

### Permission denied

```bash
sudo chown -R $USER:$USER /ucdata
sudo chmod -R 755 /ucdata
```

## Recommended paths summary

- USB-SSD mount: `/ucdata`
- UCX network shares: `/ucmount`
