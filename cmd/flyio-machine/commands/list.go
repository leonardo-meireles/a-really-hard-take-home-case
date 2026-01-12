package commands

import (
	"fmt"

	"github.com/fly-io/162719/internal/config"
	"github.com/fly-io/162719/pkg/db"
	"github.com/fly-io/162719/pkg/errors"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all images and their status",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.Wrap(err, "config load failed")
	}

	// Ensure database directory exists
	if err := ensureDirectories(cfg.SQLitePath, "", ""); err != nil {
		return err
	}

	repo, err := db.NewRepository(cfg.SQLitePath)
	if err != nil {
		return errors.Wrap(err, "db init failed")
	}
	defer repo.Close()

	images, err := repo.List()
	if err != nil {
		return errors.Wrap(err, "list failed")
	}

	if len(images) == 0 {
		fmt.Println("No images found")
		return nil
	}

	fmt.Printf("%-40s %-12s %-30s %-20s\n", "S3 KEY", "STATUS", "DEVICE", "SNAPSHOT")
	fmt.Println("------------------------------------------------------------------------------------------------")

	for _, img := range images {
		devicePath := img.DevicePath
		if devicePath == "" {
			devicePath = "-"
		}
		snapshotID := img.SnapshotID
		snapshotStr := "-"
		if snapshotID != 0 {
			snapshotStr = fmt.Sprintf("%d", snapshotID)
		}

		fmt.Printf("%-40s %-12s %-30s %-20s\n",
			img.S3Key, img.Status, devicePath, snapshotStr)
	}

	return nil
}
