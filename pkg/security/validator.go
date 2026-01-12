package security

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
)

// Validator provides security validation for tar extraction
type Validator struct {
	maxFileSize         int64
	maxTotalSize        int64
	maxCompressionRatio float64

	mu               sync.Mutex
	currentTotalSize int64
}

// NewValidator creates a new security validator
func NewValidator(maxFileSize, maxTotalSize int64, maxCompressionRatio float64) *Validator {
	slog.Info("security_validator_init",
		"max_file_size_mb", maxFileSize/1024/1024,
		"max_total_size_mb", maxTotalSize/1024/1024,
		"max_compression_ratio", maxCompressionRatio)

	return &Validator{
		maxFileSize:         maxFileSize,
		maxTotalSize:        maxTotalSize,
		maxCompressionRatio: maxCompressionRatio,
	}
}

// ValidatePath checks for path traversal attacks
// It validates file paths within a tar archive
func (v *Validator) ValidatePath(tarPath string) error {
	// Reject absolute paths
	if filepath.IsAbs(tarPath) {
		slog.Error("security_path_validation_failed", "path", tarPath, "reason", "absolute_path")
		return fmt.Errorf("security: absolute path not allowed: %s", tarPath)
	}

	// Clean the path
	clean := filepath.Clean(tarPath)

	// Reject paths that start with .. (escape current directory)
	if strings.HasPrefix(clean, "..") {
		slog.Error("security_path_validation_failed", "path", tarPath, "reason", "path_traversal")
		return fmt.Errorf("security: path traversal detected: %s", tarPath)
	}

	return nil
}

// ValidateSymlink validates a symlink target in the context of the symlink's location
// symlinkPath: where the symlink is located (e.g., "/etc/fonts/conf.d/foo")
// targetPath: where the symlink points to (e.g., "../conf.avail/bar")
func (v *Validator) ValidateSymlink(symlinkPath, targetPath string) error {
	// Absolute symlink targets are allowed (container-relative)
	// e.g., symlink /bin/sh -> /usr/bin/dash
	if filepath.IsAbs(targetPath) {
		slog.Info("security_symlink_validated", "symlink", symlinkPath, "target", targetPath, "type", "absolute")
		return nil
	}

	// For relative symlink targets, resolve them in context of the symlink's directory
	// e.g., symlink at /etc/fonts/conf.d/foo -> ../conf.avail/bar
	// resolves to /etc/fonts/conf.avail/bar
	symlinkDir := filepath.Dir(symlinkPath)
	resolvedPath := filepath.Join(symlinkDir, targetPath)
	cleanResolved := filepath.Clean(resolvedPath)

	// Check if the resolved path tries to escape the container root
	// by counting directory depth from root
	parts := strings.Split(cleanResolved, string(filepath.Separator))
	depth := 0

	for _, part := range parts {
		if part == ".." {
			depth--
		} else if part != "" && part != "." {
			depth++
		}
	}

	// Negative depth means it escapes above root
	if depth < 0 {
		slog.Error("security_symlink_validation_failed",
			"symlink", symlinkPath,
			"target", targetPath,
			"resolved", cleanResolved,
			"depth", depth)
		return fmt.Errorf("security: path traversal detected: symlink %s -> %s resolves to %s",
			symlinkPath, targetPath, cleanResolved)
	}

	slog.Info("security_symlink_validated", "symlink", symlinkPath, "target", targetPath, "type", "relative")
	return nil
}

// ValidateFileSize checks if a file exceeds max file size
func (v *Validator) ValidateFileSize(size int64) error {
	if size > v.maxFileSize {
		slog.Error("security_file_size_exceeded",
			"file_size_mb", size/1024/1024,
			"max_file_size_mb", v.maxFileSize/1024/1024)
		return fmt.Errorf("security: file size %d exceeds max %d", size, v.maxFileSize)
	}
	return nil
}

// AddExtractedSize tracks total extracted size and checks against limit
func (v *Validator) AddExtractedSize(size int64) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.currentTotalSize += size

	if v.currentTotalSize > v.maxTotalSize {
		slog.Error("security_total_size_exceeded",
			"current_total_mb", v.currentTotalSize/1024/1024,
			"max_total_mb", v.maxTotalSize/1024/1024,
			"file_size_mb", size/1024/1024)
		return fmt.Errorf("security: total extracted size %d exceeds max %d",
			v.currentTotalSize, v.maxTotalSize)
	}

	return nil
}

// ValidateCompressionRatio checks for compression bombs
func (v *Validator) ValidateCompressionRatio(compressedSize, uncompressedSize int64) error {
	if compressedSize == 0 {
		slog.Error("security_compression_validation_failed", "reason", "zero_compressed_size")
		return fmt.Errorf("security: compressed size cannot be zero")
	}

	ratio := float64(uncompressedSize) / float64(compressedSize)

	if ratio > v.maxCompressionRatio {
		slog.Error("security_compression_bomb_detected",
			"ratio", ratio,
			"max_ratio", v.maxCompressionRatio,
			"compressed_mb", compressedSize/1024/1024,
			"uncompressed_mb", uncompressedSize/1024/1024)
		return fmt.Errorf("security: compression ratio %.2f exceeds max %.2f (compressed: %d, uncompressed: %d)",
			ratio, v.maxCompressionRatio, compressedSize, uncompressedSize)
	}

	slog.Info("security_compression_validated", "ratio", ratio, "compressed_mb", compressedSize/1024/1024, "uncompressed_mb", uncompressedSize/1024/1024)
	return nil
}

// Reset resets the total size counter
func (v *Validator) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.currentTotalSize = 0
}

// GetCurrentTotalSize returns the current total extracted size
func (v *Validator) GetCurrentTotalSize() int64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.currentTotalSize
}
