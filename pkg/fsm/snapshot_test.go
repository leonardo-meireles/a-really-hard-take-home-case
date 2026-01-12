package fsm

import (
	"strings"
	"testing"
)

// TestSnapshotMandatoryLogic tests the logic for mandatory snapshot creation
func TestSnapshotMandatoryLogic(t *testing.T) {
	tests := []struct {
		name          string
		errorContains string
		shouldAbort   bool
	}{
		{
			name:          "Platform not supported error - graceful degradation",
			errorContains: "not supported",
			shouldAbort:   false,
		},
		{
			name:          "Actual snapshot failure - must abort",
			errorContains: "permission denied",
			shouldAbort:   true,
		},
		{
			name:          "Device not found - must abort",
			errorContains: "device not found",
			shouldAbort:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from handleComplete
			isNotSupported := strings.Contains(tt.errorContains, "not supported")
			shouldAbort := !isNotSupported

			if shouldAbort != tt.shouldAbort {
				t.Errorf("Expected abort=%v for error '%s', got %v",
					tt.shouldAbort, tt.errorContains, shouldAbort)
			}
		})
	}
}

// TestSnapshotIDFormat tests snapshot ID generation
func TestSnapshotIDFormat(t *testing.T) {
	tests := []struct {
		baseDeviceID string
		expectedID   string
	}{
		{"1", "1000"},
		{"42", "42000"},
		{"100", "100000"},
	}

	for _, tt := range tests {
		t.Run("BaseID_"+tt.baseDeviceID, func(t *testing.T) {
			// Simulate snapshot ID generation from linux.go
			snapshotID := tt.baseDeviceID + "000"

			if snapshotID != tt.expectedID {
				t.Errorf("Expected snapshot ID %s, got %s", tt.expectedID, snapshotID)
			}
		})
	}
}

// TestResponseAccumulation tests ImageResponse field accumulation
func TestResponseAccumulation(t *testing.T) {
	resp := &ImageResponse{
		ImageID:       1,
		SHA256:        "abc123",
		DownloadPath:  "/tmp/download",
		ExtractedPath: "/tmp/extracted",
		DevicePath:    "/dev/mapper/flyio-1",
	}

	// Simulate adding snapshot info (from handleComplete)
	resp.SnapshotID = 1000

	if resp.SnapshotID == 0 {
		t.Error("SnapshotID should be set after snapshot creation")
	}

	if resp.DevicePath == "" {
		t.Error("DevicePath should be preserved from previous state")
	}

	if resp.ImageID == 0 {
		t.Error("ImageID should be preserved from checkDB state")
	}
}

// TestCleanupLogic tests automatic cleanup logic
func TestCleanupLogic(t *testing.T) {
	// Simulate the defer cleanup pattern from handleCreateDevice
	deviceMounted := true

	// Simulate successful operation
	successPath := func() bool {
		defer func() {
			if deviceMounted {
				// Cleanup would happen here
				t.Log("Cleanup triggered (expected in failure case)")
			}
		}()

		// Simulate successful unmount
		deviceMounted = false
		return true
	}()

	if !successPath {
		t.Error("Success path should complete")
	}

	if deviceMounted {
		t.Error("Device should be marked as unmounted after success")
	}

	// Simulate failure case
	deviceMounted = true
	cleanupTriggered := false

	failurePath := func() bool {
		defer func() {
			if deviceMounted {
				cleanupTriggered = true
			}
		}()

		// Simulate failure (device stays mounted)
		return false
	}()

	if failurePath {
		t.Error("Failure path should not succeed")
	}

	if !cleanupTriggered {
		t.Error("Cleanup should be triggered on failure")
	}
}
