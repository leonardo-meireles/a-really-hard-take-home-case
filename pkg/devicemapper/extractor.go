package devicemapper

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fly-io/162719/pkg/security"
)

// ExtractTarball extracts a tarball to a directory with security validation
func ExtractTarball(tarPath, destDir string, validator *security.Validator) error {
	validator.Reset()

	f, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("failed to open tar: %w", err)
	}
	defer f.Close()

	tarReader := tar.NewReader(f)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		if err := validator.ValidatePath(header.Name); err != nil {
			return fmt.Errorf("invalid path in tar: %w", err)
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

		case tar.TypeReg:
			if err := validator.ValidateFileSize(header.Size); err != nil {
				return err
			}

			if err := validator.AddExtractedSize(header.Size); err != nil {
				return err
			}

			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent dir: %w", err)
			}

			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			outFile.Close()

		case tar.TypeSymlink:
			// Validate symlink target in context of its location
			// A symlink at /etc/fonts/conf.d/foo pointing to ../conf.avail/bar
			// resolves to /etc/fonts/conf.avail/bar (safe)
			// But a symlink at /foo pointing to ../../../etc/passwd
			// tries to escape the container root (unsafe)
			if err := validator.ValidateSymlink(header.Name, header.Linkname); err != nil {
				return fmt.Errorf("invalid symlink target: %w", err)
			}

			if err := os.Symlink(header.Linkname, target); err != nil && !os.IsExist(err) {
				return fmt.Errorf("failed to create symlink: %w", err)
			}
		}
	}

	fi, err := os.Stat(tarPath)
	if err != nil {
		return fmt.Errorf("failed to stat tar: %w", err)
	}

	if err := validator.ValidateCompressionRatio(fi.Size(), validator.GetCurrentTotalSize()); err != nil {
		return err
	}

	return nil
}
