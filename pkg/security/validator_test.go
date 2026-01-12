package security

import (
	"testing"
)

func TestValidatePath_PathTraversal(t *testing.T) {
	v := NewValidator(1024, 1024, 10.0)

	tests := []struct {
		path      string
		shouldErr bool
	}{
		{"file.txt", false},
		{"dir/file.txt", false},
		{"../etc/passwd", true},
		{"/etc/passwd", true},
		{"dir/../file.txt", false},
		{"dir/../../etc/passwd", true},
	}

	for _, tt := range tests {
		err := v.ValidatePath(tt.path)
		if tt.shouldErr && err == nil {
			t.Errorf("expected error for path: %s", tt.path)
		}
		if !tt.shouldErr && err != nil {
			t.Errorf("unexpected error for path %s: %v", tt.path, err)
		}
	}
}

func TestValidateFileSize(t *testing.T) {
	v := NewValidator(100, 1000, 10.0)

	if err := v.ValidateFileSize(50); err != nil {
		t.Errorf("expected no error for size 50, got: %v", err)
	}

	if err := v.ValidateFileSize(150); err == nil {
		t.Error("expected error for size 150 exceeding limit 100")
	}
}

func TestValidateCompressionRatio(t *testing.T) {
	v := NewValidator(1024, 10240, 10.0)

	if err := v.ValidateCompressionRatio(10, 100); err != nil {
		t.Errorf("expected no error for ratio 10.0, got: %v", err)
	}

	if err := v.ValidateCompressionRatio(50, 1000); err == nil {
		t.Error("expected error for ratio 20.0 exceeding limit 10.0")
	}
}

func TestAddExtractedSize_ExceedsTotal(t *testing.T) {
	v := NewValidator(1024, 500, 10.0)

	if err := v.AddExtractedSize(400); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := v.AddExtractedSize(200); err == nil {
		t.Error("expected error when total extracted exceeds limit")
	}
}
