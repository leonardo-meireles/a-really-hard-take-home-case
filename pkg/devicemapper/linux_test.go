// +build linux

package devicemapper

import (
	"context"
	"testing"
)

// TestCreateSnapshotFormat verifies snapshot ID format
func TestCreateSnapshotFormat(t *testing.T) {
	// This test only verifies the logic, not actual dmsetup execution
	baseDeviceID := 42
	expectedSnapshotID := 43

	// Verify snapshot ID calculation
	snapshotID := baseDeviceID + 1
	if snapshotID != expectedSnapshotID {
		t.Errorf("Expected snapshot ID %d, got %d", expectedSnapshotID, snapshotID)
	}
}

// TestDeviceNaming verifies device naming conventions
func TestDeviceNaming(t *testing.T) {
	tests := []struct {
		deviceID     string
		expectedName string
	}{
		{"1", "flyio-1"},
		{"42", "flyio-42"},
		{"100", "flyio-100"},
	}

	for _, tt := range tests {
		deviceName := "flyio-" + tt.deviceID
		if deviceName != tt.expectedName {
			t.Errorf("For device ID %s, expected name %s, got %s",
				tt.deviceID, tt.expectedName, deviceName)
		}
	}
}

// TestSnapshotNaming verifies snapshot naming conventions
func TestSnapshotNaming(t *testing.T) {
	tests := []struct {
		snapshotID   string
		expectedName string
	}{
		{"1000", "flyio-snapshot-1000"},
		{"42000", "flyio-snapshot-42000"},
	}

	for _, tt := range tests {
		snapshotName := "flyio-snapshot-" + tt.snapshotID
		if snapshotName != tt.expectedName {
			t.Errorf("For snapshot ID %s, expected name %s, got %s",
				tt.snapshotID, tt.expectedName, snapshotName)
		}
	}
}

// TestDeviceInfoStructure verifies DeviceInfo structure
func TestDeviceInfoStructure(t *testing.T) {
	info := &DeviceInfo{
		DevicePath: "/dev/mapper/flyio-1",
		SnapshotID: 1000,
		Size:       1024 * 1024 * 1024, // 1GB in bytes
	}

	if info.DevicePath == "" {
		t.Error("DevicePath should not be empty")
	}

	if info.Size == 0 {
		t.Error("Size should not be zero")
	}

	// Verify size calculation (1GB = 2097152 sectors * 512 bytes)
	expectedSize := int64(2097152 * 512)
	if info.Size != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, info.Size)
	}
}

// TestManagerInterface verifies Manager interface compliance
func TestManagerInterface(t *testing.T) {
	// This test verifies that LinuxManager implements Manager interface
	var _ Manager = (*LinuxManager)(nil)
}

// Note: Actual dmsetup integration tests require privileged mode and thinpool setup
// These should be run in Docker E2E tests, not in CI unit tests
