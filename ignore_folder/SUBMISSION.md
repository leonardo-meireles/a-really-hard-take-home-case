# Platform Machines Challenge - Submission

## My Approach

When I first read through this challenge, I'll be honest - even though I work with Docker and Kubernetes daily and deploy machines regularly, I'd never actually implemented the inner workings of orchestrators or worked directly with DeviceMapper. That's exactly what made this challenge interesting to me.

My first step wasn't to code, but to understand what I didn't know. I mapped out the unknowns: FSM library architecture, DeviceMapper thin provisioning, snapshot mechanics, and how Fly.io's orchestrator actually works under the hood. Then I dove into research mode.

I'll be transparent here - this took me more than a couple of hours. I got sidetracked reading Fly.io's engineering blogs (even found fascinating post about using Trivy for security scans and SBOM generation), watching fly.io YouTube videos, and browsing community posts. The content quality is honestly great, and I found myself going deeper than strictly necessary because I was genuinely interested in understanding the broader context of what fly.io is building.

I did face some problems running DeviceMapper on my MacBook turned into quite the debugging adventure. After some time troubleshooting, I ended up using Lima (Linux VMs on macOS) to properly test device creation and snapshots. This extra work paid off - it forced me to really understand what was happening at the kernel level, not just copy commands from the web.

During development, I had to actively monitor myself to avoid over-engineering. I started planning a daemon mode to mimic a long-running flyd process, complete with job queues and HTTP APIs. But I caught myself and refocused on the core requirements. The ability to recognize when to stop and ship is something that I'm strong at.

## TLDR - Solution Overview

### What I Built

A Go-based system that models Fly.io's flyd orchestrator core functionality:
- **FSM-driven workflow** orchestrating image lifecycle from S3 to activated snapshot
- **Idempotent operations** ensuring safe repeated executions
- **Security-first validation** protecting against malicious images
- **DeviceMapper integration** for efficient thin provisioning
- **SQLite state tracking** maintaining image registry

### Package Architecture

```
pkg/
├── fsm/          # State machine orchestration (check_db -> download -> validate -> create_device -> complete)
├── db/           # SQLite persistence layer with idempotency guarantees
├── storage/      # S3 client for image retrieval
├── security/     # Validation layer (compression bombs, path traversal, symlinks)
├── devicemapper/ # Linux thin provisioning and snapshot management
cmd/
└── flyio-machine/ # CLI commands (fetch-and-create, list, health)
```

### Running the Solution

**macOS Testing (without DeviceMapper):**
```bash
./test-cli-macos.sh
```

**Full Linux Testing (with DeviceMapper):**
```bash
./test-cli-lima.sh  # Automatically sets up Lima VM and runs E2E tests
```

### E2E Workflow - Requirement Mapping

Here's how a complete image fetch satisfies each requirement:

```
Command: ./flyio-machine fetch-and-create images/golang/2.tar

[FSM Library Integration]
State Machine initialized with BoltDB persistence
Transitions: check_db -> download -> validate -> create_device -> complete

[Idempotency Check - SQLite Tracking]
-> check_db: Query images table for 'images/golang/2.tar'
  Result: Not found
  Action: Create record with status='pending'

[S3 Download - Only If Not Retrieved]
-> download: Fetch from s3://flyio-platform-hiring-challenge/images/golang/2.tar
  Downloaded: 47MB -> /tmp/flyio-machine/downloads/2.tar
  SHA256: Computed for integrity verification (even tough not used because S3 objects did not have the expected SHA256 to validate)

[Security Validation & Canonical Layout]
-> validate: Extract tarball with security checks
  - Path traversal: Blocked (no ../ escapes)
  - Symlinks: Validated depth (no root escapes)
  - Compression ratio: Checked (no zip bombs)
  Result: Extracted to canonical filesystem layout

[DeviceMapper Thinpool Device]
-> create_device: Setup thin provisioned device
  - Create thin device ID=1 in pool
  - Activate: /dev/mapper/flyio-1
  - Format: ext4 filesystem
  - Mount and copy extracted files

[Snapshot Activation]
-> complete: Create snapshot for activation
  - Snapshot ID=1000 from base device ID=1
  - Activated: /dev/mapper/flyio-snapshot-1000
  - Status: ready

Final State:
- Image tracked in SQLite with snapshot_id
- DeviceMapper snapshot ready for container use
- Next identical request skips directly to 'ready' (idempotent)
```

## Technical Implementation Deep Dive

### CHECK_DB State - Idempotency Foundation

**Success Criteria:** Ensure images are processed exactly once, regardless of retries or concurrent requests.

**Design Decision:** Query-first approach with database as single source of truth. Every workflow starts by checking if we've seen this S3 key before.

**Trade-offs:**
- **Pro:** Rock-solid idempotency without distributed locks
- **Pro:** Simple to reason about and debug
- **Con:** Extra database query on every request

**Implementation:** The state queries SQLite by indexed S3 key. If status is 'ready', we skip everything and return immediately. This handles both retries and legitimate re-requests efficiently.

### DOWNLOAD State - S3 Integration

**Success Criteria:** Retrieve images from S3 efficiently with integrity verification.

**Design Decision:** Stream download with concurrent SHA256 computation. Store in local cache directory for subsequent operations.

**Trade-offs:**
- **Pro:** Single pass for download and hash computation
- **Pro:** Local cache enables debugging and manual inspection
- **Con:** Disk space usage (mitigated by cleanup in production)

**Why This Approach:** Computing SHA256 during streaming avoids reading the file twice. This matters for large images where I/O is the bottleneck.

### VALIDATE State - Security First

**Success Criteria:** Protect against malicious images while preserving legitimate container filesystem structures.

**Design Decision:** Multi-layer validation during extraction rather than post-extraction scanning.

**Key Validations:**
1. **Path Traversal:** Reject any attempts to escape extraction directory
2. **Symlink Depth:** Calculate relative depth to prevent escaping root
3. **Size Limits:** Both per-file and total to prevent resource exhaustion
4. **Compression Ratio:** Detect zip bombs before they detonate

**Trade-offs:**
- **Pro:** Catches threats during extraction, not after damage is done
- **Pro:** Granular control over what's allowed
- **Con:** Slightly slower extraction (worth it for security)

**Real Challenge:** Container images have tons of symlinks (like SSL certs pointing to actual certificates). The validation had to be smart enough to allow legitimate symlinks while blocking malicious ones. Solution: Calculate symlink resolution depth - legitimate links stay within their namespace, malicious ones try to escape upward.

### CREATE_DEVICE State - DeviceMapper Magic

**Success Criteria:** Create thin-provisioned block devices for efficient storage.

**Design Decision:** Direct DeviceMapper commands via dmsetup for maximum control.

**Implementation Flow:**
1. Delete existing device (idempotency)
2. Create thin metadata in pool
3. Activate thin device
4. Format with ext4
5. Mount and populate from extracted files
6. Unmount (device persists)

**Trade-offs:**
- **Pro:** True copy-on-write (CoW pattern) semantics
- **Pro:** Snapshots are instant regardless of image size
- **Con:** Requires root and Linux kernel support

**Platform Consideration:** On macOS, DeviceMapper calls return early with a platform message. The system gracefully degrades while maintaining the same workflow structure.

### COMPLETE State - Snapshot Activation

**Success Criteria:** Create instantly activatable snapshots from base devices.

**Snapshot Creation Process:**
1. Allocate device ID from unified sequence (`device_sequence` table)
2. Create snapshot using `dmsetup message /dev/mapper/pool 0 "create_snap <snapshot_id> <base_device_id>"`
3. Store snapshot_id in SQLite for tracking
4. Mark image as ready for activation

**Design Decision:** Unified sequential device ID allocation for both base devices and snapshots from a single sequence table with transaction-safe operations.

**Trade-offs:**
- **Pro:** Single allocation method simplifies code and reasoning
- **Pro:** Transaction-safe prevents race conditions in concurrent scenarios
- **Pro:** Sequential IDs (1, 2, 3, 4...) enable easy debugging and monitoring
- **Con:** Requires dedicated sequence table rather than reusing AUTO_INCREMENT

**Why This Approach:** DeviceMapper enforces unique IDs within its 24-bit namespace—snapshot IDs cannot match their origin device IDs. Unified sequential allocation guarantees collision avoidance while keeping allocation logic simple and maintainable.

## Extra Miles - What I Didn't Build (On Purpose)

I started developing a daemon mode - a long-running HTTP server with job queues, thinking it would better demonstrate production patterns. After getting the basic structure working, I made the conscious decision to stop.

Why? The CLI mode already proves all the core concepts: FSM orchestration, idempotency, security validation, and DeviceMapper integration. Adding daemon mode would be engineering for the sake of engineering, not for demonstrating competency. Knowing when to stop and ship is as important as knowing how to build.

That said, the architecture is designed to easily support daemon mode if needed - the FSM manager and database layers are already service-agnostic.

## Closing Thoughts

This challenge reminded me why I chose software engineering. Starting with something I'd only heard about (DeviceMapper), diving deep into research, and then successfully implementing a working solution - that's the cycle that keeps this field interesting after all these years.

The process of studying Fly.io's architecture, debugging kernel-level device mapper operations, and making security trade-offs in real code was genuinely enjoyable. Even the frustrating parts (looking at you, macOS DeviceMapper compatibility) taught me something valuable.

What I appreciate most about this challenge is that it mirrors real engineering: unclear requirements that need research, platform-specific edge cases that need handling, and the constant question of "when is this good enough to ship?"

Thanks for the interesting problem. Whether or not this leads to the next conversation, I learned something valuable building it.

---

**Built by:** Leonardo Meireles. 

**Time Investment:** More than a couple hours (but it did not seem like it). 

**Key Learning:** Sometimes the best code is the code you decide not to write. 