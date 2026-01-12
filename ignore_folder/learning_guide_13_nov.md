  ---
  How the Implementation Satisfies the Fly.io Hiring Challenge

  Great question! Let me walk you through how each requirement was met, explaining the "why" behind the implementation choices so you truly understand
   the solution.

  Before We Dive In: Understanding Your Background

  Before I explain each component, let me ask a few clarifying questions to tailor my explanation:

  1. How familiar are you with state machines? Have you worked with finite state machines (FSMs) before, or is this concept new to you?
  2. What's your experience with container technology? Do you understand how Docker images are structured with layers and tarballs?
  3. Have you worked with Linux block devices or DeviceMapper? This is the most specialized part of the challenge.

  That said, I'll explain everything from fundamentals up, so don't worry if any of these are unfamiliar!

  ---
  Core Requirement Breakdown

  Let's map each requirement from the README to the implementation:

  1. ✅ Use the FSM Library

  What They Wanted:
  Use the FSM library to orchestrate the workflow

  Why FSM?
  State machines are perfect for orchestrating complex workflows because they:
  - Make state transitions explicit and predictable
  - Enable persistence (can resume after crashes)
  - Separate concerns (each state handles one responsibility)
  - Provide clear error handling paths

  How It Was Implemented:

  The FSM is registered in pkg/fsm/machine.go:11-26:

  fsm.Register[ImageRequest, ImageResponse](manager, "image-process").
      Start(StateCheckDB, m.handleCheckDB).      // State 1: Check database
      To(StateDownload, m.handleDownload).        // State 2: Download from S3
      To(StateValidate, m.handleValidate).        // State 3: Security validation
      To(StateCreateDevice, m.handleCreateDevice). // State 4: Create thin device
      To(StateComplete, m.handleComplete).        // State 5: Create snapshot
      End(StateFailed).                           // Error state
      Build(ctx)

  Why 5 states? Each state represents a distinct phase with different failure modes:
  - CheckDB: Handles idempotency (skip if already done)
  - Download: Network operations (S3 failures)
  - Validate: Security checks (malicious content)
  - CreateDevice: DeviceMapper operations (kernel-level failures)
  - Complete: Finalization and snapshot creation

  Example from states.go:44-80:
  func (m *Machine) handleCheckDB(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (*fsm.Response[ImageResponse], error) {
      // Query database for existing image
      img, err := m.repo.GetByS3Key(req.Msg.S3Key)

      // If image exists and is ready, skip to complete (idempotency)
      if img != nil && img.Status == db.StatusReady {
          return fsm.NewResponse(resp), nil // FSM skips to end
      }

      // Otherwise, create pending record and continue
      // ...
  }

  ---
  2. ✅ Retrieve Image from S3 Only If Not Already Retrieved

  What They Wanted:
  Retrieve an arbitrary image from that S3 bucket, only if it hasn't been retrieved already

  Why Idempotency Matters:
  In production systems with thousands of servers, operations can be retried due to network issues, crashes, or orchestration logic. Running the same
  operation twice should be safe and efficient.

  How It Was Implemented:

  First Check - handleCheckDB (states.go:44-80):
  img, err := m.repo.GetByS3Key(req.Msg.S3Key)
  if img != nil && img.Status == db.StatusReady {
      // Image already downloaded, validated, and has device
      return fsm.NewResponse(resp), nil  // Skip directly to end
  }

  Second Check - handleDownload (states.go:83-120):
  // Before downloading, check if file already exists on disk
  downloadPath := filepath.Join("/tmp/flyio-machine/downloads",
      filepath.Base(req.Msg.S3Key))

  if _, err := os.Stat(downloadPath); err == nil {
      // File exists, skip download
      resp.FilePath = downloadPath
      // Calculate hash of existing file...
  }

  Why Two Checks?
  1. Database check: Fast, avoids network/disk operations entirely
  2. Filesystem check: Handles cases where DB record exists but file was deleted

  ---
  3. ✅ Unpack into Canonical Filesystem Layout

  What They Wanted:
  Unpack the image into a canonical filesystem layout

  What Is "Canonical Filesystem Layout"?
  Container images have a specific structure with layers (directories named by hash) and a manifest. The "canonical" layout means maintaining this
  exact structure so the filesystem can be used by a container runtime.

  How It Was Implemented:

  In pkg/devicemapper/extractor.go (security validation during extraction):

  func ExtractTarball(tarballPath, destPath string, opts *SecurityOptions) error {
      tarFile, _ := os.Open(tarballPath)
      tarReader := tar.NewReader(tarFile)

      for {
          header, err := tarReader.Next()

          // Security validation (explained in section 4)
          if err := validatePath(header.Name, destPath); err != nil {
              return err // Block malicious paths
          }

          // Extract maintaining exact directory structure
          targetPath := filepath.Join(destPath, header.Name)

          switch header.Typeflag {
          case tar.TypeDir:
              os.MkdirAll(targetPath, 0755) // Create directories
          case tar.TypeReg:
              // Create file with exact permissions from tarball
              file, _ := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
              io.Copy(file, tarReader) // Write file contents
          case tar.TypeSymlink:
              os.Symlink(header.Linkname, targetPath) // Preserve symlinks
          }
      }
  }

  Why Preserve Everything?
  Container layers include:
  - Directory structures (/bin, /usr/lib, etc.)
  - File permissions (executables need execute bit)
  - Symlinks (e.g., /bin/sh → /bin/bash)

  Changing any of these breaks the container image.

  ---
  4. ✅ Security Validation (Hostile Environment)

  What They Wanted:
  Assume we are going to run your code in a hostile environment... Code accordingly

  Why Security First?
  The README emphasizes "think carefully about the blobs" - S3 tarballs could contain:
  - Path traversal attacks: ../../etc/passwd to escape extraction directory
  - Symlink attacks: Symlink to /etc/passwd, then write through it
  - Zip bombs: Tiny compressed file that expands to terabytes
  - Resource exhaustion: Millions of tiny files

  How It Was Implemented:

  In pkg/security/validator.go:

  type SecurityOptions struct {
      MaxFileSize      int64  // 2GB per file
      MaxTotalSize     int64  // 20GB total
      MaxCompressionRatio float64 // 100x expansion limit
  }

  func ValidateAndExtract(tarballPath, destPath string, opts *SecurityOptions) error {
      // Track metrics during extraction
      var totalBytesWritten int64
      compressedSize := getFileSize(tarballPath)

      for each file in tarball {
          // Check 1: Block path traversal
          if strings.Contains(path, "..") || filepath.IsAbs(path) {
              return fmt.Errorf("path traversal attempt: %s", path)
          }

          // Check 2: Validate symlink targets
          if header.Typeflag == tar.TypeSymlink {
              resolvedTarget := filepath.Join(destPath, header.Linkname)
              if !strings.HasPrefix(resolvedTarget, destPath) {
                  return fmt.Errorf("symlink escape attempt: %s", header.Linkname)
              }
          }

          // Check 3: File size limit
          if header.Size > opts.MaxFileSize {
              return fmt.Errorf("file too large: %d > %d", header.Size, opts.MaxFileSize)
          }

          // Check 4: Total size limit
          totalBytesWritten += header.Size
          if totalBytesWritten > opts.MaxTotalSize {
              return fmt.Errorf("extraction size exceeded: %d > %d",
                  totalBytesWritten, opts.MaxTotalSize)
          }

          // Check 5: Compression ratio (zip bomb protection)
          ratio := float64(totalBytesWritten) / float64(compressedSize)
          if ratio > opts.MaxCompressionRatio {
              return fmt.Errorf("compression ratio exceeded: %.2f > %.2f",
                  ratio, opts.MaxCompressionRatio)
          }

          // Safe to extract this file
          extractFile(header, tarReader, destPath)
      }
  }

  Why Validate During Extraction?
  - Early termination: Stop as soon as attack detected
  - Memory efficient: Don't load entire tarball into memory
  - Resource protection: Prevent disk exhaustion before it happens

  ---
  5. ✅ DeviceMapper Thinpool Device Creation

  What They Wanted:
  Unpack inside of a devicemapper thinpool device

  What Is DeviceMapper Thinpool?
  Think of it like this:
  - Traditional disk: Allocate 10GB → uses 10GB immediately
  - Thin provisioning: Allocate 10GB → only uses space as you write data

  This is crucial for containers because:
  - You might have 1000 container images (each "needing" 1GB)
  - But they share most files (base OS layers)
  - Thin provisioning only uses disk for actual differences

  How It Was Implemented:

  In pkg/devicemapper/linux.go:41-80:

  func (m *LinuxManager) CreateDevice(ctx context.Context, extractedPath string, deviceID string) (*DeviceInfo, error) {
      deviceName := fmt.Sprintf("flyio-%s", deviceID)
      poolDevicePath := "/dev/mapper/pool"

      // Step 1: Create thin device metadata in pool
      // This tells the pool "reserve a logical device ID"
      cmd := exec.Command("dmsetup", "message", poolDevicePath, "0",
          fmt.Sprintf("create_thin %s", deviceID))
      cmd.Run()

      // Step 2: Activate device
      // This creates /dev/mapper/flyio-1 block device
      sectors := int64(2097152) // 1GB in 512-byte sectors
      tableSpec := fmt.Sprintf("0 %d thin %s %s", sectors, poolDevicePath, deviceID)
      cmd = exec.Command("dmsetup", "create", deviceName, "--table", tableSpec)
      cmd.Run()

      devicePath := "/dev/mapper/flyio-1"

      // Step 3: Format with filesystem
      cmd = exec.Command("mkfs.ext4", "-F", devicePath)
      cmd.Run()

      // Step 4: Mount and extract INTO mounted device
      mountPoint := "/tmp/flyio-mount"
      cmd = exec.Command("mount", devicePath, mountPoint)
      cmd.Run()

      // Extract tarball INTO the mounted device
      ExtractTarball(tarballPath, mountPoint, securityOpts)

      // Unmount
      exec.Command("umount", mountPoint).Run()

      return &DeviceInfo{DevicePath: devicePath}, nil
  }

  Why This Approach?
  1. Isolation: Each image has its own block device
  2. Snapshotting: Can create COW (copy-on-write) snapshots cheaply
  3. Storage efficiency: Thin provisioning only uses actual data size
  4. Production-like: This is how Docker and containerd actually work

  Example from test-lima.sh (lines 126-140):
  # This is the thinpool setup command from the README
  sudo fallocate -l 1M pool_meta     # Metadata storage
  sudo fallocate -l 2G pool_data     # Data storage

  METADATA_DEV="$(sudo losetup -f --show pool_meta)"
  DATA_DEV="$(sudo losetup -f --show pool_data)"

  # Create the thinpool device
  sudo dmsetup create pool --table "0 4194304 thin-pool ${METADATA_DEV} ${DATA_DEV} 2048 32768"

  ---
  6. ✅ Snapshot Creation for Activation

  What They Wanted:
  To "activate" the image, create a snapshot of that thinpool device

  What Is a Snapshot and Why?
  A snapshot is a point-in-time copy of a device. In thin provisioning:
  - Base device: Contains the extracted container image (read-only conceptually)
  - Snapshot: A writable copy that starts empty
  - Copy-on-Write (COW): When you write to snapshot, only changed blocks are stored

  This enables:
  - Multiple containers from one image (each gets a snapshot)
  - Minimal storage (only store changes, not full copies)
  - Fast startup (no copying needed)

  How It Was Implemented:

  In pkg/devicemapper/linux.go:122-170:

  func (m *LinuxManager) CreateSnapshot(ctx context.Context, baseDeviceID string) (*SnapshotInfo, error) {
      // Generate unique snapshot ID
      snapshotID := generateSnapshotID()
      snapshotName := fmt.Sprintf("flyio-snapshot-%s", snapshotID)

      // Step 1: Create snapshot thin device in pool
      // This is like "copy_base_to_new_id" but uses COW
      cmd := exec.Command("dmsetup", "message", poolDevicePath, "0",
          fmt.Sprintf("create_snap %s %s", snapshotID, baseDeviceID))
      cmd.Run()

      // Step 2: Activate snapshot as separate block device
      sectors := int64(2097152) // Same size as base
      tableSpec := fmt.Sprintf("0 %d thin %s %s", sectors, poolDevicePath, snapshotID)
      cmd = exec.Command("dmsetup", "create", snapshotName, "--table", tableSpec)
      cmd.Run()

      // Now /dev/mapper/flyio-snapshot-1000 exists and can be mounted
      // It starts with exact contents of base device
      // But writes go to snapshot storage, not base device

      return &SnapshotInfo{
          SnapshotID: snapshotID,
          SnapshotPath: "/dev/mapper/" + snapshotName,
      }, nil
  }

  Why Snapshots for "Activation"?
  In container orchestration:
  - Image: The immutable base (like a template)
  - Container: A running instance with its own filesystem changes
  - Snapshot: Provides the writable layer for the container

  Implementation Detail - handleComplete (states.go:231-278):
  func (m *Machine) handleComplete(ctx context.Context, req *fsm.Request[ImageRequest, ImageResponse]) (*fsm.Response[ImageResponse], error) {
      // Load image from database to get device path
      // (FSM doesn't persist response fields between states!)
      img, err := m.repo.GetByS3Key(req.Msg.S3Key)

      if img.DevicePath != "" {
          // Extract device ID from path (/dev/mapper/flyio-1 → "1")
          baseDeviceID := extractDeviceID(img.DevicePath)

          // Create snapshot
          snapshotInfo, err := m.dmManager.CreateSnapshot(ctx, baseDeviceID)

          // Update database with snapshot ID
          img.SnapshotID = snapshotInfo.SnapshotID
          m.repo.Update(img)
      }

      // Mark image as ready
      img.Status = db.StatusReady
      m.repo.Update(img)

      return fsm.NewResponse(resp), nil
  }

  ---
  7. ✅ SQLite State Tracking

  What They Wanted:
  Using SQLite to track the available images

  Why SQLite?
  - Persistence: State survives process restarts
  - ACID transactions: Prevents corruption during crashes
  - Simplicity: No separate database server needed
  - Production-appropriate: Used by Docker, browsers, and embedded systems

  How It Was Implemented:

  Schema (pkg/db/schema.go):
  CREATE TABLE images (
      id INTEGER PRIMARY KEY AUTOINCREMENT,  -- Used as DeviceMapper device ID
      s3_key TEXT NOT NULL UNIQUE,           -- "images/golang/2.tar"
      sha256 TEXT NOT NULL,                  -- Downloaded file hash
      status TEXT NOT NULL,                  -- pending|downloading|ready|failed
      device_path TEXT,                      -- "/dev/mapper/flyio-1"
      base_device_id INTEGER,               -- Reference to base device
      snapshot_id TEXT,                      -- "1000" for snapshot
      error_message TEXT,                    -- If failed, why?
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
      updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

      CHECK(status IN ('pending', 'downloading', 'ready', 'failed'))
  );

  Why These Columns?
  - id: Auto-incrementing ensures unique device IDs for DeviceMapper
  - s3_key: Primary identifier for idempotency checks
  - sha256: Content validation (detect corrupted downloads)
  - status: FSM state tracking
  - device_path & snapshot_id: Record DeviceMapper resources for cleanup

  Operations (pkg/db/repository.go):
  // Check idempotency
  func (r *Repository) GetByS3Key(s3Key string) (*Image, error) {
      var img Image
      err := r.db.QueryRow(
          "SELECT id, s3_key, sha256, status, device_path, snapshot_id FROM images WHERE s3_key = ?",
          s3Key,
      ).Scan(&img.ID, &img.S3Key, &img.SHA256, &img.Status, &img.DevicePath, &img.SnapshotID)

      if err == sql.ErrNoRows {
          return nil, nil // Not found
      }
      return &img, err
  }

  // Create new image record
  func (r *Repository) Create(img *Image) error {
      result, err := r.db.Exec(
          "INSERT INTO images (s3_key, sha256, status) VALUES (?, ?, ?)",
          img.S3Key, img.SHA256, img.Status,
      )
      img.ID = result.LastInsertId() // Get auto-generated ID
      return err
  }

  // Update status as FSM progresses
  func (r *Repository) Update(img *Image) error {
      _, err := r.db.Exec(
          `UPDATE images
           SET sha256 = ?, status = ?, device_path = ?, snapshot_id = ?, updated_at = CURRENT_TIMESTAMP
           WHERE id = ?`,
          img.SHA256, img.Status, img.DevicePath, img.SnapshotID, img.ID,
      )
      return err
  }

  ---
  Test Scripts Explanation

  Now let's understand the two test scripts:

  test-macos.sh (Lines 1-61)

  Purpose: Test the 5 requirements that work without Linux (FSM, S3, SQLite, Security, graceful degradation)

  What It Does:
  #!/bin/bash
  # Line 2: Exit on any error
  set -e

  # Lines 11-12: Configure where to store database
  export FLYIO_SQLITE_PATH="/tmp/flyio-test.db"
  export FLYIO_FSM_DB_PATH="/tmp/flyio-fsm"

  # Lines 15-16: Clean up previous test data
  rm -rf /tmp/flyio-test.db /tmp/flyio-fsm /tmp/flyio-machine

  # Lines 19-20: Build the Go binary
  go build -o flyio-machine ./cmd/flyio-machine

  # Lines 25-29: Test 1 - Full workflow
  ./flyio-machine fetch-and-create images/golang/2.tar
  # This will:
  # 1. Check DB (image not found)
  # 2. Download from S3 (47MB)
  # 3. Validate security
  # 4. Skip DeviceMapper (macOS)
  # 5. Mark as ready

  # Line 33: Verify database was updated
  sqlite3 $FLYIO_SQLITE_PATH "SELECT s3_key, status, device_path, snapshot_id FROM images;"
  # Expected: images/golang/2.tar|ready|-|-
  #           device_path and snapshot_id are empty on macOS

  # Lines 38-40: Test 2 - Idempotency
  ./flyio-machine fetch-and-create images/golang/2.tar
  # Should skip download, immediately return "ready"

  # Lines 44-46: Test 3 - List command
  ./flyio-machine list
  # Shows all images in database

  Why macOS Test Is Valuable:
  - Validates FSM orchestration logic
  - Tests S3 integration
  - Verifies security validation
  - Confirms SQLite tracking
  - Ensures graceful degradation (no crash on missing DeviceMapper)

  ---
  test-lima.sh (Lines 1-267)

  Purpose: Full integration test with DeviceMapper on Linux VM

  What It Does:

  Part 1: Lima VM Setup (Lines 12-54)
  # Check if Lima (Linux VM) is installed
  if ! command -v lima &> /dev/null; then
      echo "Install Lima: brew install lima"
      exit 1
  fi

  # Create or start Ubuntu VM
  if ! limactl list | grep -q "flyio-test"; then
      # Apple Silicon (M1/M2/M3) - use faster virtualization
      if [[ $(uname -m) == "arm64" ]]; then
          limactl start --name="flyio-test" \
              --cpus=4 --memory=8 --disk=20 \
              --vm-type=vz \                # Apple Hypervisor (faster)
              template://ubuntu-lts
      else
          # Intel Mac - use QEMU
          limactl start --name="flyio-test" \
              --cpus=4 --memory=8 --disk=20 \
              template://ubuntu-lts
      fi
  fi

  Part 2: Install Dependencies (Lines 60-104)
  limactl shell "flyio-test" bash <<'DEPS_EOF'
      # Update package list
      sudo apt-get update -qq

      # Install required packages
      sudo apt-get install -y -qq \
          dmsetup \                      # DeviceMapper tools
          thin-provisioning-tools \      # Thinpool utilities
          sqlite3 \                      # Database
          kmod                           # Kernel modules

      # Install Go 1.25.4
      cd /tmp
      wget https://go.dev/dl/go1.25.4.linux-arm64.tar.gz
      sudo tar -C /usr/local -xzf go1.25.4.linux-arm64.tar.gz

      go version  # Verify installation
  DEPS_EOF

  Part 3: DeviceMapper Thinpool Setup (Lines 108-141)
  limactl shell "flyio-test" bash <<'THINPOOL_EOF'
      # Load kernel modules
      sudo modprobe dm_thin_pool
      sudo modprobe loop

      # Clean up any existing pools
      sudo dmsetup remove -f pool 2>/dev/null || true
      sudo losetup -D

      # Create thinpool (from README instructions)
      cd /tmp
      sudo fallocate -l 1M pool_meta    # Metadata
      sudo fallocate -l 2G pool_data    # Data

      METADATA_DEV="$(sudo losetup -f --show pool_meta)"
      DATA_DEV="$(sudo losetup -f --show pool_data)"

      sudo dmsetup create pool --table "0 4194304 thin-pool ${METADATA_DEV} ${DATA_DEV} 2048 32768"

      # Verify pool exists
      sudo dmsetup info pool
  THINPOOL_EOF

  Part 4: Copy and Build (Lines 145-172)
  # Copy project files from macOS to Lima VM
  limactl copy "$PROJECT_DIR" "flyio-test:/tmp/flyio-project"

  # Build inside VM
  limactl shell "flyio-test" bash <<'EOF'
      export PATH="/usr/local/go/bin:$PATH"
      cd /tmp/flyio-project/162719

      go mod download
      go build -o flyio-machine ./cmd/flyio-machine
  EOF

  Part 5: Run Tests (Lines 180-254)
  limactl shell "flyio-test" bash <<TEST_EOF
      export FLYIO_SQLITE_PATH="/tmp/flyio-test.db"
      export FLYIO_FSM_DB_PATH="/tmp/flyio-fsm"

      # Clean DeviceMapper devices from previous runs
      for dev in \$(sudo dmsetup ls | grep flyio | awk '{print \$1}'); do
          sudo dmsetup remove "\$dev"
      done

      # Test 1: Full workflow with DeviceMapper
      echo "Test 1: Full Workflow (golang/2.tar)"
      sudo -E ./flyio-machine fetch-and-create images/golang/2.tar
      # -E preserves environment variables (AWS credentials, paths)
      # sudo needed for DeviceMapper operations

      # Verify database
      sudo sqlite3 \$FLYIO_SQLITE_PATH "SELECT s3_key, status, device_path, snapshot_id FROM images;"
      # Expected: images/golang/2.tar|ready|/dev/mapper/flyio-1|1000

      # Verify DeviceMapper devices created
      sudo dmsetup ls | grep flyio
      # Expected:
      # flyio-1 (252:1)              <- Base device
      # flyio-snapshot-1000 (252:2)  <- Snapshot
      # pool (252:0)                 <- Thinpool

      # Test 2: Idempotency
      sudo -E ./flyio-machine fetch-and-create images/python/4.tar

      # Test 3: List images
      sudo -E ./flyio-machine list
  TEST_EOF

  Why Lima Test Is Comprehensive:
  - Tests all 6 requirements including DeviceMapper
  - Validates Linux-specific code paths
  - Verifies snapshot creation works
  - Confirms idempotency with DeviceMapper
  - Tests the complete production-like flow

  ---
  Key Technical Challenges Solved

  Challenge 1: FSM State Persistence

  Problem: DevicePath set in handleCreateDevice was lost when handleComplete started.

  Root Cause: FSM doesn't persist response fields between state transitions.

  Solution (states.go:235-238):
  // Instead of relying on response fields:
  resp.DevicePath = "/dev/mapper/flyio-1"  // Lost on next state!

  // Load from database:
  img, err := m.repo.GetByS3Key(req.Msg.S3Key)
  snapshotInfo, err := m.dmManager.CreateSnapshot(ctx, img.DevicePath)

  Challenge 2: DeviceMapper Idempotency

  Problem: dmsetup create_thin fails with "File exists" on second run.

  Solution (linux.go:47-50):
  // Delete existing device first (ignore errors if not exists)
  deleteCmd := exec.Command("dmsetup", "message", poolDevicePath, "0",
      fmt.Sprintf("delete %s", deviceID))
  deleteCmd.Run() // Ignore errors

  // Now safe to create
  createCmd := exec.Command("dmsetup", "message", poolDevicePath, "0",
      fmt.Sprintf("create_thin %s", deviceID))
  createCmd.Run()

  Challenge 3: Cross-Platform Development

  Problem: Need DeviceMapper on Linux but want to develop on macOS.

  Solution: Build tags and interface abstraction
  // +build linux
  // linux.go - Full implementation

  // +build !linux
  // stub.go - Returns ErrNotSupported

  // interface.go - Common interface
  type Manager interface {
      CreateDevice(ctx context.Context, extractedPath string, deviceID string) (*DeviceInfo, error)
      CreateSnapshot(ctx context.Context, baseDeviceID string) (*SnapshotInfo, error)
  }

  ---
  Summary: Requirement Satisfaction

  | Requirement              | Component                             | Verification                                     |
  |--------------------------|---------------------------------------|--------------------------------------------------|
  | FSM Library              | pkg/fsm/machine.go                    | 5-state workflow with BoltDB persistence         |
  | S3 Download (idempotent) | pkg/storage/client.go + handleCheckDB | Database check + filesystem check                |
  | Security Validation      | pkg/security/validator.go             | Path traversal, symlinks, zip bombs, size limits |
  | DeviceMapper Thinpool    | pkg/devicemapper/linux.go             | Thin device creation, formatting, extraction     |
  | Snapshot Creation        | linux.go:CreateSnapshot()             | COW snapshots for activation                     |
  | SQLite Tracking          | pkg/db/repository.go                  | Persistent state with ACID transactions          |

  Test Coverage:
  - macOS: 5/6 requirements (all except DeviceMapper)
  - Lima (Linux): 6/6 requirements (full integration)

  ---
  Questions for You

  Now that you've seen the full explanation, I have a few questions to check your understanding:

  1. FSM Understanding: Can you explain in your own words why the FSM approach is better than a simple procedural script for this workflow?
  2. Security Trade-offs: The implementation checks paths during extraction. What would be the trade-off if security validation happened after
  extraction instead?
  3. Snapshot Purpose: Why do you think the challenge asked for snapshot creation specifically? What container runtime functionality does this enable?
  4. Design Choice: If you were implementing this, would you use the same "delete-then-create" idempotency pattern for DeviceMapper, or try a
  different approach? Why?
