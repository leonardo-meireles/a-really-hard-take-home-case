package db

// Schema defines the SQLite database schema for container images.
// It creates the images table with indexes for efficient querying,
// and device_sequence for unified device ID allocation.
const Schema = `
CREATE TABLE IF NOT EXISTS images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    s3_key TEXT NOT NULL UNIQUE,
    sha256 TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('pending', 'downloading', 'ready', 'failed')),
    device_path TEXT,
    base_device_id INTEGER,
    snapshot_id INTEGER,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_images_s3_key ON images(s3_key);
CREATE INDEX IF NOT EXISTS idx_images_status ON images(status);
CREATE INDEX IF NOT EXISTS idx_images_created_at ON images(created_at);

CREATE TABLE IF NOT EXISTS device_sequence (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    next_device_id INTEGER NOT NULL DEFAULT 1
);

INSERT OR IGNORE INTO device_sequence (id, next_device_id) VALUES (1, 1);
`

// Status constants
const (
	StatusPending     = "pending"
	StatusDownloading = "downloading"
	StatusReady       = "ready"
	StatusFailed      = "failed"
)

// Image represents a container image record
type Image struct {
	ID           int64
	S3Key        string
	SHA256       string
	Status       string
	DevicePath   string
	BaseDeviceID int
	SnapshotID   int
	ErrorMessage string
	CreatedAt    string
	UpdatedAt    string
}
