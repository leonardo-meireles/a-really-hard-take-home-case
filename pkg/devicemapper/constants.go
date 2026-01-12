package devicemapper

// Default configuration values for DeviceMapper thin provisioning.
const (
	// DefaultDataSize is the default data volume size (10GB)
	DefaultDataSize = 10 * 1024 * 1024 * 1024
	// DefaultMetadataSize is the default metadata volume size (100MB)
	DefaultMetadataSize = 100 * 1024 * 1024
	// DefaultSectorSize is the sector size in bytes (512 bytes)
	DefaultSectorSize = 512
	// DefaultDeviceSectors is the default device size in sectors (1GB = 2097152 sectors)
	DefaultDeviceSectors = 2097152
)
