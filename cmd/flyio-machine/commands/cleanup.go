package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fly-io/162719/internal/config"
	"github.com/fly-io/162719/pkg/db"
	"github.com/fly-io/162719/pkg/devicemapper"
	"github.com/fly-io/162719/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	cleanupAll      bool
	cleanupImage    string
	cleanupOrphaned bool
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up image resources (extracted files, devices, snapshots)",
	Long: `Clean up resources associated with images:
  --all              Clean all resources for all images
  --image <s3-key>   Clean resources for specific image
  --orphaned         Clean orphaned resources not tracked in database`,
	RunE: runCleanup,
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
	cleanupCmd.Flags().BoolVar(&cleanupAll, "all", false, "Clean all resources")
	cleanupCmd.Flags().StringVar(&cleanupImage, "image", "", "Clean specific image by S3 key")
	cleanupCmd.Flags().BoolVar(&cleanupOrphaned, "orphaned", false, "Clean orphaned resources")
}

func runCleanup(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.Wrap(err, "config load failed")
	}

	repo, err := db.NewRepository(cfg.SQLitePath)
	if err != nil {
		return errors.Wrap(err, "db init failed")
	}
	defer repo.Close()

	// Initialize devicemapper manager (may be stub on non-Linux)
	dmManager, err := devicemapper.NewManager("pool", devicemapper.DefaultDataSize, devicemapper.DefaultMetadataSize)
	if err != nil {
		fmt.Printf("⚠️  Devicemapper unavailable: %v\n", err)
		dmManager = nil
	}

	ctx := context.Background()

	if cleanupAll {
		return cleanupAllImages(ctx, repo, dmManager, cfg)
	} else if cleanupImage != "" {
		return cleanupSpecificImage(ctx, repo, dmManager, cfg, cleanupImage)
	} else if cleanupOrphaned {
		return cleanupOrphanedResources(ctx, repo, dmManager, cfg)
	} else {
		return fmt.Errorf("must specify --all, --image, or --orphaned")
	}
}

func cleanupAllImages(ctx context.Context, repo *db.Repository, dmManager devicemapper.Manager, cfg *config.Config) error {
	images, err := repo.List()
	if err != nil {
		return errors.Wrap(err, "list failed")
	}

	fmt.Printf("🧹 Cleaning up %d images...\n", len(images))

	for _, img := range images {
		if err := cleanupImageResources(ctx, repo, dmManager, cfg, img); err != nil {
			fmt.Printf("⚠️  Failed to clean %s: %v\n", img.S3Key, err)
		} else {
			fmt.Printf("✅ Cleaned: %s\n", img.S3Key)
		}
	}

	return nil
}

func cleanupSpecificImage(ctx context.Context, repo *db.Repository, dmManager devicemapper.Manager, cfg *config.Config, s3Key string) error {
	img, err := repo.GetByS3Key(s3Key)
	if err != nil {
		return errors.Wrap(err, "image not found")
	}

	fmt.Printf("🧹 Cleaning up %s...\n", s3Key)

	if err := cleanupImageResources(ctx, repo, dmManager, cfg, img); err != nil {
		return errors.Wrap(err, "cleanup failed")
	}

	fmt.Printf("✅ Cleaned: %s\n", s3Key)
	return nil
}

func cleanupImageResources(ctx context.Context, repo *db.Repository, dmManager devicemapper.Manager, cfg *config.Config, img *db.Image) error {
	// 1. Unmount and delete snapshot if exists
	if dmManager != nil && img.SnapshotID != 0 {
		snapshotName := fmt.Sprintf("flyio-snapshot-%d", img.SnapshotID)
		snapshotPath := filepath.Join("/dev/mapper", snapshotName)

		// Try to unmount if mounted
		if _, err := os.Stat(snapshotPath); err == nil {
			// Snapshot device exists, try to remove it
			if err := dmManager.DeleteDevice(ctx, fmt.Sprintf("%d", img.SnapshotID)); err != nil {
				fmt.Printf("⚠️  Snapshot cleanup warning: %v\n", err)
			}
		}
		img.SnapshotID = 0
	}

	// 2. Unmount and delete base device if exists
	if dmManager != nil && img.BaseDeviceID > 0 {
		deviceID := fmt.Sprintf("%d", img.BaseDeviceID)
		devicePath := filepath.Join("/dev/mapper", fmt.Sprintf("flyio-%s", deviceID))

		// Try to unmount if mounted
		if _, err := os.Stat(devicePath); err == nil {
			if err := dmManager.DeleteDevice(ctx, deviceID); err != nil {
				fmt.Printf("⚠️  Device cleanup warning: %v\n", err)
			}
		}
		img.BaseDeviceID = 0
		img.DevicePath = ""
	}

	// 3. Remove extracted filesystem
	extractedPath := filepath.Join(cfg.WorkDir, "extracted", img.S3Key)
	if _, err := os.Stat(extractedPath); err == nil {
		if err := os.RemoveAll(extractedPath); err != nil {
			return errors.Wrap(err, "failed to remove extracted files")
		}
	}

	// 4. Remove downloaded tarball
	downloadPath := filepath.Join(cfg.WorkDir, "downloads", img.S3Key)
	if _, err := os.Stat(downloadPath); err == nil {
		if err := os.Remove(downloadPath); err != nil {
			return errors.Wrap(err, "failed to remove download")
		}
	}

	// 5. Update database status
	img.Status = "cleaned"
	if err := repo.Update(img); err != nil {
		return errors.Wrap(err, "failed to update database")
	}

	return nil
}

func cleanupOrphanedResources(ctx context.Context, repo *db.Repository, dmManager devicemapper.Manager, cfg *config.Config) error {
	fmt.Println("🔍 Scanning for orphaned resources...")

	orphanCount := 0

	// 1. Check for orphaned extracted directories
	extractedDir := filepath.Join(cfg.WorkDir, "extracted")
	if entries, err := os.ReadDir(extractedDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			// Check if this image exists in database
			_, err := repo.GetByS3Key(entry.Name())
			if err != nil {
				// Orphaned - remove it
				orphanPath := filepath.Join(extractedDir, entry.Name())
				if err := os.RemoveAll(orphanPath); err != nil {
					fmt.Printf("⚠️  Failed to remove orphaned directory %s: %v\n", entry.Name(), err)
				} else {
					fmt.Printf("🗑️  Removed orphaned directory: %s\n", entry.Name())
					orphanCount++
				}
			}
		}
	}

	// 2. Check for orphaned downloads
	downloadDir := filepath.Join(cfg.WorkDir, "downloads")
	if entries, err := os.ReadDir(downloadDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			// Check if this image exists in database
			_, err := repo.GetByS3Key(entry.Name())
			if err != nil {
				// Orphaned - remove it
				orphanPath := filepath.Join(downloadDir, entry.Name())
				if err := os.Remove(orphanPath); err != nil {
					fmt.Printf("⚠️  Failed to remove orphaned download %s: %v\n", entry.Name(), err)
				} else {
					fmt.Printf("🗑️  Removed orphaned download: %s\n", entry.Name())
					orphanCount++
				}
			}
		}
	}

	fmt.Printf("✅ Removed %d orphaned resources\n", orphanCount)
	return nil
}
