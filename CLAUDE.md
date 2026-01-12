# Fly.io Platform Machines Hiring Challenge

## 🎯 Project Purpose

This repository contains a solution for the **Fly.io Platform Machines Work Sample** - a challenging technical assessment that models the core functionality of Fly.io's `flyd` orchestrator.

## 📋 Challenge Overview

Build a Go-based system that:

1. **Uses FSMv2 Library** (`github.com/superfly/fsm`) for state machine orchestration
2. **Retrieves Container Images** from S3 bucket: `s3://flyio-platform-hiring-challenge/images`
3. **Tracks Images with SQLite** - persistent storage for image state
4. **Implements Idempotency** - only download/unpack if not already done
5. **Uses DeviceMapper Thinpool** - unpack images into canonical filesystem layout
6. **Creates Snapshots** - activate images by creating thinpool device snapshots

## 🏗️ Architecture Components

### FSMv2 (State Machine)
- Core orchestration library from Fly.io
- Manages state transitions and workflows
- Located at: https://github.com/superfly/fsm
- **Action Required**: Study this library to understand its API and usage patterns

### Container Images (S3)
- Location: `s3://flyio-platform-hiring-challenge/images`
- Format: Ad-hoc tarball format (non-standard)
- **Security Note**: Assume hostile environment - validate all blobs before processing
- **Critical**: Think about what these blobs mean (not just arbitrary files)

### SQLite Database
- Track available images
- Maintain state for idempotency
- Store image metadata and status

### DeviceMapper Thinpool
- Kernel-level block device management
- Thin provisioning for efficient storage
- Setup example provided in README:
```bash
fallocate -l 1M pool_meta
fallocate -l 2G pool_data

METADATA_DEV="$(losetup -f --show pool_meta)"
DATA_DEV="$(losetup -f --show pool_data)"

dmsetup create --verifyudev pool --table "0 4194304 thin-pool ${METADATA_DEV} ${DATA_DEV} 2048 32768"
```

## 🔒 Security Considerations

**Critical Requirements**:
1. **Hostile Environment**: Code will run on fleet with thousands of servers
2. **Blob Validation**: Don't trust S3 blobs - validate before processing
3. **Unknown Blobs**: Code must handle blobs not currently on S3
4. **Input Sanitization**: Prevent injection attacks, path traversal, etc.
5. **Resource Limits**: Prevent DoS via resource exhaustion

## 📊 Current State

### Existing Files
- `go.mod` / `go.sum` - Go module dependencies
- `s3_trivy_detective.go` - Trivy scanner integration (security scanning)
- `trivy_scanner.go` - Security vulnerability scanning
- `.gitignore` - Git ignore patterns

### Development Status
- ⏳ Initial setup phase
- 🔍 Research FSMv2 library usage
- 📝 Design system architecture
- 💻 Implementation pending

## 🎓 Key Concepts to Understand

### 1. FSMv2 State Machine
- Event-driven state transitions
- How to define states and transitions
- Error handling and rollback mechanisms

### 2. Container Image Format
- Tarball structure and layers
- Manifest files and metadata
- Canonical filesystem layout expectations

### 3. DeviceMapper Thinpool
- Thin provisioning concepts
- Snapshot creation and management
- Block device operations

### 4. Idempotency Patterns
- Check-before-action pattern
- State tracking in SQLite
- Avoiding duplicate work

## 🚀 Implementation Strategy

### Phase 1: Research & Design
1. Study FSMv2 library API and examples
2. Understand S3 bucket structure and image format
3. Design state machine transitions
4. Plan SQLite schema for image tracking

### Phase 2: Core Implementation
1. S3 download functionality with validation
2. SQLite integration for state tracking
3. FSM state machine implementation
4. DeviceMapper thinpool integration

### Phase 3: Security & Validation
1. Input validation and sanitization
2. Error handling and recovery
3. Resource limit enforcement
4. Security testing

### Phase 4: Testing & Refinement
1. Test with images from S3 bucket
2. Verify idempotency behavior
3. Test snapshot creation
4. Performance optimization

## ⚠️ Important Notes

1. **No Test Evaluation**: Tests won't be evaluated, but quality code matters
2. **Time Estimate**: Should take hours, not days
3. **LLM Assistance**: Can use LLMs, but bar is higher than typical LLM output
4. **Golang Core**: Main tool must be in Go (can call shell scripts)
5. **Minimal Help**: Limited guidance available - figure things out independently

## 🎯 Success Criteria

- ✅ Uses FSMv2 for orchestration
- ✅ Downloads images from S3 only when needed
- ✅ Unpacks images into devicemapper thinpool
- ✅ Creates snapshots for activation
- ✅ Tracks state with SQLite
- ✅ Handles security concerns appropriately
- ✅ Idempotent operations (safe to run multiple times)

## 📚 Resources

- FSMv2 Library: https://github.com/superfly/fsm
- S3 Bucket: `s3://flyio-platform-hiring-challenge/images`
- DeviceMapper: Linux kernel documentation
- Container Format: Research OCI/Docker image formats

## 🤔 Questions to Explore

1. What is the FSMv2 API structure and usage pattern?
2. What format are the S3 blobs in? (OCI, Docker, custom?)
3. What metadata is needed in SQLite?
4. How should state transitions be modeled?
5. What is a "canonical filesystem layout" for containers?
6. How to efficiently check if image already unpacked?
7. What security validations are critical?

## 💡 Next Steps

1. **Study FSMv2**: Clone and explore the library
2. **Inspect S3**: Download sample image and analyze format
3. **Design Schema**: Plan SQLite tables and FSM states
4. **Prototype**: Start with minimal viable implementation
5. **Iterate**: Add features and security incrementally

---

**Remember**: This is a model of production orchestrator code. Think about reliability, security, and edge cases throughout implementation.
