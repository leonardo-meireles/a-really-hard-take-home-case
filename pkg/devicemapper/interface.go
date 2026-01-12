package devicemapper

import "context"

// DeviceInfo contains device metadata
type DeviceInfo struct {
	DevicePath string
	SnapshotID int
	Size       int64
}

// Manager manages devicemapper thin volumes
type Manager interface {
	// CreateDevice creates a thin volume from extracted image
	CreateDevice(ctx context.Context, extractedPath string, imageID string) (*DeviceInfo, error)

	// CreateSnapshot creates a snapshot of a device
	CreateSnapshot(ctx context.Context, baseDeviceID string, snapshotID int) (*DeviceInfo, error)

	// MountDevice mounts a device to the specified path
	MountDevice(ctx context.Context, devicePath, mountPath string) error

	// UnmountDevice unmounts a device from the specified path
	UnmountDevice(ctx context.Context, mountPath string) error

	// DeleteDevice removes a device
	DeleteDevice(ctx context.Context, deviceID string) error

	// ListDevices lists all managed devices
	ListDevices(ctx context.Context) ([]*DeviceInfo, error)

	// Close cleans up resources
	Close() error
}
