# Architecture Documentation

## Overview

This document explains the key architectural decisions made in implementing the Fly.io Platform Machines work sample.

## Design Decisions

### 1. Daemon Mode: HTTP REST API vs Unix Sockets

**Decision**: Implemented HTTP REST API instead of Unix sockets for daemon mode.

**Rationale**:
- **Wider compatibility**: HTTP works across macOS, Linux, and can be tested remotely
- **Tooling ecosystem**: Standard tools (curl, httpie, Postman) work out of the box
- **Service discovery**: Easier to integrate with load balancers, reverse proxies
- **Language agnostic**: Any client can interact with HTTP (JavaScript, Python, etc.)
- **Modern conventions**: REST APIs are the standard for daemon services
- **Debugging**: HTTP is easier to debug with browser dev tools, network inspectors

**Trade-offs**:
- Unix sockets have lower latency (microseconds vs milliseconds)
- Unix sockets have better security isolation (filesystem permissions)
- HTTP has more overhead (TCP, headers, marshaling)

**Conclusion**: For a platform orchestrator that may need to integrate with multiple services, HTTP provides better flexibility and ecosystem support. The latency difference is negligible for image processing workflows that take seconds/minutes.

### 2. Sequential Job Processing in Daemon Mode

**Decision**: Implemented single-worker queue with sequential processing (one job at a time).

**Rationale**:
- **Resource safety**: Image processing is resource-intensive (download, decompression, DeviceMapper operations)
- **DeviceMapper safety**: Thinpool operations should not be concurrent without careful locking
- **Database contention**: Sequential access reduces SQLite lock contention
- **Predictable behavior**: Easier to reason about state and debug issues
- **Production pattern**: Matches how flyd likely operates (one activation at a time per machine)

**Future enhancements**:
- Could add `--concurrency N` flag for parallel processing
- Could implement smart scheduling based on resource availability
- Could separate download/validation from DeviceMapper operations

### 3. Separate FSM Databases for CLI and Daemon

**Decision**: CLI mode uses `--fsm-db-path` (default `./fsm.db`), daemon mode uses `--daemon-fsm-path` (default `./fsm_daemon`).

**Rationale**:
- **BoltDB locking**: FSM library uses BoltDB which has file-level locking
- **Concurrent usage**: Prevents conflicts when running both CLI and daemon simultaneously
- **State isolation**: CLI operations don't interfere with daemon queue state
- **Clear separation**: Different database clearly indicates different operational contexts

**Implementation**:
- CLI: Single-shot FSM execution, database closed after completion
- Daemon: Long-lived FSM Manager, database kept open for the daemon's lifetime
- Both share the same SQLite database for image state tracking

### 4. Logging Strategy: Structured Logging with slog

**Decision**: Used Go's standard `log/slog` for all logging instead of third-party libraries.

**Rationale**:
- **Standard library**: No external dependencies, guaranteed compatibility
- **Structured logging**: Key-value pairs make logs machine-readable
- **Performance**: Optimized for high-throughput logging
- **Consistency**: Single logging interface across all packages

**Log message format**:
```
time=<timestamp> level=<LEVEL> msg=<event_name> key1=value1 key2=value2
```

**Key naming conventions**:
- `*_init`: Initialization events
- `*_start`: Operation started
- `*_complete`: Operation completed successfully
- `*_failed`: Operation failed with error
- Underscore-separated for machine parsing (e.g., `job_processing_start`)

### 5. Security Validation Approach

**Decision**: Multi-layered security validation before processing any image.

**Layers**:
1. **Input validation**: S3 key format, path traversal checks
2. **Size limits**: Pre-download size checks, extraction size limits
3. **Compression ratio**: Detect compression bombs (10:1 default)
4. **Path sanitization**: Prevent directory traversal during extraction
5. **Resource limits**: Configurable max file and total size limits

**Rationale**:
- **Hostile environment**: Challenge explicitly mentions running in hostile environments
- **Defense in depth**: Multiple layers prevent bypass
- **Configurable limits**: Operators can tune for their environment
- **Fail-safe**: Reject suspicious content rather than attempting to process

### 6. Error Handling and Retry Strategy

**Decision**: Implemented automatic retry with exponential backoff in FSM states.

**Retry logic**:
- Network errors: Retry up to 3 times with exponential backoff
- Transient failures: Automatic retry (S3 timeouts, temporary DeviceMapper issues)
- Permanent failures: Fail immediately (invalid image format, security violations)

**FSM error handling**:
- Each state returns clear error messages
- Errors are logged with context (job_id, s3_key, state)
- Failed jobs remain in database for debugging
- Status transitions tracked in database (pending → downloading → ready/failed)

### 7. Database Schema Design

**Decision**: Minimal SQLite schema focused on state tracking.

**Schema**:
```sql
CREATE TABLE images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    s3_key TEXT UNIQUE NOT NULL,
    status TEXT NOT NULL,
    device_path TEXT,
    snapshot_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**Rationale**:
- **Idempotency**: `s3_key` unique constraint prevents duplicate processing
- **State tracking**: `status` column tracks image lifecycle
- **DeviceMapper integration**: `device_path` and `snapshot_id` store activation details
- **Audit trail**: Timestamps for debugging and monitoring

**Status values**:
- `pending`: Initial state
- `downloading`: S3 download in progress
- `validating`: Security validation in progress
- `creating_device`: DeviceMapper snapshot creation
- `ready`: Image available for use
- `failed`: Processing failed (see logs for details)

### 8. Testing Strategy

**Decision**: Platform-specific test scripts instead of Go tests.

**Test files**:
- `test-cli-macos.sh`: CLI mode on macOS (DeviceMapper stub)
- `test-cli-lima.sh`: CLI mode on Lima/Linux (full DeviceMapper)
- `test-daemon-macos.sh`: Daemon mode on macOS (API testing)
- `test-daemon-lima.sh`: Daemon mode on Lima/Linux (full integration)

**Rationale**:
- **Challenge requirement**: "We will not evaluate your tests"
- **Platform differences**: DeviceMapper only works on Linux
- **Integration focus**: Test full workflows, not unit tests
- **Shell scripts**: Easy to run, understand, and modify
- **Real environment**: Tests use actual S3 bucket, real DeviceMapper

### 9. DeviceMapper Integration

**Decision**: Graceful degradation on non-Linux platforms.

**Implementation**:
- Linux: Full DeviceMapper support with thinpool operations
- macOS: Stub implementation that logs warnings
- Check at runtime: `runtime.GOOS == "linux"`

**DeviceMapper operations**:
1. Create manager with pool name and sizes
2. Create thin volume for image
3. Mount and unpack image to volume
4. Create snapshot for activation
5. Return device path and snapshot ID

**Error handling**:
- Missing DeviceMapper: Log warning, continue without snapshots
- Operation failures: Retry once, then fail the job
- Pool full: Clear error message with remediation steps

### 10. Configuration Management

**Decision**: Cobra for CLI + Viper for configuration management.

**Benefits**:
- **Hierarchical flags**: Global flags and command-specific flags
- **Environment variables**: Auto-bind to FLYIO_* env vars
- **Config files**: Can load from YAML/JSON (not implemented but supported)
- **Validation**: Built-in type checking and validation

**Flag organization**:
- Global flags: Database paths, S3 config, security limits
- Command flags: Port for daemon, specific operation parameters
- Sensible defaults: Work out of the box for development

### 11. FSM State Machine Design

**Decision**: Linear state progression with error handling.

**State flow**:
```
check_db → download → validate → create_device → complete
                ↓         ↓           ↓
              failed ← failed ← failed
```

**State transitions**:
- **check_db**: Query database, create if not exists, transition to download/complete
- **download**: S3 download with retry, transition to validate
- **validate**: Security checks, transition to create_device
- **create_device**: DeviceMapper operations, transition to complete
- **complete**: Update database to ready status, end workflow
- **failed**: Record error, retry or terminate

**Retry mechanism**:
- Built into FSM library
- Configurable retry count (default: 3)
- Exponential backoff between retries
- State-specific retry policies

## Future Improvements

1. **Concurrent processing**: Add worker pool for parallel job processing
2. **Metrics**: Prometheus metrics for monitoring
3. **Graceful reload**: SIGHUP to reload configuration
4. **Job prioritization**: Priority queue for critical images
5. **Webhook notifications**: POST to URL on job completion
6. **Image garbage collection**: Automated cleanup of old images
7. **Distributed locking**: Redis/etcd for multi-instance coordination
8. **Image verification**: GPG signature validation
9. **Partial downloads**: Resume interrupted S3 downloads
10. **Streaming extraction**: Extract while downloading for faster processing

## Performance Considerations

### Bottlenecks
1. **S3 download**: Network bandwidth limited (mitigated by validation before download)
2. **Decompression**: CPU intensive (could parallelize with pgzip)
3. **DeviceMapper**: I/O intensive (sequential processing helps)
4. **Database**: SQLite lock contention (WAL mode could help)

### Optimizations Applied
1. **Idempotency checks**: Skip already-processed images
2. **Resource limits**: Prevent memory exhaustion
3. **Structured logging**: Minimal performance overhead
4. **Single worker**: Prevents resource contention

## Security Considerations

### Threat Model
- **Malicious images**: Compression bombs, path traversal, oversized files
- **S3 bucket poisoning**: Unexpected blob formats
- **Resource exhaustion**: Memory/disk DoS attacks
- **Code injection**: Shell command injection via filenames

### Mitigations
- **Input validation**: Sanitize all S3 keys and file paths
- **Resource limits**: Hard caps on file sizes and extraction
- **Compression ratio checks**: Detect zip bombs
- **No shell execution**: Direct API calls, no command injection surface
- **Fail-safe defaults**: Reject suspicious content

## Conclusion

This architecture balances simplicity, security, and functionality. The design prioritizes:
1. **Safety**: Defensive programming for hostile environments
2. **Observability**: Comprehensive logging for debugging
3. **Flexibility**: HTTP API for wide integration
4. **Reliability**: Automatic retries and error handling
5. **Maintainability**: Clear code organization and documentation

The implementation successfully models flyd's core functionality while remaining practical for development and testing across multiple platforms.
