package commands

import (
	"context"
	"log/slog"
	"time"

	"github.com/fly-io/162719/internal/config"
	"github.com/fly-io/162719/pkg/db"
	"github.com/fly-io/162719/pkg/devicemapper"
	"github.com/fly-io/162719/pkg/errors"
	appfsm "github.com/fly-io/162719/pkg/fsm"
	"github.com/fly-io/162719/pkg/security"
	"github.com/fly-io/162719/pkg/storage"
	"github.com/spf13/cobra"
	"github.com/superfly/fsm"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch-and-create <image-key>",
	Short: "Fetch image from S3, scan, and create device",
	Args:  cobra.ExactArgs(1),
	RunE:  runFetch,
}

func init() {
	rootCmd.AddCommand(fetchCmd)
}

func runFetch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	imageKey := args[0]

	cfg, err := config.Load()
	if err != nil {
		return errors.Wrap(err, "config load failed")
	}
	if err := cfg.Validate(); err != nil {
		return errors.Wrap(err, "config invalid")
	}

	// Ensure all necessary directories exist
	if err := ensureDirectories(cfg.SQLitePath, cfg.FSMDBPath, cfg.WorkDir); err != nil {
		return err
	}

	repo, err := db.NewRepository(cfg.SQLitePath)
	if err != nil {
		return errors.Wrap(err, "db init failed")
	}
	defer repo.Close()

	s3Client, err := storage.NewClient(ctx, cfg.S3Bucket, cfg.S3Region)
	if err != nil {
		return errors.Wrap(err, "S3 client failed")
	}

	validator := security.NewValidator(cfg.MaxFileSize, cfg.MaxTotalSize, cfg.MaxCompressionRatio)

	// Initialize devicemapper (stub on non-Linux)
	dmManager, err := devicemapper.NewManager("pool", devicemapper.DefaultDataSize, devicemapper.DefaultMetadataSize)
	if err != nil {
		slog.Warn("devicemapper unavailable", "error", err)
	}
	if dmManager != nil {
		defer dmManager.Close()
	}

	fsmDBPath := cfg.FSMDBPath

	manager, err := fsm.New(fsm.Config{DBPath: fsmDBPath})
	if err != nil {
		return errors.Wrap(err, "FSM manager failed")
	}
	defer manager.Shutdown(10 * time.Second)

	machine := appfsm.NewMachine(repo, s3Client, validator, dmManager, cfg.WorkDir, cfg.FSMMaxRetries)
	start, _, err := machine.Register(ctx, manager)
	if err != nil {
		return errors.Wrap(err, "FSM register failed")
	}

	req := &appfsm.ImageRequest{
		S3Key:    imageKey,
		S3Bucket: cfg.S3Bucket,
	}
	resp := &appfsm.ImageResponse{}

	version, err := start(ctx, imageKey, fsm.NewRequest(req, resp))
	if err != nil {
		return errors.Wrap(err, "FSM start failed")
	}

	slog.Info("fsm started", "version", version)

	if err := manager.Wait(ctx, version); err != nil {
		return errors.Wrap(err, "FSM execution failed")
	}

	slog.Info("fetch completed", "status", resp.Status, "device", resp.DevicePath, "snapshot", resp.SnapshotID)

	return nil
}
