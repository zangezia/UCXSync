# 📁 Storage Architecture

UCXSync uses two separate Linux paths with different roles.

## `/ucmount` — temporary network mounts

This path is used for CIFS/SMB mounts from UCX nodes.

Typical structure:

```text
/ucmount/
├── WU01/
│   ├── E/
│   └── F/
├── WU02/
│   ├── E/
│   └── F/
└── CU/
    ├── E/
    └── F/
```

Properties:
- temporary;
- network-backed;
- managed by UCXSync;
- expected to be empty after unmount.

Manual preparation if needed:

```bash
sudo mkdir -p /ucmount
```

## `/ucdata` — mounted USB-SSD destination

This path is the mount point for the external USB-SSD used as the destination storage.

Config example:

```yaml
sync:
  destination: "/ucdata"
```

Typical copied-data layout:

```text
/ucdata/
├── WU01/
│   ├── E/
│   └── F/
├── WU02/
│   ├── E/
│   └── F/
└── CU/
    └── F/
```

Properties:
- persistent;
- local USB/NVMe-backed storage;
- survives network outages;
- should have enough free space for project data.

Manual mount example:

```bash
sudo mkdir -p /ucdata
sudo mount /dev/sdX1 /ucdata
df -h /ucdata
```

Optional subdirectory layout is still possible:

```yaml
sync:
  destination: "/ucdata/ucx"
```

## End-to-end data flow

```text
UCX node share
    ↓ CIFS mount
/ucmount/WU01/E/...
    ↓ incremental copy
/ucdata/WU01/E/...
```

## Quick operational checks

```bash
mount | grep /ucmount
mountpoint /ucdata
df -h /ucdata
```

## Summary

- `/ucmount` = temporary UCX network share mounts
- `/ucdata` = destination storage mounted from USB-SSD

UCXSync copies data from `/ucmount` to `/ucdata`.
