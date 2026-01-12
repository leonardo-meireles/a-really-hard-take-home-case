package fsm

// ImageRequest is the FSM input
type ImageRequest struct {
	S3Key    string
	S3Bucket string
}

// ImageResponse is the FSM output (accumulated across transitions)
type ImageResponse struct {
	// From CheckDB
	ImageID int64

	// From Download
	SHA256       string
	DownloadPath string
	DownloadSize int64

	// From Validate (extraction)
	ExtractedPath string

	// From Complete (devicemapper)
	DevicePath string
	SnapshotID int

	// From Complete/Failed
	Status       string
	ErrorMessage string
}

// State names
const (
	StateCheckDB      = "check_db"
	StateDownload     = "download"
	StateValidate     = "validate"
	StateCreateDevice = "create_device"
	StateComplete     = "complete"
	StateFailed       = "failed"
)
