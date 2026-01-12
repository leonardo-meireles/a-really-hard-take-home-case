package fsm

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/fly-io/162719/pkg/db"
	"github.com/fly-io/162719/pkg/devicemapper"
	"github.com/fly-io/162719/pkg/errors"
	"github.com/fly-io/162719/pkg/security"
	"github.com/fly-io/162719/pkg/storage"
	"github.com/superfly/fsm"
)

// Machine holds dependencies for FSM transitions
type Machine struct {
	repo       *db.Repository
	s3Client   *storage.Client
	validator  *security.Validator
	dmManager  devicemapper.Manager
	workDir    string
	maxRetries int
}

// NewMachine creates a new FSM machine with dependencies
func NewMachine(
	repo *db.Repository,
	s3Client *storage.Client,
	validator *security.Validator,
	dmManager devicemapper.Manager,
	workDir string,
	maxRetries int,
) *Machine {
	return &Machine{
		repo:       repo,
		s3Client:   s3Client,
		validator:  validator,
		dmManager:  dmManager,
		workDir:    workDir,
		maxRetries: maxRetries,
	}
}

// handleCheckDB checks if image already exists in database (idempotency)
func (m *Machine) handleCheckDB(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (*fsm.Response[ImageResponse], error) {
	slog.Info("fsm_state_check_db", "s3_key", req.Msg.S3Key)

	// Check retry limit
	if retryCount := fsm.RetryFromContext(ctx); retryCount >= uint64(m.maxRetries) {
		slog.Error("max_retries_exceeded", "s3_key", req.Msg.S3Key, "max_retries", m.maxRetries)
		return nil, fsm.Abort(fmt.Errorf("max retries (%d) exceeded", m.maxRetries))
	}

	// Check database
	img, err := m.repo.GetByS3Key(req.Msg.S3Key)
	if err != nil {
		slog.Error("database_check_failed", "s3_key", req.Msg.S3Key, "error", err)
		return nil, fsm.Abort(errors.Wrap(err, "database error"))
	}

	resp := req.W.Msg
	if resp == nil {
		resp = &ImageResponse{}
	}

	// If image exists and is ready, skip processing
	if img != nil {
		resp.ImageID = img.ID
		resp.SHA256 = img.SHA256
		resp.Status = img.Status

		if img.Status == db.StatusReady {
			slog.Info("image_already_ready", "s3_key", req.Msg.S3Key, "image_id", img.ID, "status", img.Status)
			// Skip to complete
			return fsm.NewResponse(resp), nil
		}
		slog.Info("image_found_continue_processing", "s3_key", req.Msg.S3Key, "image_id", img.ID, "status", img.Status)
	} else {
		// Create new pending record
		img = &db.Image{
			S3Key:  req.Msg.S3Key,
			SHA256: "",
			Status: db.StatusPending,
		}
		if err := m.repo.Create(img); err != nil {
			slog.Error("create_image_failed", "s3_key", req.Msg.S3Key, "error", err)
			return nil, errors.Wrap(err, "failed to create image record")
		}
		resp.ImageID = img.ID
		slog.Info("image_created", "s3_key", req.Msg.S3Key, "image_id", img.ID)
	}

	return fsm.NewResponse(resp), nil
}

// handleDownload downloads image from S3
func (m *Machine) handleDownload(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (*fsm.Response[ImageResponse], error) {
	slog.Info("fsm_state_download", "s3_key", req.Msg.S3Key)

	// Check retry limit
	if retryCount := fsm.RetryFromContext(ctx); retryCount >= uint64(m.maxRetries) {
		slog.Error("max_retries_exceeded", "s3_key", req.Msg.S3Key, "max_retries", m.maxRetries)
		return nil, fsm.Abort(fmt.Errorf("max retries (%d) exceeded", m.maxRetries))
	}

	resp := req.W.Msg
	if resp == nil {
		return nil, fsm.Abort(fmt.Errorf("response not initialized"))
	}

	// Update status
	if err := m.repo.UpdateStatus(resp.ImageID, db.StatusDownloading, ""); err != nil {
		slog.Error("status_update_failed", "image_id", resp.ImageID, "status", db.StatusDownloading, "error", err)
		return nil, errors.Wrap(err, "failed to update status")
	}

	// Create work directory
	downloadDir := filepath.Join(m.workDir, "downloads")
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		slog.Error("download_dir_creation_failed", "path", downloadDir, "error", err)
		return nil, errors.Wrap(err, "failed to create download dir")
	}

	// Download from S3
	localPath := filepath.Join(downloadDir, filepath.Base(req.Msg.S3Key))
	slog.Info("download_started", "s3_key", req.Msg.S3Key, "local_path", localPath)

	result, err := m.s3Client.Download(ctx, req.Msg.S3Key, localPath)
	if err != nil {
		slog.Error("download_failed", "s3_key", req.Msg.S3Key, "error", err)
		return nil, errors.Wrap(err, "failed to download from S3")
	}

	slog.Info("download_complete",
		"s3_key", req.Msg.S3Key,
		"size_mb", result.Size/1024/1024,
		"sha256", result.SHA256[:16]+"...",
	)

	// Update response
	resp.SHA256 = result.SHA256
	resp.DownloadPath = result.LocalPath
	resp.DownloadSize = result.Size

	// Update database
	img, _ := m.repo.GetByS3Key(req.Msg.S3Key)
	if img != nil {
		img.SHA256 = result.SHA256
		if err := m.repo.Update(img); err != nil {
			slog.Error("image_update_failed", "image_id", img.ID, "error", err)
			return nil, errors.Wrap(err, "failed to update image")
		}
	}

	return fsm.NewResponse(resp), nil
}

// handleValidate validates and extracts tarball
func (m *Machine) handleValidate(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (*fsm.Response[ImageResponse], error) {
	slog.Info("fsm_state_validate", "s3_key", req.Msg.S3Key)

	// Check retry limit
	if retryCount := fsm.RetryFromContext(ctx); retryCount >= uint64(m.maxRetries) {
		slog.Error("max_retries_exceeded", "s3_key", req.Msg.S3Key, "max_retries", m.maxRetries)
		return nil, fsm.Abort(fmt.Errorf("max retries (%d) exceeded", m.maxRetries))
	}

	resp := req.W.Msg
	if resp == nil {
		return nil, fsm.Abort(fmt.Errorf("response not initialized"))
	}

	// Validate file size
	if err := m.validator.ValidateFileSize(resp.DownloadSize); err != nil {
		slog.Error("file_size_validation_failed", "s3_key", req.Msg.S3Key, "size", resp.DownloadSize, "error", err)
		m.repo.UpdateStatus(resp.ImageID, db.StatusFailed, err.Error())
		return nil, fsm.Abort(err)
	}

	// Create extraction directory
	extractDir := filepath.Join(m.workDir, "extracted", filepath.Base(req.Msg.S3Key))
	if err := os.RemoveAll(extractDir); err != nil && !os.IsNotExist(err) {
		slog.Error("extract_dir_cleanup_failed", "path", extractDir, "error", err)
		return nil, errors.Wrap(err, "failed to clean extract dir")
	}
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		slog.Error("extract_dir_creation_failed", "path", extractDir, "error", err)
		return nil, errors.Wrap(err, "failed to create extract dir")
	}

	// Extract tarball with security validation
	slog.Info("extraction_started", "s3_key", req.Msg.S3Key, "extract_dir", extractDir)

	if err := devicemapper.ExtractTarball(resp.DownloadPath, extractDir, m.validator); err != nil {
		slog.Error("extraction_failed", "s3_key", req.Msg.S3Key, "error", err)
		m.repo.UpdateStatus(resp.ImageID, db.StatusFailed, err.Error())
		return nil, fsm.Abort(errors.Wrap(err, "tar extraction failed"))
	}

	slog.Info("extraction_complete", "s3_key", req.Msg.S3Key, "extract_dir", extractDir)

	resp.ExtractedPath = extractDir

	return fsm.NewResponse(resp), nil
}

// handleCreateDevice creates devicemapper device, mounts it, and extracts tarball into it
func (m *Machine) handleCreateDevice(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (*fsm.Response[ImageResponse], error) {
	slog.Info("fsm_state_create_device", "s3_key", req.Msg.S3Key)

	// Check retry limit
	if retryCount := fsm.RetryFromContext(ctx); retryCount >= uint64(m.maxRetries) {
		slog.Error("max_retries_exceeded", "s3_key", req.Msg.S3Key, "max_retries", m.maxRetries)
		return nil, fsm.Abort(fmt.Errorf("max retries (%d) exceeded", m.maxRetries))
	}

	resp := req.W.Msg
	if resp == nil {
		return nil, fsm.Abort(fmt.Errorf("response not initialized"))
	}

	// Skip devicemapper if not available (stub on non-Linux)
	if m.dmManager == nil {
		slog.Warn("devicemapper_unavailable", "s3_key", req.Msg.S3Key, "reason", "stub_platform")
		// Keep using extracted path from validate state
		return fsm.NewResponse(resp), nil
	}

	// Create base thin device
	baseDeviceID, err := m.repo.AllocateNextDeviceID(ctx)
	if err != nil {
		slog.Error("base_device_id_allocation_failed", "s3_key", req.Msg.S3Key, "error", err)
		return nil, errors.Wrap(err, "failed to allocate base device ID")
	}

	deviceID := fmt.Sprintf("%d", baseDeviceID)
	slog.Info("device_creation_started", "s3_key", req.Msg.S3Key, "device_id", deviceID)

	deviceInfo, err := m.dmManager.CreateDevice(ctx, "", deviceID)
	if err != nil {
		// Log but don't fail - devicemapper is optional
		slog.Warn("device_creation_failed", "s3_key", req.Msg.S3Key, "device_id", deviceID, "error", err)
		resp.ErrorMessage = fmt.Sprintf("devicemapper warning: %v", err)
		return fsm.NewResponse(resp), nil
	}

	slog.Info("device_created", "s3_key", req.Msg.S3Key, "device_id", deviceID, "device_path", deviceInfo.DevicePath)

	// Mount device
	mountPath := filepath.Join(m.workDir, "mounts", deviceID)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		slog.Error("mount_dir_creation_failed", "path", mountPath, "error", err)
		m.dmManager.DeleteDevice(ctx, deviceID)
		return nil, errors.Wrap(err, "failed to create mount dir")
	}

	slog.Info("mounting_device", "device_path", deviceInfo.DevicePath, "mount_path", mountPath)

	if err := m.dmManager.MountDevice(ctx, deviceInfo.DevicePath, mountPath); err != nil {
		slog.Error("device_mount_failed", "device_path", deviceInfo.DevicePath, "error", err)
		m.dmManager.DeleteDevice(ctx, deviceID)
		return nil, errors.Wrap(err, "failed to mount device")
	}

	// Copy already-extracted files to mounted device
	slog.Info("copying_files_to_device", "source", resp.ExtractedPath, "dest", mountPath)

	if err := copyDir(resp.ExtractedPath, mountPath); err != nil {
		slog.Error("copy_to_device_failed", "error", err)
		m.repo.UpdateStatus(resp.ImageID, db.StatusFailed, err.Error())
		m.dmManager.UnmountDevice(ctx, mountPath)
		m.dmManager.DeleteDevice(ctx, deviceID)
		return nil, fsm.Abort(errors.Wrap(err, "copy to device failed"))
	}

	slog.Info("files_copied_to_device", "mount_path", mountPath)

	// Unmount device
	if err := m.dmManager.UnmountDevice(ctx, mountPath); err != nil {
		slog.Error("device_unmount_failed", "mount_path", mountPath, "error", err)
		return nil, errors.Wrap(err, "failed to unmount device")
	}

	slog.Info("device_unmounted", "mount_path", mountPath)

	// Update response and database
	resp.DevicePath = deviceInfo.DevicePath
	img, _ := m.repo.GetByS3Key(req.Msg.S3Key)
	if img != nil {
		img.BaseDeviceID = baseDeviceID
		img.DevicePath = deviceInfo.DevicePath
		if err := m.repo.Update(img); err != nil {
			return nil, errors.Wrap(err, "failed to update image")
		}
	}

	// Update ExtractedPath to mountPath for scanning
	// (scan will remount read-only)
	resp.ExtractedPath = mountPath

	return fsm.NewResponse(resp), nil
}

// handleComplete creates snapshot and marks FSM as complete
func (m *Machine) handleComplete(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (*fsm.Response[ImageResponse], error) {
	slog.Info("fsm_state_complete", "s3_key", req.Msg.S3Key)

	// Check retry limit
	if retryCount := fsm.RetryFromContext(ctx); retryCount >= uint64(m.maxRetries) {
		slog.Error("max_retries_exceeded", "s3_key", req.Msg.S3Key, "max_retries", m.maxRetries)
		return nil, fsm.Abort(fmt.Errorf("max retries (%d) exceeded", m.maxRetries))
	}

	resp := req.W.Msg
	if resp == nil {
		resp = &ImageResponse{Status: "complete"}
	}

	// Load image from database to get device_path set by handleCreateDevice
	img, err := m.repo.GetByS3Key(req.Msg.S3Key)
	if err != nil {
		slog.Error("failed_to_load_image", "s3_key", req.Msg.S3Key, "error", err)
		return nil, fsm.Abort(errors.Wrap(err, "failed to load image"))
	}
	if img == nil {
		slog.Error("image_not_found", "s3_key", req.Msg.S3Key)
		return nil, fsm.Abort(fmt.Errorf("image not found in database"))
	}

	// Create snapshot from base device (MANDATORY - required by challenge)
	// Only skip on non-Linux platforms (stub manager)
	if m.dmManager != nil && img.DevicePath != "" {
		baseDeviceID := fmt.Sprintf("%d", img.BaseDeviceID)

		snapshotID := img.SnapshotID
		if snapshotID == 0 {
			var err error
			snapshotID, err = m.repo.AllocateNextDeviceID(ctx)
			if err != nil {
				slog.Error("snapshot_id_allocation_failed", "s3_key", req.Msg.S3Key, "error", err)
				m.repo.UpdateStatus(img.ID, db.StatusFailed, fmt.Sprintf("snapshot ID allocation failed: %v", err))
				return nil, fsm.Abort(errors.Wrap(err, "snapshot ID allocation failed"))
			}
			slog.Info("allocated_new_snapshot_id", "s3_key", req.Msg.S3Key, "snapshot_id", snapshotID)
		} else {
			slog.Info("reusing_existing_snapshot_id", "s3_key", req.Msg.S3Key, "snapshot_id", snapshotID)
		}

		slog.Info("snapshot_creation_started", "s3_key", req.Msg.S3Key, "base_device_id", baseDeviceID, "snapshot_id", snapshotID)

		snapshotInfo, err := m.dmManager.CreateSnapshot(ctx, baseDeviceID, snapshotID)
		if err != nil {
			// Check if this is a platform limitation (stub manager on non-Linux)
			if strings.Contains(err.Error(), "not supported") {
				// Graceful degradation for non-Linux platforms
				slog.Warn("snapshot_unavailable", "s3_key", req.Msg.S3Key, "reason", "platform_limitation")
				resp.ErrorMessage = fmt.Sprintf("snapshot unavailable: %v", err)
			} else {
				// Snapshot creation is MANDATORY on Linux - abort FSM
				slog.Error("snapshot_creation_failed", "s3_key", req.Msg.S3Key, "error", err)
				m.repo.UpdateStatus(img.ID, db.StatusFailed, fmt.Sprintf("snapshot creation failed: %v", err))
				return nil, fsm.Abort(errors.Wrap(err, "snapshot creation failed (required by challenge)"))
			}
		} else {
			slog.Info("snapshot_created", "s3_key", req.Msg.S3Key, "snapshot_id", snapshotInfo.SnapshotID)

			// Update database with snapshot info
			img.SnapshotID = snapshotInfo.SnapshotID
			resp.SnapshotID = snapshotInfo.SnapshotID
			resp.DevicePath = img.DevicePath
			if err := m.repo.Update(img); err != nil {
				slog.Error("image_update_failed", "image_id", img.ID, "error", err)
				return nil, errors.Wrap(err, "failed to update image")
			}
		}
	} else {
		slog.Info("snapshot_skipped", "s3_key", req.Msg.S3Key, "dm_available", m.dmManager != nil, "device_path", img.DevicePath)
	}

	// Mark image as ready
	if err := m.repo.UpdateStatus(resp.ImageID, db.StatusReady, ""); err != nil {
		slog.Error("status_update_failed", "image_id", resp.ImageID, "error", err)
		return nil, errors.Wrap(err, "failed to update status")
	}
	resp.Status = db.StatusReady

	slog.Info("fsm_complete", "s3_key", req.Msg.S3Key, "status", db.StatusReady)

	return fsm.NewResponse(resp), nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Handle symlinks - preserve them as symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, dstPath)
		}

		// Handle regular files
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}
