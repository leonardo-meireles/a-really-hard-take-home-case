// +build !linux

package devicemapper

import (
	"context"
	"fmt"
	"runtime"
)

// StubManager is a no-op devicemapper for non-Linux systems
type StubManager struct{}

// NewManager creates a stub manager on non-Linux systems
func NewManager(poolName string, dataSize, metadataSize int64) (Manager, error) {
	return &StubManager{}, nil
}

func (m *StubManager) CreateDevice(ctx context.Context, extractedPath string, imageID string) (*DeviceInfo, error) {
	return nil, fmt.Errorf("devicemapper not supported on %s", runtime.GOOS)
}

func (m *StubManager) CreateSnapshot(ctx context.Context, baseDeviceID string, snapshotID int) (*DeviceInfo, error) {
	return nil, fmt.Errorf("devicemapper not supported on %s", runtime.GOOS)
}

func (m *StubManager) MountDevice(ctx context.Context, devicePath, mountPath string) error {
	return fmt.Errorf("devicemapper not supported on %s", runtime.GOOS)
}

func (m *StubManager) UnmountDevice(ctx context.Context, mountPath string) error {
	return fmt.Errorf("devicemapper not supported on %s", runtime.GOOS)
}

func (m *StubManager) DeleteDevice(ctx context.Context, deviceID string) error {
	return fmt.Errorf("devicemapper not supported on %s", runtime.GOOS)
}

func (m *StubManager) ListDevices(ctx context.Context) ([]*DeviceInfo, error) {
	return nil, fmt.Errorf("devicemapper not supported on %s", runtime.GOOS)
}

func (m *StubManager) Close() error {
	return nil
}
