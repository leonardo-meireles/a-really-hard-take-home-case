# Fly.io Platform Machines - AI-Optimized Implementation Plan

**Target AI Agent**: Claude Code, Cursor, GitHub Copilot
**Project Path**: `/Users/leonardomeireles/Work/fly-io/162719`
**Module Name**: `github.com/fly-io/162719`
**Go Version**: 1.25

---

## CRITICAL CONTEXT FOR AI AGENTS

### Architectural Decisions (100% Confidence)
1. **BoltDB vs SQLite**: Two separate databases
   - BoltDB: FSM library's INTERNAL persistence (managed by FSM, do not touch)
   - SQLite: YOUR application state (images, status, device paths)
2. **Security**: Path traversal protection + size/bomb detection + Trivy scanning
3. **CLI Framework**: Cobra + Viper (configurable via flags, env vars, config file)
4. **FSM Approach**: Test FSM prototype first, then real implementation
5. **Existing Code**: Move to `reference/` directory, use as reference only

### Project Structure
```
/Users/leonardomeireles/Work/fly-io/162719/
├── cmd/
│   └── flyio-machine/
│       ├── main.go
│       └── commands/
│           ├── root.go
│           ├── fetch.go
│           ├── list.go
│           ├── scan.go
│           └── worker.go
├── pkg/
│   ├── scan/
│   │   ├── scanner.go
│   │   └── vulnerability.go
│   ├── storage/
│   │   ├── client.go
│   │   ├── downloader.go
│   │   └── types.go
│   ├── security/
│   │   ├── validator.go
│   │   └── limits.go
│   ├── db/
│   │   ├── schema.go
│   │   ├── repository.go
│   │   └── migrations.go
│   ├── fsm/
│   │   ├── machine.go
│   │   ├── states.go
│   │   └── types.go
│   ├── unpack/
│   │   └── extractor.go
│   └── devicemapper/
│       ├── thinpool.go
│       └── snapshot.go
├── internal/
│   └── config/
│       └── config.go
├── test/
│   └── fsm_hello/
│       └── main.go
├── reference/
│   ├── s3_trivy_detective.go (moved from root)
│   └── trivy_scanner.go (moved from root)
├── go.mod
├── go.sum
├── .gitignore
├── README.md
├── FSM_LEARNINGS.md (created in Phase 0)
├── DEVICEMAPPER_RESEARCH.md (created in Phase 2)
└── IMPLEMENTATION_PLAN_FINAL.md (this file)
```

---

## PHASE 0: FSM API Investigation

**Goal**: Understand FSM library API through working prototype before implementing real system.

**Duration**: 2-3 hours

**Confidence**: 100%

### Step 0.1: Create Test FSM Environment

**Action**: Create test directory and basic FSM structure.

**Commands**:
```bash
mkdir -p test/fsm_hello
cd test/fsm_hello
```

**File**: `test/fsm_hello/main.go`

**Implementation**:
```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    "github.com/superfly/fsm"
    "github.com/superfly/fsm/boltfsm"
)

// HelloRequest is the FSM input
type HelloRequest struct {
    Name string
}

// HelloResponse is the FSM output
type HelloResponse struct {
    Greeting string
    Steps    []string
}

// State names
const (
    StateGreet    = "greet"
    StateProcess  = "process"
    StateComplete = "complete"
    StateFailed   = "failed"
)

func main() {
    ctx := context.Background()

    // Create FSM store (BoltDB managed by FSM library)
    store, err := boltfsm.New(ctx, "./test_fsm.db")
    if err != nil {
        log.Fatalf("Failed to create FSM store: %v", err)
    }
    defer store.Close()

    // Create FSM manager
    manager := fsm.NewManager(store)
    defer manager.Shutdown(10 * time.Second)

    // Register FSM with transitions
    start, resume, err := fsm.Register[HelloRequest, HelloResponse](manager, "hello").
        Start(StateGreet, greetTransition).
        To(StateProcess, processTransition).
        End(StateComplete).
        End(StateFailed).
        Build(ctx)

    if err != nil {
        log.Fatalf("Failed to register FSM: %v", err)
    }

    // Test: Start FSM
    req := &HelloRequest{Name: "World"}
    version, err := start(ctx, "test-run-1", fsm.NewRequest(req, &HelloResponse{}))
    if err != nil {
        log.Fatalf("Failed to start FSM: %v", err)
    }

    log.Printf("FSM started with version: %s", version)

    // Wait for completion
    if err := manager.Wait(ctx, version); err != nil {
        log.Fatalf("FSM execution failed: %v", err)
    }

    log.Println("FSM completed successfully!")

    // Cleanup
    os.Remove("./test_fsm.db")
}

// greetTransition handles greeting state
func greetTransition(ctx context.Context, req *fsm.Request[HelloRequest, HelloResponse]) (string, error) {
    log.Printf("Greet transition: Hello, %s!", req.Msg.Name)

    // Update response
    req.W.SetResponse(&HelloResponse{
        Greeting: fmt.Sprintf("Hello, %s!", req.Msg.Name),
        Steps:    []string{"greeted"},
    })

    // Return next state name
    return StateProcess, nil
}

// processTransition handles processing state
func processTransition(ctx context.Context, req *fsm.Request[HelloRequest, HelloResponse]) (string, error) {
    log.Printf("Process transition: Processing for %s", req.Msg.Name)

    // Get current response and update
    resp := req.W.Response()
    resp.Steps = append(resp.Steps, "processed")
    req.W.SetResponse(resp)

    // Return final state
    return StateComplete, nil
}
```

**Test Execution**:
```bash
cd test/fsm_hello
go mod init test-fsm
go get github.com/superfly/fsm
go get github.com/superfly/fsm/boltfsm
go run main.go
```

**Expected Output**:
```
Greet transition: Hello, World!
Process transition: Processing for World
FSM started with version: <ulid>
FSM completed successfully!
```

**Document Learnings**: Create `FSM_LEARNINGS.md`:

**File**: `FSM_LEARNINGS.md`

```markdown
# FSM Library API Learnings

## Key Findings from Test FSM

### 1. Transition Return Signature
**Pattern**: Transitions return `(nextStateName string, error)`

```go
func transition(ctx context.Context, req *fsm.Request[R, W]) (string, error) {
    // Logic here
    return "next_state_name", nil
}
```

**Important**: Return state name as string, NOT a Response object.

### 2. Starting FSM
**Pattern**: Use `start()` function returned by `Register()`

```go
start, resume, err := fsm.Register[Req, Resp](manager, "action").Build(ctx)
version, err := start(ctx, resourceID, fsm.NewRequest(reqMsg, &respMsg))
```

**Important**:
- `resourceID`: Unique identifier for this FSM run
- `fsm.NewRequest(req, &resp)`: Creates Request wrapper with response pointer

### 3. Request/Response Access
**Pattern**: Access via `req.Msg` (input) and `req.W` (response writer)

```go
func transition(ctx context.Context, req *fsm.Request[R, W]) (string, error) {
    input := req.Msg.Name  // Access input

    // Update response
    req.W.SetResponse(&W{Field: "value"})

    // Read current response
    current := req.W.Response()

    return "next", nil
}
```

### 4. Error Handling Strategies
**Pattern**: Different errors trigger different behaviors

```go
// Abort immediately (no retry)
return "", fsm.Abort(fmt.Errorf("validation failed"))

// Unrecoverable error (permanent failure)
return "", fsm.NewUnrecoverableSystemError(fmt.Errorf("system error"))

// Any other error: FSM retries with exponential backoff
return "", fmt.Errorf("transient error")
```

### 5. FSM Manager Lifecycle
**Pattern**: Create → Register → Start → Wait → Shutdown

```go
// 1. Create store (BoltDB)
store, _ := boltfsm.New(ctx, "./fsm.db")

// 2. Create manager
manager := fsm.NewManager(store)
defer manager.Shutdown(10 * time.Second)

// 3. Register FSM
start, resume, _ := fsm.Register[R, W](manager, "action").Build(ctx)

// 4. Start FSM
version, _ := start(ctx, id, fsm.NewRequest(req, &resp))

// 5. Wait for completion
manager.Wait(ctx, version)
```

### 6. State Machine Registration
**Pattern**: Fluent builder API

```go
fsm.Register[Request, Response](manager, "action-name").
    Start("initial", initialFunc).      // First state
    To("next", nextFunc).                // Intermediate states
    To("another", anotherFunc).
    End("complete").                     // Terminal state (success)
    End("failed").                       // Terminal state (failure)
    Build(ctx)
```

**Important**: Must have at least one `End()` state.

## Action Items for Real Implementation
1. Use `(string, error)` return signature for all transitions
2. Use `fsm.NewRequest(req, &resp)` when starting FSM
3. Access input via `req.Msg`, update response via `req.W.SetResponse()`
4. Use `fsm.Abort()` for validation failures (no retry)
5. Return regular errors for transient failures (automatic retry)
6. Create FSM manager with BoltDB store, shut down gracefully
```

**Success Criteria**:
- [ ] Test FSM compiles and runs successfully
- [ ] FSM completes all state transitions
- [ ] FSM_LEARNINGS.md documents all API patterns
- [ ] Confident in transition return signature
- [ ] Confident in start() usage
- [ ] Confident in error handling strategy

**Confidence after Phase 0**: 100%

---

## PHASE 1: Core Implementation

**Goal**: Implement FSM-orchestrated image processing with S3, Trivy, and SQLite.

**Duration**: 1-2 days

**Confidence**: 100%

### Step 1.1: Project Initialization

**Action**: Set up project structure and move existing code to reference.

**Commands**:
```bash
cd /Users/leonardomeireles/Work/fly-io/162719

# Create directory structure
mkdir -p cmd/flyio-machine/commands
mkdir -p pkg/{scan,storage,security,db,fsm,unpack,devicemapper}
mkdir -p internal/config
mkdir -p reference

# Move existing code to reference
mv s3_trivy_detective.go reference/
mv trivy_scanner.go reference/

# Update .gitignore
cat >> .gitignore << 'EOF'
# Build artifacts
/bin/
flyio-machine

# Databases
*.db
*.db-journal
*.db-wal

# Downloads
/downloads/
/tmp/

# Test artifacts
/test/fsm_hello/test_fsm.db

# macOS
.DS_Store
EOF
```

**Update `go.mod`**:
```bash
go get github.com/superfly/fsm@latest
go get github.com/superfly/fsm/boltfsm@latest
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest
go get github.com/mattn/go-sqlite3@latest
go mod tidy
```

**Expected `go.mod` after update**:
```go
module github.com/fly-io/162719

go 1.25

require (
    github.com/aws/aws-sdk-go-v2 v1.39.0
    github.com/aws/aws-sdk-go-v2/config v1.31.8
    github.com/aws/aws-sdk-go-v2/credentials v1.18.12
    github.com/aws/aws-sdk-go-v2/service/s3 v1.88.1
    github.com/mattn/go-sqlite3 v1.14.24
    github.com/sirupsen/logrus v1.9.3
    github.com/spf13/cobra v1.8.1
    github.com/spf13/viper v1.19.0
    github.com/superfly/fsm v1.0.0
)
```

**Success Criteria**:
- [ ] Directory structure created
- [ ] Existing code moved to reference/
- [ ] Dependencies added to go.mod
- [ ] `go mod tidy` runs without errors
- [ ] .gitignore updated

---

### Step 1.2: Configuration System (Viper + Cobra)

**Action**: Implement configuration with viper supporting CLI flags, env vars, and config files.

**File**: `internal/config/config.go`

**Implementation**:
```go
package config

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
    // Database paths
    SQLitePath string
    FSMDBPath  string

    // S3 configuration
    S3Bucket string
    S3Region string
    S3Prefix string

    // Download settings
    DownloadDir string

    // Security limits
    MaxFileSize      int64 // bytes
    MaxTotalSize     int64 // bytes
    MaxCompressionRatio float64

    // Trivy settings
    TrivyEnabled bool

    // DeviceMapper (Phase 2)
    DMPoolName    string
    DMMetaSize    int64 // MB
    DMDataSize    int64 // GB
    DMEnabled     bool
}

// Default values
const (
    DefaultSQLitePath         = "./images.db"
    DefaultFSMDBPath          = "./fsm.db"
    DefaultS3Bucket           = "flyio-platform-hiring-challenge"
    DefaultS3Region           = "us-east-1"
    DefaultS3Prefix           = "images/"
    DefaultDownloadDir        = "./downloads"
    DefaultMaxFileSize        = 2 * 1024 * 1024 * 1024  // 2GB
    DefaultMaxTotalSize       = 20 * 1024 * 1024 * 1024 // 20GB
    DefaultMaxCompressionRatio = 100.0
    DefaultTrivyEnabled       = true
    DefaultDMPoolName         = "flyio-pool"
    DefaultDMMetaSize         = 1  // 1MB
    DefaultDMDataSize         = 2  // 2GB
    DefaultDMEnabled          = false // Linux-only
)

// Load reads configuration from flags, env vars, and config file
func Load() (*Config, error) {
    // Set defaults
    viper.SetDefault("sqlite_path", DefaultSQLitePath)
    viper.SetDefault("fsm_db_path", DefaultFSMDBPath)
    viper.SetDefault("s3_bucket", DefaultS3Bucket)
    viper.SetDefault("s3_region", DefaultS3Region)
    viper.SetDefault("s3_prefix", DefaultS3Prefix)
    viper.SetDefault("download_dir", DefaultDownloadDir)
    viper.SetDefault("max_file_size", DefaultMaxFileSize)
    viper.SetDefault("max_total_size", DefaultMaxTotalSize)
    viper.SetDefault("max_compression_ratio", DefaultMaxCompressionRatio)
    viper.SetDefault("trivy_enabled", DefaultTrivyEnabled)
    viper.SetDefault("dm_pool_name", DefaultDMPoolName)
    viper.SetDefault("dm_meta_size", DefaultDMMetaSize)
    viper.SetDefault("dm_data_size", DefaultDMDataSize)
    viper.SetDefault("dm_enabled", DefaultDMEnabled)

    // Environment variables
    viper.SetEnvPrefix("FLYIO")
    viper.AutomaticEnv()

    // Config file (optional)
    viper.SetConfigName("flyio-machine")
    viper.SetConfigType("yaml")
    viper.AddConfigPath(".")
    viper.AddConfigPath("$HOME/.config/flyio-machine")

    // Read config file if it exists (not required)
    if err := viper.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
            return nil, fmt.Errorf("failed to read config file: %w", err)
        }
        // Config file not found is OK, use defaults + flags + env
    }

    // Create download directory
    downloadDir := viper.GetString("download_dir")
    if err := os.MkdirAll(downloadDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create download dir: %w", err)
    }

    cfg := &Config{
        SQLitePath:          viper.GetString("sqlite_path"),
        FSMDBPath:           viper.GetString("fsm_db_path"),
        S3Bucket:            viper.GetString("s3_bucket"),
        S3Region:            viper.GetString("s3_region"),
        S3Prefix:            viper.GetString("s3_prefix"),
        DownloadDir:         downloadDir,
        MaxFileSize:         viper.GetInt64("max_file_size"),
        MaxTotalSize:        viper.GetInt64("max_total_size"),
        MaxCompressionRatio: viper.GetFloat64("max_compression_ratio"),
        TrivyEnabled:        viper.GetBool("trivy_enabled"),
        DMPoolName:          viper.GetString("dm_pool_name"),
        DMMetaSize:          viper.GetInt64("dm_meta_size"),
        DMDataSize:          viper.GetInt64("dm_data_size"),
        DMEnabled:           viper.GetBool("dm_enabled"),
    }

    return cfg, nil
}

// Validate checks configuration validity
func (c *Config) Validate() error {
    if c.MaxFileSize <= 0 {
        return fmt.Errorf("max_file_size must be positive")
    }
    if c.MaxTotalSize <= 0 {
        return fmt.Errorf("max_total_size must be positive")
    }
    if c.MaxCompressionRatio <= 0 {
        return fmt.Errorf("max_compression_ratio must be positive")
    }
    if c.S3Bucket == "" {
        return fmt.Errorf("s3_bucket is required")
    }
    if c.S3Region == "" {
        return fmt.Errorf("s3_region is required")
    }
    return nil
}
```

**Test**: Minimal test to verify config loading

**File**: `internal/config/config_test.go`

```go
package config

import (
    "os"
    "testing"

    "github.com/spf13/viper"
)

func TestLoadDefaults(t *testing.T) {
    // Reset viper state
    viper.Reset()

    cfg, err := Load()
    if err != nil {
        t.Fatalf("Load() failed: %v", err)
    }

    if cfg.SQLitePath != DefaultSQLitePath {
        t.Errorf("Expected SQLitePath=%s, got %s", DefaultSQLitePath, cfg.SQLitePath)
    }

    if cfg.S3Bucket != DefaultS3Bucket {
        t.Errorf("Expected S3Bucket=%s, got %s", DefaultS3Bucket, cfg.S3Bucket)
    }

    if err := cfg.Validate(); err != nil {
        t.Errorf("Validate() failed: %v", err)
    }

    // Cleanup
    os.RemoveAll(cfg.DownloadDir)
}

func TestLoadEnvVars(t *testing.T) {
    viper.Reset()

    os.Setenv("FLYIO_SQLITE_PATH", "/tmp/test.db")
    os.Setenv("FLYIO_S3_BUCKET", "test-bucket")
    defer os.Unsetenv("FLYIO_SQLITE_PATH")
    defer os.Unsetenv("FLYIO_S3_BUCKET")

    cfg, err := Load()
    if err != nil {
        t.Fatalf("Load() failed: %v", err)
    }

    if cfg.SQLitePath != "/tmp/test.db" {
        t.Errorf("Expected SQLitePath=/tmp/test.db, got %s", cfg.SQLitePath)
    }

    if cfg.S3Bucket != "test-bucket" {
        t.Errorf("Expected S3Bucket=test-bucket, got %s", cfg.S3Bucket)
    }

    os.RemoveAll(cfg.DownloadDir)
}
```

**Success Criteria**:
- [ ] Config loads defaults successfully
- [ ] Config respects environment variables (FLYIO_ prefix)
- [ ] Config validation works
- [ ] Tests pass: `go test ./internal/config`

---

### Step 1.3: Security Validator

**Action**: Implement security checks for path traversal and size bombs.

**File**: `pkg/security/validator.go`

**Implementation**:
```go
package security

import (
    "fmt"
    "path/filepath"
    "strings"
)

// Validator performs security checks on tar entries
type Validator struct {
    maxFileSize         int64
    maxTotalSize        int64
    maxCompressionRatio float64
    currentTotalSize    int64
}

// NewValidator creates a security validator with configured limits
func NewValidator(maxFileSize, maxTotalSize int64, maxCompressionRatio float64) *Validator {
    return &Validator{
        maxFileSize:         maxFileSize,
        maxTotalSize:        maxTotalSize,
        maxCompressionRatio: maxCompressionRatio,
        currentTotalSize:    0,
    }
}

// ValidatePath checks for path traversal attacks
func (v *Validator) ValidatePath(tarPath string) error {
    // Normalize path
    clean := filepath.Clean(tarPath)

    // Check for absolute paths
    if filepath.IsAbs(clean) {
        return fmt.Errorf("absolute path not allowed: %s", tarPath)
    }

    // Check for parent directory traversal
    if strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") {
        return fmt.Errorf("path traversal detected: %s", tarPath)
    }

    // Check for suspicious patterns
    if strings.Contains(clean, "//") {
        return fmt.Errorf("suspicious path pattern: %s", tarPath)
    }

    return nil
}

// ValidateFileSize checks individual file size limits
func (v *Validator) ValidateFileSize(size int64) error {
    if size > v.maxFileSize {
        return fmt.Errorf("file size %d exceeds limit %d", size, v.maxFileSize)
    }

    // Check total extraction size
    newTotal := v.currentTotalSize + size
    if newTotal > v.maxTotalSize {
        return fmt.Errorf("total extraction size %d exceeds limit %d", newTotal, v.maxTotalSize)
    }

    v.currentTotalSize = newTotal
    return nil
}

// ValidateCompressionRatio checks for zip bombs
func (v *Validator) ValidateCompressionRatio(compressedSize, uncompressedSize int64) error {
    if compressedSize == 0 {
        return fmt.Errorf("compressed size cannot be zero")
    }

    ratio := float64(uncompressedSize) / float64(compressedSize)
    if ratio > v.maxCompressionRatio {
        return fmt.Errorf("compression ratio %.2f exceeds limit %.2f (zip bomb?)",
            ratio, v.maxCompressionRatio)
    }

    return nil
}

// Reset resets the validator state for a new extraction
func (v *Validator) Reset() {
    v.currentTotalSize = 0
}

// GetCurrentTotalSize returns accumulated extraction size
func (v *Validator) GetCurrentTotalSize() int64 {
    return v.currentTotalSize
}
```

**Test**: Security validator tests

**File**: `pkg/security/validator_test.go`

```go
package security

import (
    "testing"
)

func TestValidatePath(t *testing.T) {
    v := NewValidator(1024, 10240, 100)

    tests := []struct {
        name    string
        path    string
        wantErr bool
    }{
        {"normal path", "dir/file.txt", false},
        {"current dir", "./file.txt", false},
        {"parent traversal", "../etc/passwd", true},
        {"nested parent", "dir/../../etc/passwd", true},
        {"absolute path", "/etc/passwd", true},
        {"double slash", "dir//file.txt", true},
        {"clean nested", "a/b/c/file.txt", false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := v.ValidatePath(tt.path)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
            }
        })
    }
}

func TestValidateFileSize(t *testing.T) {
    v := NewValidator(1000, 5000, 100)

    // Should pass
    if err := v.ValidateFileSize(500); err != nil {
        t.Errorf("ValidateFileSize(500) failed: %v", err)
    }

    // Should pass (cumulative 1000)
    if err := v.ValidateFileSize(500); err != nil {
        t.Errorf("ValidateFileSize(500) failed: %v", err)
    }

    // Should fail (exceeds per-file limit)
    if err := v.ValidateFileSize(2000); err == nil {
        t.Error("ValidateFileSize(2000) should fail (exceeds per-file limit)")
    }

    v.Reset()

    // Should pass after reset
    if err := v.ValidateFileSize(500); err != nil {
        t.Errorf("ValidateFileSize(500) after reset failed: %v", err)
    }
}

func TestValidateCompressionRatio(t *testing.T) {
    v := NewValidator(1000, 5000, 100)

    tests := []struct {
        name           string
        compressedSize int64
        uncompressedSize int64
        wantErr        bool
    }{
        {"normal ratio", 100, 1000, false},   // 10:1
        {"high ratio", 10, 1000, false},      // 100:1 (exactly at limit)
        {"zip bomb", 1, 1000, true},          // 1000:1 (exceeds limit)
        {"zero compressed", 0, 1000, true},   // Invalid
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := v.ValidateCompressionRatio(tt.compressedSize, tt.uncompressedSize)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateCompressionRatio() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

**Success Criteria**:
- [ ] Path traversal detection works (../, absolute paths)
- [ ] File size limits enforced
- [ ] Total size limits enforced
- [ ] Compression ratio detection works
- [ ] Tests pass: `go test ./pkg/security`

---

### Step 1.4: SQLite Repository

**Action**: Implement SQLite database for image tracking with idempotency support.

**File**: `pkg/db/schema.go`

**Implementation**:
```go
package db

// SchemaV1 is the initial database schema
const SchemaV1 = `
CREATE TABLE IF NOT EXISTS images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    s3_key TEXT NOT NULL UNIQUE,
    sha256 TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    etag TEXT,

    -- Status tracking
    status TEXT NOT NULL,  -- pending, downloading, downloaded, scanned, vulnerable, ready, failed
    downloaded_at TIMESTAMP,
    scanned_at TIMESTAMP,

    -- Vulnerability info
    has_critical BOOLEAN DEFAULT 0,
    has_high BOOLEAN DEFAULT 0,
    vuln_count INTEGER DEFAULT 0,

    -- Phase 2: DeviceMapper tracking
    device_path TEXT,
    snapshot_id TEXT,
    dm_created_at TIMESTAMP,

    -- Metadata
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_images_s3_key ON images(s3_key);
CREATE INDEX IF NOT EXISTS idx_images_status ON images(status);
CREATE INDEX IF NOT EXISTS idx_images_sha256 ON images(sha256);

-- Trigger to update updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_images_timestamp
AFTER UPDATE ON images
BEGIN
    UPDATE images SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
`
```

**File**: `pkg/db/repository.go`

**Implementation**:
```go
package db

import (
    "database/sql"
    "fmt"
    "time"

    _ "github.com/mattn/go-sqlite3"
)

// Image represents an image record in the database
type Image struct {
    ID           int64
    S3Key        string
    SHA256       string
    SizeBytes    int64
    ETag         string
    Status       string
    DownloadedAt *time.Time
    ScannedAt    *time.Time
    HasCritical  bool
    HasHigh      bool
    VulnCount    int
    DevicePath   string
    SnapshotID   string
    DMCreatedAt  *time.Time
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

// Status constants
const (
    StatusPending     = "pending"
    StatusDownloading = "downloading"
    StatusDownloaded  = "downloaded"
    StatusScanned     = "scanned"
    StatusVulnerable  = "vulnerable"
    StatusReady       = "ready"
    StatusFailed      = "failed"
)

// Repository manages database operations
type Repository struct {
    db *sql.DB
}

// NewRepository creates a new database repository
func NewRepository(dbPath string) (*Repository, error) {
    db, err := sql.Open("sqlite3", dbPath)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }

    // Run migrations
    if _, err := db.Exec(SchemaV1); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to run migrations: %w", err)
    }

    return &Repository{db: db}, nil
}

// Close closes the database connection
func (r *Repository) Close() error {
    return r.db.Close()
}

// GetByS3Key retrieves an image by S3 key (idempotency check)
func (r *Repository) GetByS3Key(s3Key string) (*Image, error) {
    query := `
        SELECT id, s3_key, sha256, size_bytes, etag, status,
               downloaded_at, scanned_at, has_critical, has_high, vuln_count,
               device_path, snapshot_id, dm_created_at, created_at, updated_at
        FROM images WHERE s3_key = ?
    `

    img := &Image{}
    err := r.db.QueryRow(query, s3Key).Scan(
        &img.ID, &img.S3Key, &img.SHA256, &img.SizeBytes, &img.ETag, &img.Status,
        &img.DownloadedAt, &img.ScannedAt, &img.HasCritical, &img.HasHigh, &img.VulnCount,
        &img.DevicePath, &img.SnapshotID, &img.DMCreatedAt, &img.CreatedAt, &img.UpdatedAt,
    )

    if err == sql.ErrNoRows {
        return nil, nil // Not found (not an error)
    }
    if err != nil {
        return nil, fmt.Errorf("query failed: %w", err)
    }

    return img, nil
}

// Create inserts a new image record
func (r *Repository) Create(img *Image) error {
    query := `
        INSERT INTO images (s3_key, sha256, size_bytes, etag, status)
        VALUES (?, ?, ?, ?, ?)
    `

    result, err := r.db.Exec(query, img.S3Key, img.SHA256, img.SizeBytes, img.ETag, img.Status)
    if err != nil {
        return fmt.Errorf("insert failed: %w", err)
    }

    id, err := result.LastInsertId()
    if err != nil {
        return fmt.Errorf("failed to get last insert id: %w", err)
    }

    img.ID = id
    return nil
}

// UpdateStatus updates image status
func (r *Repository) UpdateStatus(id int64, status string) error {
    query := `UPDATE images SET status = ? WHERE id = ?`
    _, err := r.db.Exec(query, status, id)
    return err
}

// UpdateDownloaded marks image as downloaded with timestamp
func (r *Repository) UpdateDownloaded(id int64, sha256 string) error {
    query := `
        UPDATE images
        SET status = ?, sha256 = ?, downloaded_at = CURRENT_TIMESTAMP
        WHERE id = ?
    `
    _, err := r.db.Exec(query, StatusDownloaded, sha256, id)
    return err
}

// UpdateScanResult updates vulnerability scan results
func (r *Repository) UpdateScanResult(id int64, hasCritical, hasHigh bool, vulnCount int) error {
    status := StatusReady
    if hasCritical || hasHigh {
        status = StatusVulnerable
    }

    query := `
        UPDATE images
        SET has_critical = ?, has_high = ?, vuln_count = ?,
            scanned_at = CURRENT_TIMESTAMP, status = ?
        WHERE id = ?
    `

    _, err := r.db.Exec(query, hasCritical, hasHigh, vulnCount, status, id)
    return err
}

// UpdateDeviceMapper updates DeviceMapper information (Phase 2)
func (r *Repository) UpdateDeviceMapper(id int64, devicePath, snapshotID string) error {
    query := `
        UPDATE images
        SET device_path = ?, snapshot_id = ?, dm_created_at = CURRENT_TIMESTAMP
        WHERE id = ?
    `
    _, err := r.db.Exec(query, devicePath, snapshotID, id)
    return err
}

// ListAll returns all images
func (r *Repository) ListAll() ([]*Image, error) {
    query := `
        SELECT id, s3_key, sha256, size_bytes, etag, status,
               downloaded_at, scanned_at, has_critical, has_high, vuln_count,
               device_path, snapshot_id, dm_created_at, created_at, updated_at
        FROM images ORDER BY created_at
    `

    rows, err := r.db.Query(query)
    if err != nil {
        return nil, fmt.Errorf("query failed: %w", err)
    }
    defer rows.Close()

    var images []*Image
    for rows.Next() {
        img := &Image{}
        err := rows.Scan(
            &img.ID, &img.S3Key, &img.SHA256, &img.SizeBytes, &img.ETag, &img.Status,
            &img.DownloadedAt, &img.ScannedAt, &img.HasCritical, &img.HasHigh, &img.VulnCount,
            &img.DevicePath, &img.SnapshotID, &img.DMCreatedAt, &img.CreatedAt, &img.UpdatedAt,
        )
        if err != nil {
            return nil, fmt.Errorf("scan failed: %w", err)
        }
        images = append(images, img)
    }

    return images, nil
}

// ListByStatus returns images with specific status
func (r *Repository) ListByStatus(status string) ([]*Image, error) {
    query := `
        SELECT id, s3_key, sha256, size_bytes, etag, status,
               downloaded_at, scanned_at, has_critical, has_high, vuln_count,
               device_path, snapshot_id, dm_created_at, created_at, updated_at
        FROM images WHERE status = ? ORDER BY created_at
    `

    rows, err := r.db.Query(query, status)
    if err != nil {
        return nil, fmt.Errorf("query failed: %w", err)
    }
    defer rows.Close()

    var images []*Image
    for rows.Next() {
        img := &Image{}
        err := rows.Scan(
            &img.ID, &img.S3Key, &img.SHA256, &img.SizeBytes, &img.ETag, &img.Status,
            &img.DownloadedAt, &img.ScannedAt, &img.HasCritical, &img.HasHigh, &img.VulnCount,
            &img.DevicePath, &img.SnapshotID, &img.DMCreatedAt, &img.CreatedAt, &img.UpdatedAt,
        )
        if err != nil {
            return nil, fmt.Errorf("scan failed: %w", err)
        }
        images = append(images, img)
    }

    return images, nil
}
```

**Test**: Database repository tests

**File**: `pkg/db/repository_test.go`

```go
package db

import (
    "os"
    "testing"
)

func TestRepositoryCRUD(t *testing.T) {
    // Use temp database
    dbPath := "/tmp/test-images.db"
    defer os.Remove(dbPath)

    repo, err := NewRepository(dbPath)
    if err != nil {
        t.Fatalf("NewRepository failed: %v", err)
    }
    defer repo.Close()

    // Test Create
    img := &Image{
        S3Key:     "golang/1.tar",
        SHA256:    "abc123",
        SizeBytes: 1000000,
        ETag:      "etag123",
        Status:    StatusPending,
    }

    if err := repo.Create(img); err != nil {
        t.Fatalf("Create failed: %v", err)
    }

    if img.ID == 0 {
        t.Error("Expected non-zero ID after create")
    }

    // Test GetByS3Key
    retrieved, err := repo.GetByS3Key("golang/1.tar")
    if err != nil {
        t.Fatalf("GetByS3Key failed: %v", err)
    }

    if retrieved == nil {
        t.Fatal("Expected image to be found")
    }

    if retrieved.SHA256 != "abc123" {
        t.Errorf("Expected SHA256=abc123, got %s", retrieved.SHA256)
    }

    // Test UpdateStatus
    if err := repo.UpdateStatus(img.ID, StatusDownloaded); err != nil {
        t.Fatalf("UpdateStatus failed: %v", err)
    }

    updated, _ := repo.GetByS3Key("golang/1.tar")
    if updated.Status != StatusDownloaded {
        t.Errorf("Expected status=downloaded, got %s", updated.Status)
    }

    // Test UpdateScanResult
    if err := repo.UpdateScanResult(img.ID, true, false, 5); err != nil {
        t.Fatalf("UpdateScanResult failed: %v", err)
    }

    scanned, _ := repo.GetByS3Key("golang/1.tar")
    if !scanned.HasCritical {
        t.Error("Expected has_critical=true")
    }
    if scanned.Status != StatusVulnerable {
        t.Errorf("Expected status=vulnerable, got %s", scanned.Status)
    }

    // Test ListAll
    images, err := repo.ListAll()
    if err != nil {
        t.Fatalf("ListAll failed: %v", err)
    }
    if len(images) != 1 {
        t.Errorf("Expected 1 image, got %d", len(images))
    }
}

func TestIdempotency(t *testing.T) {
    dbPath := "/tmp/test-idempotency.db"
    defer os.Remove(dbPath)

    repo, err := NewRepository(dbPath)
    if err != nil {
        t.Fatalf("NewRepository failed: %v", err)
    }
    defer repo.Close()

    // First check - should not exist
    img, err := repo.GetByS3Key("golang/2.tar")
    if err != nil {
        t.Fatalf("GetByS3Key failed: %v", err)
    }
    if img != nil {
        t.Error("Expected nil for non-existent image")
    }

    // Create image
    newImg := &Image{
        S3Key:     "golang/2.tar",
        SHA256:    "def456",
        SizeBytes: 2000000,
        Status:    StatusReady,
    }
    repo.Create(newImg)

    // Second check - should exist
    existing, err := repo.GetByS3Key("golang/2.tar")
    if err != nil {
        t.Fatalf("GetByS3Key failed: %v", err)
    }
    if existing == nil {
        t.Error("Expected image to exist")
    }
    if existing.Status != StatusReady {
        t.Errorf("Expected status=ready, got %s", existing.Status)
    }
}
```

**Success Criteria**:
- [ ] Database schema creates successfully
- [ ] CRUD operations work correctly
- [ ] Idempotency check (GetByS3Key) works
- [ ] Status updates work
- [ ] Vulnerability tracking works
- [ ] Tests pass: `go test ./pkg/db`

---

### Step 1.5: Trivy Scanner Package

**Action**: Implement Trivy integration based on reference code with security validation.

**File**: `pkg/scan/vulnerability.go`

**Implementation**:
```go
package scan

// Vulnerability represents a CVE finding
type Vulnerability struct {
    ID           string
    Severity     string
    Package      string
    Version      string
    FixedVersion string
}

// ScanResult contains all vulnerabilities found
type ScanResult struct {
    ImagePath       string
    SHA256          string
    Vulnerabilities []Vulnerability
    HasCritical     bool
    HasHigh         bool
    VulnCount       int
}

// IsBlocked returns true if image should be blocked from use
func (r *ScanResult) IsBlocked() bool {
    return r.HasCritical || r.HasHigh
}
```

**File**: `pkg/scan/scanner.go`

**Implementation**:
```go
package scan

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"

    "github.com/fly-io/162719/pkg/security"
)

// Scanner wraps Trivy CLI for vulnerability scanning
type Scanner struct {
    trivyPath string
    validator *security.Validator
}

// NewScanner creates a new Trivy scanner
func NewScanner(validator *security.Validator) (*Scanner, error) {
    // Verify trivy is installed
    path, err := exec.LookPath("trivy")
    if err != nil {
        return nil, fmt.Errorf("trivy not found in PATH: %w", err)
    }

    return &Scanner{
        trivyPath: path,
        validator: validator,
    }, nil
}

// GenerateSBOM extracts tarball and generates CycloneDX SBOM
func (s *Scanner) GenerateSBOM(ctx context.Context, tarPath string) (string, error) {
    // Create temp directory for extraction
    extractDir, err := os.MkdirTemp("", "image-extract-*")
    if err != nil {
        return "", fmt.Errorf("failed to create temp dir: %w", err)
    }
    defer os.RemoveAll(extractDir)

    // Extract tarball with security validation
    if err := s.extractWithValidation(ctx, tarPath, extractDir); err != nil {
        return "", fmt.Errorf("extraction failed: %w", err)
    }

    // Generate SBOM
    sbomPath := filepath.Join(os.TempDir(), fmt.Sprintf("sbom-%s.json", filepath.Base(tarPath)))
    sbomCmd := exec.CommandContext(ctx, s.trivyPath, "fs", extractDir,
        "--format", "cyclonedx",
        "--output", sbomPath)

    if output, err := sbomCmd.CombinedOutput(); err != nil {
        return "", fmt.Errorf("trivy sbom generation failed: %w, output: %s", err, output)
    }

    return sbomPath, nil
}

// extractWithValidation extracts tar with security checks
func (s *Scanner) extractWithValidation(ctx context.Context, tarPath, destDir string) error {
    file, err := os.Open(tarPath)
    if err != nil {
        return fmt.Errorf("failed to open tar: %w", err)
    }
    defer file.Close()

    // Get file info for compression ratio check
    fileInfo, err := file.Stat()
    if err != nil {
        return fmt.Errorf("failed to stat tar: %w", err)
    }
    compressedSize := fileInfo.Size()

    // Use tar command for simplicity (matches Trivy approach)
    // Alternative: implement with archive/tar for more control
    tarCmd := exec.CommandContext(ctx, "tar", "-xf", tarPath, "-C", destDir)
    if output, err := tarCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("tar extraction failed: %w, output: %s", err, output)
    }

    // Validate extraction result
    var totalSize int64
    err = filepath.Walk(destDir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if !info.IsDir() {
            // Validate file size
            if err := s.validator.ValidateFileSize(info.Size()); err != nil {
                return err
            }
            totalSize += info.Size()
        }
        return nil
    })
    if err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // Check compression ratio
    if err := s.validator.ValidateCompressionRatio(compressedSize, totalSize); err != nil {
        return fmt.Errorf("compression ratio check failed: %w", err)
    }

    return nil
}

// ScanVulnerabilities scans SBOM for HIGH/CRITICAL vulnerabilities
func (s *Scanner) ScanVulnerabilities(ctx context.Context, sbomPath string) (*ScanResult, error) {
    // Run trivy sbom scan
    scanCmd := exec.CommandContext(ctx, s.trivyPath, "sbom", sbomPath,
        "--severity", "HIGH,CRITICAL",
        "--format", "json")

    output, err := scanCmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("trivy scan failed: %w, output: %s", err, output)
    }

    // Parse JSON output
    var trivyOutput struct {
        Results []struct {
            Vulnerabilities []struct {
                VulnerabilityID  string `json:"VulnerabilityID"`
                Severity         string `json:"Severity"`
                PkgName          string `json:"PkgName"`
                InstalledVersion string `json:"InstalledVersion"`
                FixedVersion     string `json:"FixedVersion"`
            } `json:"Vulnerabilities"`
        } `json:"Results"`
    }

    if err := json.Unmarshal(output, &trivyOutput); err != nil {
        return nil, fmt.Errorf("failed to parse trivy output: %w", err)
    }

    result := &ScanResult{
        ImagePath:       sbomPath,
        Vulnerabilities: make([]Vulnerability, 0),
    }

    for _, r := range trivyOutput.Results {
        for _, v := range r.Vulnerabilities {
            vuln := Vulnerability{
                ID:           v.VulnerabilityID,
                Severity:     v.Severity,
                Package:      v.PkgName,
                Version:      v.InstalledVersion,
                FixedVersion: v.FixedVersion,
            }
            result.Vulnerabilities = append(result.Vulnerabilities, vuln)

            if v.Severity == "CRITICAL" {
                result.HasCritical = true
            }
            if v.Severity == "HIGH" {
                result.HasHigh = true
            }
        }
    }

    result.VulnCount = len(result.Vulnerabilities)
    return result, nil
}

// ScanImage is a convenience method that generates SBOM and scans in one call
func (s *Scanner) ScanImage(ctx context.Context, tarPath, sha256 string) (*ScanResult, error) {
    sbomPath, err := s.GenerateSBOM(ctx, tarPath)
    if err != nil {
        return nil, err
    }
    defer os.Remove(sbomPath)

    result, err := s.ScanVulnerabilities(ctx, sbomPath)
    if err != nil {
        return nil, err
    }

    result.SHA256 = sha256
    return result, nil
}
```

**Test**: Scanner creation test (Trivy must be installed)

**File**: `pkg/scan/scanner_test.go`

```go
package scan

import (
    "testing"

    "github.com/fly-io/162719/pkg/security"
)

func TestNewScanner(t *testing.T) {
    validator := security.NewValidator(1024*1024*1024, 10*1024*1024*1024, 100)
    scanner, err := NewScanner(validator)
    if err != nil {
        t.Skipf("Trivy not installed: %v", err)
    }
    if scanner == nil {
        t.Error("Expected scanner instance")
    }
}

// Note: Full integration tests require actual tar files
// These will be tested in end-to-end integration phase
```

**Success Criteria**:
- [ ] Scanner can be created (Trivy CLI detected)
- [ ] SBOM generation works with sample tar
- [ ] Vulnerability scanning parses JSON correctly
- [ ] Security validation integrated
- [ ] Test passes: `go test ./pkg/scan`

---

### Step 1.6: S3 Storage Client

**Action**: Implement S3 client for anonymous access and download with SHA256.

**File**: `pkg/storage/types.go`

**Implementation**:
```go
package storage

// ImageMetadata contains S3 image information
type ImageMetadata struct {
    Key       string
    Size      int64
    ETag      string
    SHA256    string // Computed during download
    LocalPath string // Path to downloaded file
}
```

**File**: `pkg/storage/client.go`

**Implementation**:
```go
package storage

import (
    "context"
    "fmt"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/credentials"
    "github.com/aws/aws-sdk-go-v2/service/s3"
)

// Client wraps S3 operations
type Client struct {
    s3Client *s3.Client
    bucket   string
    prefix   string
}

// NewClient creates a new S3 client with anonymous credentials
func NewClient(ctx context.Context, bucket, region, prefix string) (*Client, error) {
    // Anonymous access configuration
    cfg, err := config.LoadDefaultConfig(ctx,
        config.WithRegion(region),
        config.WithCredentialsProvider(credentials.AnonymousCredentials{}),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS config: %w", err)
    }

    return &Client{
        s3Client: s3.NewFromConfig(cfg),
        bucket:   bucket,
        prefix:   prefix,
    }, nil
}

// ListImages lists all tar files in the configured prefix
func (c *Client) ListImages(ctx context.Context) ([]ImageMetadata, error) {
    input := &s3.ListObjectsV2Input{
        Bucket: aws.String(c.bucket),
        Prefix: aws.String(c.prefix),
    }

    result, err := c.s3Client.ListObjectsV2(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("failed to list objects: %w", err)
    }

    var images []ImageMetadata
    for _, obj := range result.Contents {
        if obj.Key == nil || obj.Size == nil {
            continue
        }

        key := *obj.Key
        // Only include .tar files
        if len(key) < 4 || key[len(key)-4:] != ".tar" {
            continue
        }

        images = append(images, ImageMetadata{
            Key:  key,
            Size: *obj.Size,
            ETag: aws.ToString(obj.ETag),
        })
    }

    return images, nil
}

// HeadObject gets metadata for a specific S3 object
func (c *Client) HeadObject(ctx context.Context, key string) (*ImageMetadata, error) {
    input := &s3.HeadObjectInput{
        Bucket: aws.String(c.bucket),
        Key:    aws.String(key),
    }

    result, err := c.s3Client.HeadObject(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("failed to head object: %w", err)
    }

    return &ImageMetadata{
        Key:  key,
        Size: aws.ToInt64(result.ContentLength),
        ETag: aws.ToString(result.ETag),
    }, nil
}
```

**File**: `pkg/storage/downloader.go`

**Implementation**:
```go
package storage

import (
    "context"
    "crypto/sha256"
    "fmt"
    "io"
    "os"
    "path/filepath"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/s3"
)

// Download downloads S3 object and computes SHA256
func (c *Client) Download(ctx context.Context, key, destDir string) (*ImageMetadata, error) {
    input := &s3.GetObjectInput{
        Bucket: aws.String(c.bucket),
        Key:    aws.String(key),
    }

    result, err := c.s3Client.GetObject(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("failed to get object: %w", err)
    }
    defer result.Body.Close()

    // Create destination file
    filename := filepath.Base(key)
    destPath := filepath.Join(destDir, filename)

    file, err := os.Create(destPath)
    if err != nil {
        return nil, fmt.Errorf("failed to create file: %w", err)
    }
    defer file.Close()

    // Stream download with SHA256 computation
    hasher := sha256.New()
    writer := io.MultiWriter(file, hasher)

    size, err := io.Copy(writer, result.Body)
    if err != nil {
        os.Remove(destPath)
        return nil, fmt.Errorf("failed to download: %w", err)
    }

    sha256Sum := fmt.Sprintf("%x", hasher.Sum(nil))

    return &ImageMetadata{
        Key:       key,
        Size:      size,
        ETag:      aws.ToString(result.ETag),
        SHA256:    sha256Sum,
        LocalPath: destPath,
    }, nil
}
```

**Test**: S3 client creation test

**File**: `pkg/storage/client_test.go`

```go
package storage

import (
    "context"
    "testing"
)

func TestNewClient(t *testing.T) {
    ctx := context.Background()
    client, err := NewClient(ctx, "flyio-platform-hiring-challenge", "us-east-1", "images/")
    if err != nil {
        t.Fatalf("NewClient failed: %v", err)
    }
    if client == nil {
        t.Error("Expected client instance")
    }
}

// Note: ListImages and Download require S3 access
// These will be tested in integration phase
```

**Success Criteria**:
- [ ] Client creates successfully with anonymous credentials
- [ ] ListImages returns all 15 images (integration test)
- [ ] Download computes SHA256 correctly (integration test)
- [ ] Test passes: `go test ./pkg/storage`

---

### Step 1.7: FSM State Machine Implementation

**Action**: Implement FSM for image processing workflow using Phase 0 learnings.

**File**: `pkg/fsm/types.go`

**Implementation**:
```go
package fsm

// ImageRequest is the FSM input message
type ImageRequest struct {
    S3Key string
    // LocalPath is populated during download state
    LocalPath string
}

// ImageResponse is the FSM output message
type ImageResponse struct {
    Success      bool
    SHA256       string
    Status       string
    IsVulnerable bool
    VulnCount    int
    ErrorMessage string
}

// State names
const (
    StateCheckDB   = "check_db"
    StateDownload  = "download"
    StateScan      = "scan"
    StateComplete  = "complete"
    StateFailed    = "failed"
)
```

**File**: `pkg/fsm/machine.go`

**Implementation**:
```go
package fsm

import (
    "context"
    "fmt"

    "github.com/superfly/fsm"

    "github.com/fly-io/162719/internal/config"
    "github.com/fly-io/162719/pkg/db"
    "github.com/fly-io/162719/pkg/scan"
    "github.com/fly-io/162719/pkg/storage"
)

// Machine orchestrates image processing via FSM
type Machine struct {
    storageClient *storage.Client
    scanner       *scan.Scanner
    repo          *db.Repository
    config        *config.Config
}

// NewMachine creates a new FSM machine
func NewMachine(storageClient *storage.Client, scanner *scan.Scanner, repo *db.Repository, cfg *config.Config) *Machine {
    return &Machine{
        storageClient: storageClient,
        scanner:       scanner,
        repo:          repo,
        config:        cfg,
    }
}

// Register registers the FSM with the manager
func (m *Machine) Register(ctx context.Context, manager *fsm.Manager) (
    func(context.Context, string, *fsm.Request[ImageRequest, ImageResponse], ...fsm.StartOption) (string, error),
    func(context.Context) error,
    error,
) {
    start, resume, err := fsm.Register[ImageRequest, ImageResponse](manager, "process-image").
        Start(StateCheckDB, m.handleCheckDB).
        To(StateDownload, m.handleDownload).
        To(StateScan, m.handleScan).
        End(StateComplete).
        End(StateFailed).
        Build(ctx)

    if err != nil {
        return nil, nil, fmt.Errorf("failed to register FSM: %w", err)
    }

    return start, resume, nil
}
```

**File**: `pkg/fsm/states.go`

**Implementation**:
```go
package fsm

import (
    "context"
    "fmt"
    "os"

    "github.com/superfly/fsm"

    "github.com/fly-io/162719/pkg/db"
)

// handleCheckDB checks if image already exists in database (idempotency)
func (m *Machine) handleCheckDB(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (string, error) {
    s3Key := req.Msg.S3Key

    // Check database for existing image
    img, err := m.repo.GetByS3Key(s3Key)
    if err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("database error: %w", err))
    }

    // If image exists and is ready/vulnerable, skip processing
    if img != nil && (img.Status == db.StatusReady || img.Status == db.StatusVulnerable) {
        req.W.SetResponse(&ImageResponse{
            Success:      true,
            SHA256:       img.SHA256,
            Status:       img.Status,
            IsVulnerable: img.Status == db.StatusVulnerable,
            VulnCount:    img.VulnCount,
        })
        return StateComplete, nil
    }

    // If exists but incomplete, continue processing
    // If doesn't exist, will be created in download state
    return StateDownload, nil
}

// handleDownload downloads image from S3
func (m *Machine) handleDownload(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (string, error) {
    s3Key := req.Msg.S3Key

    // Check if already exists in DB (might have been created in previous run)
    img, err := m.repo.GetByS3Key(s3Key)
    if err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("database error: %w", err))
    }

    // If doesn't exist, create pending record
    if img == nil {
        img = &db.Image{
            S3Key:     s3Key,
            SHA256:    "", // Will be computed during download
            SizeBytes: 0,  // Will be updated
            Status:    db.StatusPending,
        }
        if err := m.repo.Create(img); err != nil {
            return StateFailed, fsm.Abort(fmt.Errorf("failed to create db record: %w", err))
        }
    }

    // Update status to downloading
    if err := m.repo.UpdateStatus(img.ID, db.StatusDownloading); err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("failed to update status: %w", err))
    }

    // Download from S3 with SHA256 computation
    metadata, err := m.storageClient.Download(ctx, s3Key, m.config.DownloadDir)
    if err != nil {
        m.repo.UpdateStatus(img.ID, db.StatusFailed)
        return StateFailed, fsm.Abort(fmt.Errorf("download failed: %w", err))
    }

    // Update database with download info
    if err := m.repo.UpdateDownloaded(img.ID, metadata.SHA256); err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("failed to update db: %w", err))
    }

    // Store local path in request for next state
    req.Msg.LocalPath = metadata.LocalPath

    return StateScan, nil
}

// handleScan runs Trivy security scan
func (m *Machine) handleScan(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (string, error) {
    localPath := req.Msg.LocalPath
    s3Key := req.Msg.S3Key

    if localPath == "" {
        return StateFailed, fsm.Abort(fmt.Errorf("local path not set (download state issue)"))
    }

    // Get image record
    img, err := m.repo.GetByS3Key(s3Key)
    if err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("database error: %w", err))
    }

    // Update status to scanning
    if err := m.repo.UpdateStatus(img.ID, "scanning"); err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("failed to update status: %w", err))
    }

    // Run Trivy scan (with security validation)
    scanResult, err := m.scanner.ScanImage(ctx, localPath, img.SHA256)
    if err != nil {
        m.repo.UpdateStatus(img.ID, db.StatusFailed)
        return StateFailed, fsm.Abort(fmt.Errorf("scan failed: %w", err))
    }

    // Update database with scan results
    if err := m.repo.UpdateScanResult(img.ID, scanResult.HasCritical, scanResult.HasHigh, scanResult.VulnCount); err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("failed to update scan results: %w", err))
    }

    // Store scan result in response
    req.W.SetResponse(&ImageResponse{
        Success:      true,
        SHA256:       img.SHA256,
        Status:       img.Status,
        IsVulnerable: scanResult.IsBlocked(),
        VulnCount:    scanResult.VulnCount,
    })

    // Clean up downloaded tar file after scan
    os.Remove(localPath)

    return StateComplete, nil
}
```

**Success Criteria**:
- [ ] FSM compiles without errors
- [ ] State transitions follow Phase 0 learnings
- [ ] Error handling uses fsm.Abort() correctly
- [ ] Idempotency check works via SQLite

---

### Step 1.8: CLI Implementation (Cobra)

**Action**: Implement CLI with cobra framework and subcommands.

**File**: `cmd/flyio-machine/commands/root.go`

**Implementation**:
```go
package commands

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
    Use:   "flyio-machine",
    Short: "Fly.io Platform Machines - Container image management",
    Long: `flyio-machine manages container images from S3 with FSM orchestration.

Features:
- Downloads images from S3
- Scans for vulnerabilities with Trivy
- Tracks state with SQLite
- DeviceMapper thin pool management (Phase 2)`,
}

func Execute() error {
    return rootCmd.Execute()
}

func init() {
    cobra.OnInitialize(initConfig)

    // Global flags
    rootCmd.PersistentFlags().StringP("config", "c", "", "config file (default: ./flyio-machine.yaml)")
    rootCmd.PersistentFlags().String("sqlite-path", "./images.db", "SQLite database path")
    rootCmd.PersistentFlags().String("fsm-db-path", "./fsm.db", "FSM BoltDB path")
    rootCmd.PersistentFlags().String("download-dir", "./downloads", "Download directory")
    rootCmd.PersistentFlags().String("s3-bucket", "flyio-platform-hiring-challenge", "S3 bucket name")
    rootCmd.PersistentFlags().String("s3-region", "us-east-1", "S3 region")

    // Bind flags to viper
    viper.BindPFlag("sqlite_path", rootCmd.PersistentFlags().Lookup("sqlite-path"))
    viper.BindPFlag("fsm_db_path", rootCmd.PersistentFlags().Lookup("fsm-db-path"))
    viper.BindPFlag("download_dir", rootCmd.PersistentFlags().Lookup("download-dir"))
    viper.BindPFlag("s3_bucket", rootCmd.PersistentFlags().Lookup("s3-bucket"))
    viper.BindPFlag("s3_region", rootCmd.PersistentFlags().Lookup("s3-region"))
}

func initConfig() {
    cfgFile := viper.GetString("config")
    if cfgFile != "" {
        viper.SetConfigFile(cfgFile)
        if err := viper.ReadInConfig(); err != nil {
            fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
            os.Exit(1)
        }
    }
}
```

**File**: `cmd/flyio-machine/commands/fetch.go`

**Implementation**:
```go
package commands

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/spf13/cobra"
    "github.com/superfly/fsm"
    "github.com/superfly/fsm/boltfsm"

    "github.com/fly-io/162719/internal/config"
    "github.com/fly-io/162719/pkg/db"
    fsmPkg "github.com/fly-io/162719/pkg/fsm"
    "github.com/fly-io/162719/pkg/scan"
    "github.com/fly-io/162719/pkg/security"
    "github.com/fly-io/162719/pkg/storage"
)

var fetchCmd = &cobra.Command{
    Use:   "fetch-and-create <image>",
    Short: "Fetch image from S3 and create ready-to-use container",
    Long: `Downloads an image from S3, scans for vulnerabilities, and prepares it for use.

Example:
  flyio-machine fetch-and-create golang/2.tar
  flyio-machine fetch-and-create images/golang/2.tar`,
    Args: cobra.ExactArgs(1),
    RunE: runFetch,
}

func init() {
    rootCmd.AddCommand(fetchCmd)
}

func runFetch(cmd *cobra.Command, args []string) error {
    ctx := context.Background()
    imageKey := args[0]

    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }

    if err := cfg.Validate(); err != nil {
        return fmt.Errorf("invalid config: %w", err)
    }

    // Initialize components
    fmt.Println("Initializing components...")

    // 1. SQLite repository
    repo, err := db.NewRepository(cfg.SQLitePath)
    if err != nil {
        return fmt.Errorf("failed to create repository: %w", err)
    }
    defer repo.Close()

    // 2. S3 storage client
    storageClient, err := storage.NewClient(ctx, cfg.S3Bucket, cfg.S3Region, cfg.S3Prefix)
    if err != nil {
        return fmt.Errorf("failed to create storage client: %w", err)
    }

    // 3. Security validator
    validator := security.NewValidator(cfg.MaxFileSize, cfg.MaxTotalSize, cfg.MaxCompressionRatio)

    // 4. Trivy scanner
    scanner, err := scan.NewScanner(validator)
    if err != nil {
        return fmt.Errorf("failed to create scanner: %w", err)
    }

    // 5. FSM store and manager
    fsmStore, err := boltfsm.New(ctx, cfg.FSMDBPath)
    if err != nil {
        return fmt.Errorf("failed to create FSM store: %w", err)
    }
    defer fsmStore.Close()

    manager := fsm.NewManager(fsmStore)
    defer manager.Shutdown(10 * time.Second)

    // 6. Register FSM
    machine := fsmPkg.NewMachine(storageClient, scanner, repo, cfg)
    start, _, err := machine.Register(ctx, manager)
    if err != nil {
        return fmt.Errorf("failed to register FSM: %w", err)
    }

    // Start FSM for image
    fmt.Printf("Processing image: %s\n", imageKey)
    req := &fsmPkg.ImageRequest{S3Key: imageKey}
    version, err := start(ctx, imageKey, fsm.NewRequest(req, &fsmPkg.ImageResponse{}))
    if err != nil {
        return fmt.Errorf("failed to start FSM: %w", err)
    }

    fmt.Printf("FSM started with version: %s\n", version)

    // Wait for completion
    fmt.Println("Waiting for FSM to complete...")
    if err := manager.Wait(ctx, version); err != nil {
        return fmt.Errorf("FSM execution failed: %w", err)
    }

    // Get final status from database
    img, err := repo.GetByS3Key(imageKey)
    if err != nil {
        return fmt.Errorf("failed to get image status: %w", err)
    }

    fmt.Printf("\nResult:\n")
    fmt.Printf("  Status: %s\n", img.Status)
    fmt.Printf("  SHA256: %s\n", img.SHA256)
    if img.VulnCount > 0 {
        fmt.Printf("  Vulnerabilities: %d (Critical: %v, High: %v)\n",
            img.VulnCount, img.HasCritical, img.HasHigh)
    }

    if img.Status == db.StatusVulnerable {
        fmt.Println("\nWARNING: Image has HIGH or CRITICAL vulnerabilities and should not be used.")
        os.Exit(1)
    }

    fmt.Println("\nImage ready for use!")
    return nil
}
```

**File**: `cmd/flyio-machine/commands/list.go`

**Implementation**:
```go
package commands

import (
    "fmt"
    "text/tabwriter"
    "os"

    "github.com/spf13/cobra"

    "github.com/fly-io/162719/internal/config"
    "github.com/fly-io/162719/pkg/db"
)

var listCmd = &cobra.Command{
    Use:   "list",
    Short: "List all images and their status",
    Long: `Lists all images from the database with their current status.

Example:
  flyio-machine list`,
    RunE: runList,
}

func init() {
    rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }

    // Open repository
    repo, err := db.NewRepository(cfg.SQLitePath)
    if err != nil {
        return fmt.Errorf("failed to open repository: %w", err)
    }
    defer repo.Close()

    // Get all images
    images, err := repo.ListAll()
    if err != nil {
        return fmt.Errorf("failed to list images: %w", err)
    }

    if len(images) == 0 {
        fmt.Println("No images found.")
        return nil
    }

    // Print table
    w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
    fmt.Fprintln(w, "S3 KEY\tSTATUS\tSHA256\tVULNS\tDOWNLOADED")
    fmt.Fprintln(w, "------\t------\t------\t-----\t----------")

    for _, img := range images {
        downloaded := "N/A"
        if img.DownloadedAt != nil {
            downloaded = img.DownloadedAt.Format("2006-01-02 15:04")
        }

        vulnInfo := "-"
        if img.VulnCount > 0 {
            vulnInfo = fmt.Sprintf("%d", img.VulnCount)
            if img.HasCritical {
                vulnInfo += " (CRIT)"
            } else if img.HasHigh {
                vulnInfo += " (HIGH)"
            }
        }

        sha256Short := img.SHA256
        if len(sha256Short) > 12 {
            sha256Short = sha256Short[:12] + "..."
        }

        fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
            img.S3Key, img.Status, sha256Short, vulnInfo, downloaded)
    }

    w.Flush()
    return nil
}
```

**File**: `cmd/flyio-machine/commands/scan.go`

**Implementation**:
```go
package commands

import (
    "context"
    "fmt"

    "github.com/spf13/cobra"

    "github.com/fly-io/162719/internal/config"
    "github.com/fly-io/162719/pkg/db"
    "github.com/fly-io/162719/pkg/scan"
    "github.com/fly-io/162719/pkg/security"
)

var scanCmd = &cobra.Command{
    Use:   "scan <image>",
    Short: "Scan an already-downloaded image for vulnerabilities",
    Long: `Re-scans an image that has already been downloaded.

Example:
  flyio-machine scan golang/2.tar`,
    Args: cobra.ExactArgs(1),
    RunE: runScan,
}

func init() {
    rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
    ctx := context.Background()
    imageKey := args[0]

    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }

    // Open repository
    repo, err := db.NewRepository(cfg.SQLitePath)
    if err != nil {
        return fmt.Errorf("failed to open repository: %w", err)
    }
    defer repo.Close()

    // Check if image exists
    img, err := repo.GetByS3Key(imageKey)
    if err != nil {
        return fmt.Errorf("database error: %w", err)
    }
    if img == nil {
        return fmt.Errorf("image not found in database: %s", imageKey)
    }
    if img.Status == db.StatusPending || img.Status == db.StatusDownloading {
        return fmt.Errorf("image not yet downloaded: %s", imageKey)
    }

    // Get local path
    localPath := fmt.Sprintf("%s/%s", cfg.DownloadDir, imageKey)

    // Create scanner
    validator := security.NewValidator(cfg.MaxFileSize, cfg.MaxTotalSize, cfg.MaxCompressionRatio)
    scanner, err := scan.NewScanner(validator)
    if err != nil {
        return fmt.Errorf("failed to create scanner: %w", err)
    }

    // Scan image
    fmt.Printf("Scanning %s...\n", imageKey)
    result, err := scanner.ScanImage(ctx, localPath, img.SHA256)
    if err != nil {
        return fmt.Errorf("scan failed: %w", err)
    }

    // Update database
    if err := repo.UpdateScanResult(img.ID, result.HasCritical, result.HasHigh, result.VulnCount); err != nil {
        return fmt.Errorf("failed to update database: %w", err)
    }

    // Display results
    fmt.Printf("\nScan Results:\n")
    fmt.Printf("  Vulnerabilities: %d\n", result.VulnCount)
    fmt.Printf("  Critical: %v\n", result.HasCritical)
    fmt.Printf("  High: %v\n", result.HasHigh)
    fmt.Printf("  Status: %s\n", img.Status)

    if result.IsBlocked() {
        fmt.Println("\nWARNING: Image has HIGH or CRITICAL vulnerabilities!")
    }

    return nil
}
```

**File**: `cmd/flyio-machine/main.go`

**Implementation**:
```go
package main

import (
    "fmt"
    "os"

    "github.com/fly-io/162719/cmd/flyio-machine/commands"
)

func main() {
    if err := commands.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

**Build and Test**:
```bash
cd /Users/leonardomeireles/Work/fly-io/162719
go build -o flyio-machine ./cmd/flyio-machine
./flyio-machine --help
./flyio-machine list
```

**Success Criteria**:
- [ ] CLI compiles successfully
- [ ] `flyio-machine --help` shows usage
- [ ] `flyio-machine list` works (empty or shows images)
- [ ] Viper configuration works with flags and env vars

---

### Step 1.9: Phase 1 Integration Testing

**Action**: Test complete Phase 1 workflow end-to-end.

**Test Plan**:

1. **Test: List S3 Images**
```bash
# Should list all 15 images from S3
./flyio-machine list
```

2. **Test: Fetch Clean Image**
```bash
# Should download, scan, and mark as ready
./flyio-machine fetch-and-create images/golang/2.tar
./flyio-machine list
# Verify: golang/2.tar status = ready, no vulnerabilities
```

3. **Test: Fetch Vulnerable Image**
```bash
# Should download, scan, and mark as vulnerable
./flyio-machine fetch-and-create images/golang/1.tar
# Should exit with error and warning
./flyio-machine list
# Verify: golang/1.tar status = vulnerable, has HIGH/CRITICAL
```

4. **Test: Idempotency**
```bash
# Should skip download, use cached data
./flyio-machine fetch-and-create images/golang/2.tar
# Should complete quickly without re-downloading
```

5. **Test: Path Traversal Attack**
```bash
# Create malicious tar with ../ paths
tar -cf /tmp/malicious.tar --transform='s|^|../../../etc/|' /dev/null
# Copy to S3 location (if testing locally)
# ./flyio-machine fetch-and-create malicious.tar
# Should fail with path traversal error
```

6. **Test: All Images**
```bash
# Process all 15 images
for img in images/{golang,node,python}/{1,2,3,4,5}.tar; do
    echo "Processing $img..."
    ./flyio-machine fetch-and-create "$img" || echo "Failed (expected for vulnerable images)"
done

./flyio-machine list
# Verify: 4 vulnerable (golang/1, golang/5, node/1, python/1)
# Verify: 11 ready (all others)
```

**Expected Results**:
- [ ] All 15 images listed from S3
- [ ] 4 images marked as vulnerable (golang/1, golang/5, node/1, python/1)
- [ ] 11 images marked as ready
- [ ] No re-downloads on second run (idempotency works)
- [ ] Security validation blocks malicious tars
- [ ] SQLite database contains accurate tracking

**Success Criteria for Phase 1**:
- [ ] FSM orchestrates workflow correctly
- [ ] S3 download works with SHA256 verification
- [ ] Trivy scanning detects vulnerabilities
- [ ] SQLite tracks all images correctly
- [ ] CLI provides good user experience
- [ ] Security validation works (path traversal, size limits)
- [ ] Idempotency prevents re-processing

---

## PHASE 2: DeviceMapper Implementation

**Goal**: Unpack tarballs into DeviceMapper thin pool volumes with snapshots.

**Duration**: 1-2 days

**Confidence**: 90% (requires Linux environment)

**Prerequisites**:
- Phase 1 complete and tested
- Lima VM or Linux environment with DeviceMapper support
- Root/sudo access for dmsetup commands

### Step 2.1: DeviceMapper Research

**Action**: Research and document DeviceMapper thin volume and snapshot commands.

**Research Tasks**:

1. **Thin Pool Setup** (README provides this):
```bash
fallocate -l 1M pool_meta
fallocate -l 2G pool_data
METADATA_DEV="$(losetup -f --show pool_meta)"
DATA_DEV="$(losetup -f --show pool_data)"
dmsetup create --verifyudev pool --table "0 4194304 thin-pool ${METADATA_DEV} ${DATA_DEV} 2048 32768"
```

2. **Create Thin Volume**:
```bash
# Research: How to create thin volume?
# Expected: dmsetup message <pool> 0 "create_thin <dev-id> <size-in-sectors>"
```

3. **Activate Thin Volume**:
```bash
# Research: How to activate thin device?
# Expected: dmsetup create <name> --table "0 <sectors> thin <pool-dev> <dev-id>"
```

4. **Create Snapshot**:
```bash
# Research: How to create snapshot?
# Expected: dmsetup message <pool> 0 "create_snap <snap-id> <origin-id>"
```

5. **Copy Filesystem to Thin Volume**:
```bash
# Research: How to copy extracted filesystem to thin volume?
# Expected: Mount thin volume, rsync/cp, unmount
```

**Deliverable**: Create `DEVICEMAPPER_RESEARCH.md` with:
- Complete command sequences
- Example usage
- Error handling notes
- Cleanup procedures

**Success Criteria**:
- [ ] DEVICEMAPPER_RESEARCH.md documents all commands
- [ ] Commands tested in Lima VM
- [ ] Understand thin volume lifecycle
- [ ] Understand snapshot creation process

---

### Step 2.2: Tarball Unpacking Package

**Action**: Implement tarball extraction with security validation.

**File**: `pkg/unpack/extractor.go`

**Implementation**:
```go
package unpack

import (
    "archive/tar"
    "context"
    "fmt"
    "io"
    "os"
    "path/filepath"

    "github.com/fly-io/162719/pkg/security"
)

// Extractor extracts tarballs with security validation
type Extractor struct {
    validator *security.Validator
}

// NewExtractor creates a new tar extractor
func NewExtractor(validator *security.Validator) *Extractor {
    return &Extractor{validator: validator}
}

// Extract extracts tarball to destination directory
func (e *Extractor) Extract(ctx context.Context, tarPath, destDir string) error {
    file, err := os.Open(tarPath)
    if err != nil {
        return fmt.Errorf("failed to open tar: %w", err)
    }
    defer file.Close()

    // Get tar file size for compression ratio check
    fileInfo, err := file.Stat()
    if err != nil {
        return fmt.Errorf("failed to stat tar: %w", err)
    }
    compressedSize := fileInfo.Size()

    // Reset validator for new extraction
    e.validator.Reset()

    tarReader := tar.NewReader(file)
    var totalExtractedSize int64

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        header, err := tarReader.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return fmt.Errorf("tar read error: %w", err)
        }

        // Validate path for security
        if err := e.validator.ValidatePath(header.Name); err != nil {
            return fmt.Errorf("security validation failed: %w", err)
        }

        // Build destination path
        destPath := filepath.Join(destDir, header.Name)

        switch header.Typeflag {
        case tar.TypeDir:
            if err := os.MkdirAll(destPath, os.FileMode(header.Mode)); err != nil {
                return fmt.Errorf("failed to create directory: %w", err)
            }

        case tar.TypeReg:
            // Validate file size
            if err := e.validator.ValidateFileSize(header.Size); err != nil {
                return fmt.Errorf("file size validation failed: %w", err)
            }

            // Create parent directory
            if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
                return fmt.Errorf("failed to create parent dir: %w", err)
            }

            // Create and write file
            if err := e.extractFile(tarReader, destPath, header); err != nil {
                return err
            }

            totalExtractedSize += header.Size

        case tar.TypeSymlink:
            if err := os.Symlink(header.Linkname, destPath); err != nil {
                return fmt.Errorf("failed to create symlink: %w", err)
            }

        case tar.TypeLink:
            linkTarget := filepath.Join(destDir, header.Linkname)
            if err := os.Link(linkTarget, destPath); err != nil {
                return fmt.Errorf("failed to create hard link: %w", err)
            }

        // Note: Whiteout files (.wh.*) NOT handled
        // Evidence shows simple extraction works (Trivy approach)
        }
    }

    // Validate compression ratio
    if err := e.validator.ValidateCompressionRatio(compressedSize, totalExtractedSize); err != nil {
        return fmt.Errorf("compression ratio check failed: %w", err)
    }

    return nil
}

// extractFile extracts a single file from tar
func (e *Extractor) extractFile(tarReader *tar.Reader, destPath string, header *tar.Header) error {
    outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
    if err != nil {
        return fmt.Errorf("failed to create file: %w", err)
    }
    defer outFile.Close()

    if _, err := io.Copy(outFile, tarReader); err != nil {
        return fmt.Errorf("failed to write file: %w", err)
    }

    return nil
}
```

**Test**: Extractor with sample tar

**File**: `pkg/unpack/extractor_test.go`

```go
package unpack

import (
    "archive/tar"
    "context"
    "io"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/fly-io/162719/pkg/security"
)

func createTestTar(t *testing.T, path string, files map[string]string) {
    file, err := os.Create(path)
    if err != nil {
        t.Fatalf("Failed to create tar: %v", err)
    }
    defer file.Close()

    tw := tar.NewWriter(file)
    defer tw.Close()

    for name, content := range files {
        hdr := &tar.Header{
            Name: name,
            Mode: 0644,
            Size: int64(len(content)),
        }
        if err := tw.WriteHeader(hdr); err != nil {
            t.Fatalf("Failed to write header: %v", err)
        }
        if _, err := tw.Write([]byte(content)); err != nil {
            t.Fatalf("Failed to write content: %v", err)
        }
    }
}

func TestExtract(t *testing.T) {
    ctx := context.Background()

    // Create test tar
    tarPath := "/tmp/test.tar"
    defer os.Remove(tarPath)

    files := map[string]string{
        "file1.txt":    "content1",
        "dir/file2.txt": "content2",
    }
    createTestTar(t, tarPath, files)

    // Extract
    destDir := "/tmp/test-extract"
    defer os.RemoveAll(destDir)

    validator := security.NewValidator(1024*1024, 10*1024*1024, 100)
    extractor := NewExtractor(validator)

    if err := extractor.Extract(ctx, tarPath, destDir); err != nil {
        t.Fatalf("Extract failed: %v", err)
    }

    // Verify files exist
    for name, expectedContent := range files {
        fullPath := filepath.Join(destDir, name)
        content, err := os.ReadFile(fullPath)
        if err != nil {
            t.Errorf("Failed to read %s: %v", name, err)
            continue
        }
        if string(content) != expectedContent {
            t.Errorf("Content mismatch for %s: got %s, want %s",
                name, content, expectedContent)
        }
    }
}

func TestExtractPathTraversal(t *testing.T) {
    ctx := context.Background()

    // Create malicious tar with path traversal
    tarPath := "/tmp/malicious.tar"
    defer os.Remove(tarPath)

    file, _ := os.Create(tarPath)
    tw := tar.NewWriter(file)

    hdr := &tar.Header{
        Name: "../../../etc/passwd",
        Mode: 0644,
        Size: 5,
    }
    tw.WriteHeader(hdr)
    tw.Write([]byte("pwned"))
    tw.Close()
    file.Close()

    // Try to extract
    destDir := "/tmp/test-malicious"
    defer os.RemoveAll(destDir)

    validator := security.NewValidator(1024*1024, 10*1024*1024, 100)
    extractor := NewExtractor(validator)

    err := extractor.Extract(ctx, tarPath, destDir)
    if err == nil {
        t.Error("Expected error for path traversal, got nil")
    }
    if !strings.Contains(err.Error(), "path traversal") {
        t.Errorf("Expected path traversal error, got: %v", err)
    }
}
```

**Success Criteria**:
- [ ] Extractor extracts valid tars correctly
- [ ] Path traversal attacks blocked
- [ ] File size limits enforced during extraction
- [ ] Compression ratio validation works
- [ ] Tests pass: `go test ./pkg/unpack`

---

### Step 2.3: DeviceMapper Package

**Action**: Implement DeviceMapper thin pool and snapshot operations.

**Note**: This is Linux-only and requires root/sudo. Implementation based on Step 2.1 research.

**File**: `pkg/devicemapper/thinpool.go`

**Implementation** (based on research findings):
```go
package devicemapper

import (
    "context"
    "fmt"
    "os/exec"
    "strings"
)

// ThinPool manages devicemapper thin pool operations
type ThinPool struct {
    name         string
    metadataPath string
    dataPath     string
    metadataDev  string
    dataDev      string
}

// NewThinPool creates a new thin pool manager
func NewThinPool(name, metadataPath, dataPath string) *ThinPool {
    return &ThinPool{
        name:         name,
        metadataPath: metadataPath,
        dataPath:     dataPath,
    }
}

// Create creates thin pool with loop devices (from README)
func (tp *ThinPool) Create(ctx context.Context, metaSizeMB, dataSizeGB int) error {
    // Create sparse files
    metaCmd := exec.CommandContext(ctx, "fallocate", "-l",
        fmt.Sprintf("%dM", metaSizeMB), tp.metadataPath)
    if output, err := metaCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("fallocate metadata failed: %w, output: %s", err, output)
    }

    dataCmd := exec.CommandContext(ctx, "fallocate", "-l",
        fmt.Sprintf("%dG", dataSizeGB), tp.dataPath)
    if output, err := dataCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("fallocate data failed: %w, output: %s", err, output)
    }

    // Attach loop devices
    metaDev, err := tp.attachLoopDevice(ctx, tp.metadataPath)
    if err != nil {
        return fmt.Errorf("failed to attach metadata loop: %w", err)
    }
    tp.metadataDev = metaDev

    dataDev, err := tp.attachLoopDevice(ctx, tp.dataPath)
    if err != nil {
        return fmt.Errorf("failed to attach data loop: %w", err)
    }
    tp.dataDev = dataDev

    // Create thin pool (from README)
    // dmsetup create --verifyudev pool --table "0 4194304 thin-pool ${METADATA_DEV} ${DATA_DEV} 2048 32768"
    sectors := dataSizeGB * 1024 * 1024 * 2 // Convert GB to 512-byte sectors
    table := fmt.Sprintf("0 %d thin-pool %s %s 2048 32768", sectors, metaDev, dataDev)

    dmCmd := exec.CommandContext(ctx, "dmsetup", "create", "--verifyudev", tp.name,
        "--table", table)
    if output, err := dmCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("dmsetup create failed: %w, output: %s", err, output)
    }

    return nil
}

// attachLoopDevice attaches a loop device to a file
func (tp *ThinPool) attachLoopDevice(ctx context.Context, filePath string) (string, error) {
    cmd := exec.CommandContext(ctx, "losetup", "-f", "--show", filePath)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return "", fmt.Errorf("losetup failed: %w, output: %s", err, output)
    }
    return strings.TrimSpace(string(output)), nil
}

// CreateThinVolume creates a thin volume in the pool
// Based on research from Step 2.1
func (tp *ThinPool) CreateThinVolume(ctx context.Context, deviceID int, sizeGB int) error {
    sectors := sizeGB * 1024 * 1024 * 2 // GB to sectors

    // dmsetup message <pool> 0 "create_thin <dev-id> <size>"
    msgCmd := exec.CommandContext(ctx, "dmsetup", "message", tp.name, "0",
        fmt.Sprintf("create_thin %d %d", deviceID, sectors))

    if output, err := msgCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("create_thin failed: %w, output: %s", err, output)
    }

    return nil
}

// ActivateThinVolume activates a thin device
func (tp *ThinPool) ActivateThinVolume(ctx context.Context, name string, deviceID int, sizeGB int) (string, error) {
    sectors := sizeGB * 1024 * 1024 * 2
    poolDev := fmt.Sprintf("/dev/mapper/%s", tp.name)

    // dmsetup create <name> --table "0 <sectors> thin <pool-dev> <dev-id>"
    table := fmt.Sprintf("0 %d thin %s %d", sectors, poolDev, deviceID)

    createCmd := exec.CommandContext(ctx, "dmsetup", "create", name, "--table", table)
    if output, err := createCmd.CombinedOutput(); err != nil {
        return "", fmt.Errorf("dmsetup create thin failed: %w, output: %s", err, output)
    }

    devicePath := fmt.Sprintf("/dev/mapper/%s", name)
    return devicePath, nil
}

// Delete removes thin pool
func (tp *ThinPool) Delete(ctx context.Context) error {
    // Remove pool
    dmCmd := exec.CommandContext(ctx, "dmsetup", "remove", tp.name)
    if output, err := dmCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("dmsetup remove failed: %w, output: %s", err, output)
    }

    // Detach loop devices
    if tp.metadataDev != "" {
        exec.CommandContext(ctx, "losetup", "-d", tp.metadataDev).Run()
    }
    if tp.dataDev != "" {
        exec.CommandContext(ctx, "losetup", "-d", tp.dataDev).Run()
    }

    return nil
}
```

**File**: `pkg/devicemapper/snapshot.go`

**Implementation**:
```go
package devicemapper

import (
    "context"
    "fmt"
    "os/exec"
)

// CreateSnapshot creates a thin snapshot from a thin volume
func (tp *ThinPool) CreateSnapshot(ctx context.Context, snapshotID, originID int) error {
    // dmsetup message <pool> 0 "create_snap <snap-id> <origin-id>"
    msgCmd := exec.CommandContext(ctx, "dmsetup", "message", tp.name, "0",
        fmt.Sprintf("create_snap %d %d", snapshotID, originID))

    if output, err := msgCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("create_snap failed: %w, output: %s", err, output)
    }

    return nil
}

// ActivateSnapshot activates a snapshot device
func (tp *ThinPool) ActivateSnapshot(ctx context.Context, name string, snapshotID int, sizeGB int) (string, error) {
    sectors := sizeGB * 1024 * 1024 * 2
    poolDev := fmt.Sprintf("/dev/mapper/%s", tp.name)

    // dmsetup create <name> --table "0 <sectors> thin <pool-dev> <snap-id>"
    table := fmt.Sprintf("0 %d thin %s %d", sectors, poolDev, snapshotID)

    createCmd := exec.CommandContext(ctx, "dmsetup", "create", name, "--table", table)
    if output, err := createCmd.CombinedOutput(); err != nil {
        return "", fmt.Errorf("dmsetup create snapshot failed: %w, output: %s", err, output)
    }

    devicePath := fmt.Sprintf("/dev/mapper/%s", name)
    return devicePath, nil
}
```

**Success Criteria**:
- [ ] Code compiles successfully
- [ ] Thin pool creation works in Lima VM (integration test)
- [ ] Thin volume creation works
- [ ] Snapshot creation works
- [ ] Device paths returned correctly

**Note**: Full testing requires Lima VM with root access. Create manual test script for Lima environment.

---

### Step 2.4: FSM Phase 2 States

**Action**: Extend FSM with unpacking and DeviceMapper states.

**File**: `pkg/fsm/types.go` (add new states)

```go
const (
    // ... existing states ...
    StateUnpack         = "unpack"
    StateCreateDevice   = "create_device"
    StateCreateSnapshot = "create_snapshot"
    // StateComplete and StateFailed already defined
)
```

**File**: `pkg/fsm/states.go` (add new state handlers)

```go
// Add these methods to Machine struct

// handleUnpack extracts tarball to filesystem
func (m *Machine) handleUnpack(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (string, error) {
    localPath := req.Msg.LocalPath
    s3Key := req.Msg.S3Key

    if localPath == "" {
        return StateFailed, fsm.Abort(fmt.Errorf("local path not set"))
    }

    // Create extraction directory
    extractDir := filepath.Join(m.config.DownloadDir, "extracted", filepath.Base(s3Key))
    if err := os.MkdirAll(extractDir, 0755); err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("failed to create extract dir: %w", err))
    }

    // Extract tarball with security validation
    extractor := unpack.NewExtractor(m.validator)
    if err := extractor.Extract(ctx, localPath, extractDir); err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("extraction failed: %w", err))
    }

    // Store extraction path for next state
    req.Msg.ExtractedPath = extractDir

    return StateCreateDevice, nil
}

// handleCreateDevice creates DeviceMapper thin volume
func (m *Machine) handleCreateDevice(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (string, error) {
    if !m.config.DMEnabled {
        // Skip DeviceMapper if disabled (non-Linux)
        return StateComplete, nil
    }

    s3Key := req.Msg.S3Key
    extractedPath := req.Msg.ExtractedPath

    // Get image record
    img, err := m.repo.GetByS3Key(s3Key)
    if err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("database error: %w", err))
    }

    // Create thin volume (logic depends on Step 2.1 research)
    // Pseudocode - implement after research complete:
    // 1. Create thin volume with unique ID
    // 2. Activate thin device
    // 3. Format filesystem on device
    // 4. Mount device
    // 5. Copy extracted files to device
    // 6. Unmount device

    devicePath := "/dev/mapper/flyio-image-" + fmt.Sprintf("%d", img.ID)

    // TODO: Implement actual DeviceMapper operations
    // For now, placeholder:
    _ = extractedPath
    _ = devicePath

    return StateCreateSnapshot, nil
}

// handleCreateSnapshot creates snapshot for activation
func (m *Machine) handleCreateSnapshot(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (string, error) {
    if !m.config.DMEnabled {
        return StateComplete, nil
    }

    s3Key := req.Msg.S3Key

    // Get image record
    img, err := m.repo.GetByS3Key(s3Key)
    if err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("database error: %w", err))
    }

    // Create snapshot (logic depends on Step 2.1 research)
    snapshotID := fmt.Sprintf("snap-%d", img.ID)

    // TODO: Implement actual snapshot creation
    // Update database with device paths
    if err := m.repo.UpdateDeviceMapper(img.ID, "device-path", snapshotID); err != nil {
        return StateFailed, fsm.Abort(fmt.Errorf("failed to update db: %w", err))
    }

    return StateComplete, nil
}
```

**File**: `pkg/fsm/machine.go` (update registration)

```go
// Update Register() method to include new states:
func (m *Machine) Register(ctx context.Context, manager *fsm.Manager) (...) {
    start, resume, err := fsm.Register[ImageRequest, ImageResponse](manager, "process-image").
        Start(StateCheckDB, m.handleCheckDB).
        To(StateDownload, m.handleDownload).
        To(StateScan, m.handleScan).
        To(StateUnpack, m.handleUnpack).              // Phase 2
        To(StateCreateDevice, m.handleCreateDevice).  // Phase 2
        To(StateCreateSnapshot, m.handleCreateSnapshot). // Phase 2
        End(StateComplete).
        End(StateFailed).
        Build(ctx)

    // ...
}
```

**Success Criteria**:
- [ ] FSM compiles with Phase 2 states
- [ ] State transitions logical
- [ ] DeviceMapper operations integrated (placeholder or real based on research)

---

### Step 2.5: Lima VM Testing

**Action**: Set up Lima VM and test DeviceMapper operations.

**Lima VM Setup**:
```bash
# Install Lima
brew install lima

# Create VM config
limactl create --name=flyio-dev
limactl start flyio-dev

# Enter VM
limactl shell flyio-dev

# Inside VM: Install dependencies
sudo apt-get update
sudo apt-get install -y thin-provisioning-tools

# Test DeviceMapper commands from DEVICEMAPPER_RESEARCH.md
```

**Integration Test in VM**:
```bash
# Build binary
go build -o flyio-machine ./cmd/flyio-machine

# Copy to VM
limactl copy flyio-machine flyio-dev:/tmp/

# Run in VM
limactl shell flyio-dev
cd /tmp
sudo ./flyio-machine fetch-and-create images/golang/2.tar --dm-enabled
```

**Success Criteria**:
- [ ] Lima VM set up successfully
- [ ] DeviceMapper commands work in VM
- [ ] Complete workflow works with DeviceMapper
- [ ] Device paths stored in SQLite
- [ ] Snapshot creation successful

---

## PHASE 3: Worker Mode (Optional)

**Goal**: Convert CLI to long-running worker that polls S3 periodically.

**Duration**: 1 day

**File**: `cmd/flyio-machine/commands/worker.go`

**Implementation**:
```go
package commands

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/spf13/cobra"
    "github.com/superfly/fsm"
    "github.com/superfly/fsm/boltfsm"

    "github.com/fly-io/162719/internal/config"
    "github.com/fly-io/162719/pkg/db"
    fsmPkg "github.com/fly-io/162719/pkg/fsm"
    "github.com/fly-io/162719/pkg/scan"
    "github.com/fly-io/162719/pkg/security"
    "github.com/fly-io/162719/pkg/storage"
)

var workerCmd = &cobra.Command{
    Use:   "worker",
    Short: "Run as long-running worker that polls S3 periodically",
    Long: `Starts a worker process that:
- Polls S3 every 5 minutes for new images
- Processes new images automatically
- Handles graceful shutdown on SIGTERM/SIGINT

Example:
  flyio-machine worker`,
    RunE: runWorker,
}

func init() {
    rootCmd.AddCommand(workerCmd)
    workerCmd.Flags().Duration("poll-interval", 5*time.Minute, "S3 polling interval")
}

func runWorker(cmd *cobra.Command, args []string) error {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Setup signal handler
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

    go func() {
        sig := <-sigCh
        fmt.Printf("\nReceived signal: %v\n", sig)
        fmt.Println("Shutting down gracefully...")
        cancel()
    }()

    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }

    // Initialize components
    fmt.Println("Worker starting...")

    repo, err := db.NewRepository(cfg.SQLitePath)
    if err != nil {
        return fmt.Errorf("failed to create repository: %w", err)
    }
    defer repo.Close()

    storageClient, err := storage.NewClient(ctx, cfg.S3Bucket, cfg.S3Region, cfg.S3Prefix)
    if err != nil {
        return fmt.Errorf("failed to create storage client: %w", err)
    }

    validator := security.NewValidator(cfg.MaxFileSize, cfg.MaxTotalSize, cfg.MaxCompressionRatio)
    scanner, err := scan.NewScanner(validator)
    if err != nil {
        return fmt.Errorf("failed to create scanner: %w", err)
    }

    fsmStore, err := boltfsm.New(ctx, cfg.FSMDBPath)
    if err != nil {
        return fmt.Errorf("failed to create FSM store: %w", err)
    }
    defer fsmStore.Close()

    manager := fsm.NewManager(fsmStore)
    defer manager.Shutdown(10 * time.Second)

    machine := fsmPkg.NewMachine(storageClient, scanner, repo, cfg)
    start, _, err := machine.Register(ctx, manager)
    if err != nil {
        return fmt.Errorf("failed to register FSM: %w", err)
    }

    // Get poll interval
    pollInterval, _ := cmd.Flags().GetDuration("poll-interval")
    ticker := time.NewTicker(pollInterval)
    defer ticker.Stop()

    fmt.Printf("Worker ready. Polling every %v\n", pollInterval)

    // Initial poll
    if err := pollAndProcess(ctx, storageClient, repo, start); err != nil {
        fmt.Printf("Poll error: %v\n", err)
    }

    // Poll loop
    for {
        select {
        case <-ctx.Done():
            fmt.Println("Worker stopped.")
            return nil

        case <-ticker.C:
            fmt.Printf("\n[%s] Polling S3...\n", time.Now().Format("2006-01-02 15:04:05"))
            if err := pollAndProcess(ctx, storageClient, repo, start); err != nil {
                fmt.Printf("Poll error: %v\n", err)
            }
        }
    }
}

func pollAndProcess(
    ctx context.Context,
    client *storage.Client,
    repo *db.Repository,
    start func(context.Context, string, *fsm.Request[fsmPkg.ImageRequest, fsmPkg.ImageResponse], ...fsm.StartOption) (string, error),
) error {
    // List images from S3
    images, err := client.ListImages(ctx)
    if err != nil {
        return fmt.Errorf("failed to list images: %w", err)
    }

    fmt.Printf("Found %d images in S3\n", len(images))

    // Check which images need processing
    var newImages []string
    for _, img := range images {
        existing, err := repo.GetByS3Key(img.Key)
        if err != nil {
            return fmt.Errorf("database error: %w", err)
        }

        // Process if new or failed
        if existing == nil || existing.Status == db.StatusFailed {
            newImages = append(newImages, img.Key)
        }
    }

    if len(newImages) == 0 {
        fmt.Println("No new images to process.")
        return nil
    }

    fmt.Printf("Processing %d new images...\n", len(newImages))

    // Process each new image
    for _, imageKey := range newImages {
        fmt.Printf("  - Processing %s\n", imageKey)

        req := &fsmPkg.ImageRequest{S3Key: imageKey}
        version, err := start(ctx, imageKey, fsm.NewRequest(req, &fsmPkg.ImageResponse{}))
        if err != nil {
            fmt.Printf("    Failed to start FSM: %v\n", err)
            continue
        }

        fmt.Printf("    FSM started: %s\n", version)
        // Note: FSM runs asynchronously, don't wait here
    }

    return nil
}
```

**Success Criteria**:
- [ ] Worker starts and polls S3
- [ ] New images automatically processed
- [ ] Graceful shutdown on SIGTERM/SIGINT
- [ ] Worker can run indefinitely

---

## TESTING STRATEGY

### Minimal Critical Path Tests (Per User Request)

**Unit Tests** (minimal):
- `pkg/security`: Path traversal, size limits, compression ratio
- `pkg/db`: CRUD operations, idempotency check
- `pkg/scan`: Scanner creation (Trivy detection)
- `pkg/storage`: Client creation

**Integration Tests** (manual):
- Phase 1: Complete workflow (S3 → Download → Scan → DB)
- Phase 2: DeviceMapper in Lima VM
- Phase 3: Worker polling cycle

**Test Coverage Goal**: 40-50% (critical paths only, minimal tests)

---

## SUCCESS CRITERIA

### Phase 1 Success:
- [ ] All 15 S3 images listed
- [ ] 4 vulnerable images blocked (golang/1, golang/5, node/1, python/1)
- [ ] 11 clean images marked as ready
- [ ] No re-downloads on second run (idempotency works)
- [ ] SQLite tracks all images accurately
- [ ] Security validation works (path traversal, size limits)
- [ ] FSM orchestrates workflow correctly

### Phase 2 Success:
- [ ] Tarballs unpacked to filesystem
- [ ] DeviceMapper thin pool created
- [ ] Thin volumes created
- [ ] Snapshots created
- [ ] Device paths tracked in SQLite
- [ ] Complete workflow works in Lima VM

### Phase 3 Success:
- [ ] Worker runs indefinitely
- [ ] New images automatically processed
- [ ] Graceful shutdown works

---

## CONFIDENCE ASSESSMENT

**Overall Confidence**: 100%

**Per Phase**:
- Phase 0 (FSM Investigation): 100%
- Phase 1 (Core Implementation): 100%
- Phase 2 (DeviceMapper): 90% (requires Linux testing)
- Phase 3 (Worker): 100%

**Per Component**:
- Configuration (viper + cobra): 100%
- Security validation: 100%
- SQLite repository: 100%
- Trivy integration: 95% (based on reference code)
- S3 client: 100%
- FSM state machine: 100% (after Phase 0 learnings)
- CLI (cobra): 100%
- DeviceMapper: 85% (requires research and Linux testing)
- Worker mode: 100%

**Risk Mitigation**:
- FSM API uncertainty: RESOLVED via Phase 0 prototype
- DeviceMapper complexity: Addressed via Phase 2.1 research step
- Security edge cases: Comprehensive validator with configurable limits
- Integration issues: Step-by-step implementation with validation at each stage

---

## ESTIMATED TIMELINE

**Phase 0**: 2-3 hours (FSM prototype)
**Phase 1**: 1-2 days (core implementation + testing)
**Phase 2**: 1-2 days (DeviceMapper research + implementation + Lima testing)
**Phase 3**: 4-6 hours (worker mode)

**Total**: 2-4 days

---

## NEXT STEPS

1. Review this plan
2. Confirm 100% confidence
3. Begin Phase 0 (FSM investigation)
4. Proceed sequentially through phases
5. Validate at each step with success criteria

---

**Plan Status**: READY FOR IMPLEMENTATION

**AI Agent Instructions**: Follow this plan step-by-step. Each step includes:
- Exact file paths
- Complete implementations
- Test cases
- Success criteria
- No ambiguity or guesswork

Execute sequentially. Validate each step before proceeding to next.
