# DeviceMapper Implementation

## Overview

The devicemapper package provides an interface for managing thin-provisioned block devices for container images.

## Architecture

### Platform Support

- **Linux**: Full devicemapper implementation using dmsetup commands (requires root)
- **Non-Linux** (macOS, Windows): Stub implementation that gracefully fails

### Components

1. **Interface** (`interface.go`)
   - `Manager` interface defining devicemapper operations
   - `DeviceInfo` struct for device metadata

2. **Tarball Extraction** (`extractor.go`)
   - Secure tar extraction with path traversal protection
   - Size and compression ratio validation
   - Works on all platforms

3. **Linux Implementation** (`linux.go`)
   - Uses dmsetup for thinpool management
   - Creates thin volumes from extracted images
   - Requires root privileges and thinpool setup

4. **Stub Implementation** (`stub.go`)
   - No-op implementation for non-Linux systems
   - Returns platform-unsupported errors

## Integration Flow

```
Download → Extract → Scan → CreateDevice
```

1. **Download**: S3 image downloaded to `/tmp/flyio-machine/downloads/`
2. **Extract**: Tarball extracted to `/tmp/flyio-machine/extracted/` with security validation
3. **Scan**: Trivy scans extracted filesystem
4. **CreateDevice**: (Optional) DeviceMapper creates thin volume on Linux

## Usage

### Basic Operation

```bash
# Fetch image (works on all platforms)
./flyio-machine fetch-and-create images/golang/1.tar

# Output on macOS:
# ⚠️  Devicemapper unavailable: devicemapper not supported on darwin
# Started FSM: <id>
# Status: ready
# ✅ Clean: No HIGH/CRITICAL vulnerabilities
```

### Linux Setup (Optional)

For full devicemapper support on Linux:

1. **Create thinpool**:
```bash
# Create sparse files for metadata and data
truncate -s 100M /tmp/metadata.img
truncate -s 10G /tmp/data.img

# Setup loop devices
METADATA_DEV=$(losetup -f --show /tmp/metadata.img)
DATA_DEV=$(losetup -f --show /tmp/data.img)

# Create thin pool
dmsetup create flyio-pool --table "0 20971520 thin-pool $METADATA_DEV $DATA_DEV 128 32768"
```

2. **Run with root**:
```bash
sudo ./flyio-machine fetch-and-create images/golang/1.tar
```

## Security Features

- **Path Traversal Protection**: Validates all tar entry paths
- **Size Limits**: Enforces max file size and total extraction size
- **Compression Bomb Detection**: Validates compression ratios
- **Symlink Validation**: Checks symlink targets for path traversal

## Limitations

- **Platform**: Full devicemapper only on Linux
- **Privileges**: Requires root on Linux
- **Thinpool**: Must be manually configured before use
- **Snapshots**: Not yet implemented

## Future Enhancements

- Snapshot support for copy-on-write images
- Automatic thinpool setup
- Device cleanup on failure
- Metrics and monitoring
