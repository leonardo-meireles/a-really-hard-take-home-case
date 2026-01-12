package commands

import (
	"os"
	"path/filepath"

	"github.com/fly-io/162719/pkg/errors"
)

// ensureDirectories creates all necessary directories for the application
func ensureDirectories(sqlitePath, fsmDBPath, workDir string) error {
	// Create database directory
	if err := os.MkdirAll(filepath.Dir(sqlitePath), 0755); err != nil {
		return errors.Wrap(err, "failed to create database directory")
	}

	// Create FSM database directory (only needed for fetch command)
	if fsmDBPath != "" {
		if err := os.MkdirAll(fsmDBPath, 0755); err != nil {
			return errors.Wrap(err, "failed to create FSM directory")
		}
	}

	// Create work directory (only needed for fetch command)
	if workDir != "" {
		if err := os.MkdirAll(workDir, 0755); err != nil {
			return errors.Wrap(err, "failed to create work directory")
		}
	}

	return nil
}