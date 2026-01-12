// +build linux

package devicemapper

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fly-io/162719/pkg/errors"
)

// LinuxManager implements devicemapper on Linux
type LinuxManager struct {
	poolName     string
	dataSize     int64
	metadataSize int64
	devices      map[string]*DeviceInfo
}

// NewManager creates a Linux devicemapper manager
func NewManager(poolName string, dataSize, metadataSize int64) (Manager, error) {
	slog.Info("devicemapper_init", "pool", poolName, "platform", "linux")

	if !isRoot() {
		slog.Error("devicemapper_requires_root")
		return nil, fmt.Errorf("devicemapper requires root privileges")
	}

	m := &LinuxManager{
		poolName:     poolName,
		dataSize:     dataSize,
		metadataSize: metadataSize,
		devices:      make(map[string]*DeviceInfo),
	}

	if err := m.initThinpool(); err != nil {
		slog.Error("thinpool_init_failed", "pool", poolName, "error", err)
		return nil, errors.Wrap(err, "failed to init thinpool")
	}

	slog.Info("devicemapper_ready", "pool", poolName)
	return m, nil
}

func (m *LinuxManager) CreateDevice(ctx context.Context, extractedPath string, deviceID string) (*DeviceInfo, error) {
	slog.Info("create_device_start", "device_id", deviceID, "pool", m.poolName)

	// deviceID is numeric (from database AUTOINCREMENT id)
	deviceName := fmt.Sprintf("flyio-%s", deviceID)
	poolDevicePath := filepath.Join("/dev/mapper", m.poolName)

	// Step 1: Create thin device metadata in pool
	// Try to delete existing device first (idempotency)
	slog.Info("delete_existing_device", "device_id", deviceID)
	deleteCmd := exec.CommandContext(ctx, "dmsetup", "message", poolDevicePath, "0",
		fmt.Sprintf("delete %s", deviceID))
	deleteCmd.Run() // Ignore errors - device may not exist

	slog.Info("create_thin_metadata", "device_id", deviceID)
	cmd := exec.CommandContext(ctx, "dmsetup", "message", poolDevicePath, "0",
		fmt.Sprintf("create_thin %s", deviceID))
	if err := cmd.Run(); err != nil {
		slog.Error("create_thin_failed", "device_id", deviceID, "error", err)
		return nil, errors.Wrap(err, "failed to create thin device metadata")
	}

	// Step 2: Activate device with dmsetup create
	// Size: 1GB = 2097152 sectors (512 bytes each)
	sectors := int64(2097152)
	tableSpec := fmt.Sprintf("0 %d thin %s %s", sectors, poolDevicePath, deviceID)
	slog.Info("activate_device", "device_name", deviceName, "sectors", sectors)

	cmd = exec.CommandContext(ctx, "dmsetup", "create", deviceName, "--table", tableSpec)
	if err := cmd.Run(); err != nil {
		slog.Error("device_activation_failed", "device_name", deviceName, "error", err)
		return nil, errors.Wrap(err, "failed to activate device")
	}

	devicePath := filepath.Join("/dev/mapper", deviceName)

	// Step 3: Format with ext4 filesystem
	slog.Info("format_device", "device_path", devicePath, "filesystem", "ext4")
	cmd = exec.CommandContext(ctx, "mkfs.ext4", "-F", devicePath)
	if err := cmd.Run(); err != nil {
		slog.Error("device_format_failed", "device_path", devicePath, "error", err)
		return nil, errors.Wrap(err, "failed to format device")
	}

	info := &DeviceInfo{
		DevicePath: devicePath,
		SnapshotID: 0,
		Size:       sectors * 512, // Convert sectors to bytes
	}

	m.devices[deviceID] = info

	slog.Info("create_device_complete", "device_id", deviceID, "device_path", devicePath, "size_mb", info.Size/1024/1024)
	return info, nil
}

func (m *LinuxManager) CreateSnapshot(ctx context.Context, baseDeviceID string, snapshotID int) (*DeviceInfo, error) {
	snapshotIDStr := fmt.Sprintf("%d", snapshotID)
	snapshotName := fmt.Sprintf("flyio-snapshot-%d", snapshotID)
	poolDevicePath := filepath.Join("/dev/mapper", m.poolName)

	slog.Info("create_snapshot_start", "base_device_id", baseDeviceID, "snapshot_id", snapshotID)

	// Step 1: Create snapshot from base device
	// Try to delete existing snapshot first (idempotency)
	slog.Info("delete_existing_snapshot", "snapshot_id", snapshotID)
	deleteCmd := exec.CommandContext(ctx, "dmsetup", "message", poolDevicePath, "0",
		fmt.Sprintf("delete %s", snapshotIDStr))
	deleteCmd.Run() // Ignore errors - snapshot may not exist

	slog.Info("create_snapshot_metadata", "snapshot_id", snapshotID, "base_device_id", baseDeviceID)
	cmd := exec.CommandContext(ctx, "dmsetup", "message", poolDevicePath, "0",
		fmt.Sprintf("create_snap %s %s", snapshotIDStr, baseDeviceID))
	if err := cmd.Run(); err != nil {
		slog.Error("snapshot_metadata_failed", "snapshot_id", snapshotID, "error", err)
		return nil, errors.Wrap(err, "failed to create snapshot metadata")
	}

	// Step 2: Activate snapshot device
	sectors := int64(2097152) // 1GB
	tableSpec := fmt.Sprintf("0 %d thin %s %s", sectors, poolDevicePath, snapshotIDStr)
	slog.Info("activate_snapshot", "snapshot_name", snapshotName, "sectors", sectors)

	cmd = exec.CommandContext(ctx, "dmsetup", "create", snapshotName, "--table", tableSpec)
	if err := cmd.Run(); err != nil {
		slog.Error("snapshot_activation_failed", "snapshot_name", snapshotName, "error", err)
		return nil, errors.Wrap(err, "failed to activate snapshot")
	}

	snapshotPath := filepath.Join("/dev/mapper", snapshotName)

	info := &DeviceInfo{
		DevicePath: snapshotPath,
		SnapshotID: snapshotID,
		Size:       sectors * 512,
	}

	slog.Info("create_snapshot_complete", "snapshot_id", snapshotID, "snapshot_path", snapshotPath, "size_mb", info.Size/1024/1024)
	return info, nil
}

func (m *LinuxManager) MountDevice(ctx context.Context, devicePath, mountPath string) error {
	slog.Info("mount_device", "device_path", devicePath, "mount_path", mountPath)

	// Mount the device to the specified path
	cmd := exec.CommandContext(ctx, "mount", devicePath, mountPath)
	if err := cmd.Run(); err != nil {
		slog.Error("mount_failed", "device_path", devicePath, "mount_path", mountPath, "error", err)
		return errors.Wrap(err, "failed to mount device")
	}

	slog.Info("mount_complete", "mount_path", mountPath)
	return nil
}

func (m *LinuxManager) UnmountDevice(ctx context.Context, mountPath string) error {
	slog.Info("unmount_device", "mount_path", mountPath)

	// Unmount the device from the specified path
	cmd := exec.CommandContext(ctx, "umount", mountPath)
	if err := cmd.Run(); err != nil {
		slog.Error("unmount_failed", "mount_path", mountPath, "error", err)
		return errors.Wrap(err, "failed to unmount device")
	}

	slog.Info("unmount_complete", "mount_path", mountPath)
	return nil
}

func (m *LinuxManager) DeleteDevice(ctx context.Context, deviceID string) error {
	deviceName := fmt.Sprintf("flyio-%s", deviceID)
	slog.Info("delete_device", "device_id", deviceID, "device_name", deviceName)

	cmd := exec.Command("dmsetup", "remove", deviceName)
	if err := cmd.Run(); err != nil {
		slog.Error("device_deletion_failed", "device_name", deviceName, "error", err)
		return errors.Wrap(err, "failed to remove device")
	}

	delete(m.devices, deviceID)
	slog.Info("device_deleted", "device_id", deviceID)
	return nil
}

func (m *LinuxManager) ListDevices(ctx context.Context) ([]*DeviceInfo, error) {
	devices := make([]*DeviceInfo, 0, len(m.devices))
	for _, dev := range m.devices {
		devices = append(devices, dev)
	}
	return devices, nil
}

func (m *LinuxManager) Close() error {
	return nil
}

func (m *LinuxManager) initThinpool() error {
	slog.Info("checking_thinpool", "pool", m.poolName)

	// Check if thinpool already exists
	cmd := exec.Command("dmsetup", "info", m.poolName)
	if err := cmd.Run(); err == nil {
		slog.Info("thinpool_exists", "pool", m.poolName)
		return nil
	}

	slog.Error("thinpool_not_found", "pool", m.poolName)
	return fmt.Errorf("thinpool setup requires manual configuration - see docs")
}

func isRoot() bool {
	cmd := exec.Command("id", "-u")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "0"
}
