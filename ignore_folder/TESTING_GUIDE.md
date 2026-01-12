# Testing Guide - Fly.io Platform Machines Challenge

## Quick Verification (5 minutes)

### Step 1: Verify Build
```bash
go build -o flyio-machine ./cmd/flyio-machine
./flyio-machine --help
```

**Expected Output**: Should show 6 commands including `cleanup`

### Step 2: Run Unit Tests
```bash
go test ./pkg/fsm ./pkg/security ./pkg/db -v
```

**Expected Output**: All 11 tests should pass

### Step 3: Run Lima E2E Tests
```bash
./test-lima.sh
```

**Expected Output**: 5 E2E tests pass, including snapshot validation

---

## Detailed Testing

### Test 1: Unit Tests (No privileged mode required)
```bash
# Run all unit tests with coverage
go test ./pkg/fsm ./pkg/security ./pkg/db -v -cover

# Expected Results:
# - pkg/fsm: 4/4 tests passing (snapshot logic, cleanup, accumulation)
# - pkg/security: 4/4 tests passing (path traversal, size limits)
# - pkg/db: 3/3 tests passing (CRUD operations)
# - Total: 11/11 tests passing
```

**What's Tested**:
- ✅ Mandatory snapshot behavior (aborts on failure)
- ✅ Graceful degradation for non-Linux
- ✅ Automatic cleanup logic
- ✅ Response accumulation across FSM states
- ✅ Security validation (path traversal, compression bombs)
- ✅ Database operations (CRUD, state tracking)

### Test 2: Local Build (macOS/non-Linux)
```bash
# Build binary
go build -o flyio-machine ./cmd/flyio-machine

# Test basic workflow (without devicemapper)
./flyio-machine fetch-and-create images/golang/2.tar

# Expected Output:
# ⚠️  Devicemapper unavailable: devicemapper not supported on darwin
# Started FSM: <ulid>
# Status: ready OR vulnerable
# Vulnerabilities: X (Critical: Y, High: Z)
```

**What's Tested**:
- ✅ S3 download with SHA256 verification
- ✅ Secure tarball extraction
- ✅ Trivy vulnerability scanning
- ✅ SQLite state tracking
- ✅ Graceful degradation without devicemapper

### Test 3: Lima E2E (Full Linux Testing)
```bash
# Run full E2E test suite
./test-lima.sh

# Or run interactively
lima -n flyio-test
# Inside Lima VM:
cd /tmp/lima
go build -o flyio-machine ./cmd/flyio-machine
# Run tests manually
```

**What's Tested**:
- ✅ Full FSM workflow (download → extract → scan → device → snapshot)
- ✅ Idempotency (skip already-downloaded images)
- ✅ DeviceMapper device creation
- ✅ **Snapshot creation (MANDATORY requirement)**
- ✅ Cleanup command functionality
- ✅ Database state verification

**Expected Final Output**:
```
═══════════════════════════════════════════════
✅ All E2E Tests Passed!
═══════════════════════════════════════════════

Test Results:
  ✅ Full FSM workflow (download → extract → scan → device → snapshot)
  ✅ Idempotency (skip already-downloaded images)
  ✅ List command
  ✅ Cleanup mechanism
  ✅ Snapshot creation (MANDATORY requirement)

Challenge Requirements Met:
  ✅ FSM library integration
  ✅ S3 download with idempotency
  ✅ Tarball extraction with security validation
  ✅ DeviceMapper thinpool device creation
  ✅ Snapshot creation for activation
  ✅ SQLite state tracking

🎉 E2E Test Suite Complete!
```

### Test 4: Cleanup Command
```bash
# Inside Lima VM (or on Linux with devicemapper)
lima -n flyio-test
cd /tmp/lima

./flyio-machine fetch-and-create images/golang/2.tar
./flyio-machine list  # Should show image with snapshot ID

# Test cleanup
./flyio-machine cleanup --image images/golang/2.tar

# Verify cleanup
./flyio-machine list  # Should show status: cleaned
sudo dmsetup ls | grep flyio  # Should show no devices
ls -la /tmp/flyio-machine/extracted/  # Should be empty
```

**What's Tested**:
- ✅ Unmount and delete snapshots
- ✅ Unmount and delete base devices
- ✅ Remove extracted files
- ✅ Update database status

---

## Verification Checklist

Before submitting, verify:

### ✅ Build & Compile
- [ ] `go build -o flyio-machine ./cmd/flyio-machine` succeeds
- [ ] `./flyio-machine --help` shows 6 commands including `cleanup`
- [ ] No compilation errors or warnings

### ✅ Unit Tests
- [ ] `go test ./pkg/fsm -v` - 4/4 tests pass
- [ ] `go test ./pkg/security -v` - 4/4 tests pass
- [ ] `go test ./pkg/db -v` - 3/3 tests pass

### ✅ Lima E2E Tests
- [ ] `./test-lima.sh` completes successfully
- [ ] All 5 E2E tests pass
- [ ] Snapshot creation validation passes
- [ ] No errors in test output

### ✅ Challenge Requirements
- [ ] FSM library orchestration (working)
- [ ] S3 download with idempotency (tested)
- [ ] Tarball extraction with security (tested)
- [ ] DeviceMapper thinpool device (E2E tested)
- [ ] **Snapshot creation (MANDATORY - E2E verified)**
- [ ] SQLite state tracking (tested)

---

## Troubleshooting

### Issue: Lima E2E tests fail
**Solution**:
```bash
# Check Lima status
limactl list

# Restart Lima VM
limactl stop flyio-test
limactl start flyio-test

# Check if Lima is installed
brew install lima
```

### Issue: "Thinpool setup failed"
**Check**:
```bash
# Inside Lima VM
lima -n flyio-test
lsmod | grep dm_thin_pool  # Should show devicemapper module
sudo dmsetup info pool  # Should show pool device
```

### Issue: Unit tests fail
**Check**:
```bash
go mod tidy  # Ensure dependencies are correct
go clean -testcache  # Clear test cache
go test ./pkg/fsm -v  # Run with verbose output
```

### Issue: Build fails
**Check**:
```bash
go version  # Should be Go 1.21+
go mod download  # Re-download dependencies
```

---

## Performance Expectations

### Local Build (macOS)
- Build time: ~5-10 seconds
- Test time: <1 second
- Workflow (no DM): ~30-60 seconds per image

### Docker E2E (Linux)
- Image build: ~2-3 minutes (first time)
- Thinpool setup: ~5 seconds
- Full test suite: ~2-4 minutes
- Per-image workflow: ~30-90 seconds

---

## CI/CD Integration

### GitHub Actions Example
```yaml
name: E2E Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Run E2E Tests
        run: docker-compose up --abort-on-container-exit
```

### GitLab CI Example
```yaml
e2e:
  image: docker:latest
  services:
    - docker:dind
  script:
    - docker-compose up --abort-on-container-exit
```

---

## Success Criteria

Your implementation passes if:

✅ All unit tests pass (11/11)
✅ Docker E2E tests pass (5/5)
✅ Build succeeds without errors
✅ Snapshot creation is verified in database
✅ Cleanup command works correctly
✅ All 8 README requirements are met

---

## Questions?

See documentation:
- `IMPLEMENTATION_COMPLETE.md` - Full implementation details
- `test/e2e/README.md` - Docker testing guide
- `IMPLEMENTATION_STATUS.md` - Original analysis
- `README.md` - Challenge requirements
