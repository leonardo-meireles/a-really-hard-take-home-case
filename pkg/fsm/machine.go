// Package fsm implements the image processing finite state machine workflow.
// It orchestrates the download, validation, device creation, and snapshot activation
// of container images from S3 using the superfly/fsm library.
package fsm

import (
	"context"
	"runtime"

	"github.com/fly-io/162719/pkg/devicemapper"
	"github.com/fly-io/162719/pkg/errors"
	"github.com/superfly/fsm"
)

// Register registers the image processing FSM
func (m *Machine) Register(ctx context.Context, manager *fsm.Manager) (fsm.Start[ImageRequest, ImageResponse], fsm.Resume, error) {
	start, resume, err := fsm.Register[ImageRequest, ImageResponse](manager, "image-process").
		Start(StateCheckDB, m.handleCheckDB).
		To(StateDownload, m.handleDownload).
		To(StateValidate, m.handleValidate).
		To(StateCreateDevice, m.handleCreateDevice).
		To(StateComplete, m.handleComplete).
		End(StateFailed).
		Build(ctx)

	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to register FSM")
	}

	return start, resume, nil
}

// CheckDeviceMapperHealth checks if DeviceMapper is available and functional.
// Returns "ok" if healthy, "not_available" if not on Linux, or error description otherwise.
func CheckDeviceMapperHealth(ctx context.Context) string {
	if runtime.GOOS != "linux" {
		return "not_available"
	}

	// Try to create a DeviceMapper manager to check if thinpool is configured
	_, err := devicemapper.NewManager("pool", 0, 0)
	if err != nil {
		return err.Error()
	}

	return "ok"
}
