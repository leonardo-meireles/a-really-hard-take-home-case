
# Fly.io Platform Machines Challenge: Study & Implementation Guide

---

## 1. Digest of Questions and Clarifications

This section summarizes the questions asked and their clarified answers.

### Q1: What am I building?
A one-shot CLI tool (initially) that downloads container image tarballs from S3, verifies them, unpacks them into device-mapper thin volumes, tracks their state in SQLite, and coordinates everything via FSMs. Later, this will evolve into a long-running worker process.

### Q2: Will this run as a daemon or one-shot tool?
Initially one-shot CLI. Later refactored to a persistent worker mode.

### Q3: What exactly is the output supposed to be?
- Verified blob files downloaded from S3.
- Unpacked filesystem contents (whiteout-handled) into device-mapper volumes.
- Snapshotted thin volumes (via `dmsetup`).
- Recorded state in SQLite showing completed steps.

### Q4: Where should this be tested from?
macOS (M1) is the dev platform, but device-mapper support is Linux-only. You’ll test this via Lima VM with nested loop device and `dmsetup` support.

---

## 2. Study Guide (Concepts)

Each section includes beginner intro + infra/devops deeper view + caveats + resources.

### 2.1 Finite State Machines (FSM)

**What:** A way to model progress of a task as transitions between well-defined states.  
**Why:** Fly.io uses FSMs to break orchestration into atomic steps that can be retried/recovered.  
**How:** Each operation (image fetch, unpack, activate) has its own FSM with current state, retry logic, etc.  
**Gotchas:** FSMs must be idempotent. No implicit side effects allowed.

**Resources:**
- [superfly/fsm GitHub](https://github.com/superfly/fsm)
- [State Machines in Go](https://mt165.co.uk/blog/state-machine-go/)

---

### 2.2 S3 and Blob Validation

**What:** Blobs are binary objects stored in S3.  
**Why:** You must safely fetch these and ensure they weren’t corrupted or tampered.  
**How:** Use anonymous S3 GET/HEAD. Compute SHA256 and compare to metadata. Enforce max size.  
**Gotchas:** Don’t trust ETags or size alone. Stream to disk while hashing.

**Resources:**
- [AWS SDK for Go v2](https://docs.aws.amazon.com/sdk-for-go/api/)
- [Public S3 Access Pattern](https://gist.github.com/tehmoon/fcced113b7f3c501084b2ff07aef0039)

---

### 2.3 OCI Image Format and Whiteouts

**What:** Container image is a manifest + config + multiple tar layers.  
**Why:** You’ll unpack these into root filesystems.  
**How:** Tar files include whiteout markers like `.wh.filename` for deletions.  
**Gotchas:** Must correctly handle AUFS-style whiteouts, translate into overlay-style deletions.

**Resources:**
- [OCI Image Spec](https://github.com/opencontainers/image-spec)
- [Interpreting whiteout files](https://www.madebymikal.com/interpreting-whiteout-files-in-docker-image-layers/)

---

### 2.4 Device Mapper and Thin Provisioning

**What:** A Linux kernel system to create logical volumes over physical devices.  
**Why:** Used to create copy-on-write image activation snapshots efficiently.  
**How:** Create a thin pool (meta + data), then thin volumes + snapshots using `dmsetup`.  
**Gotchas:** Only works on Linux. Must use loopback devices in a VM.

**Resources:**
- [Kernel thin provisioning docs](https://www.kernel.org/doc/Documentation/device-mapper/thin-provisioning.txt)

---

### 2.5 SQLite and Concurrency

**What:** Lightweight embedded DB.  
**Why:** Used to track which image has been downloaded, unpacked, activated.  
**How:** Use WAL mode. Separate read-only and write-only connections.  
**Gotchas:** Only one concurrent writer. Use transactions carefully.

**Resources:**
- [SQLite WAL Mode](https://sqlite.org/wal.html)
- [go-sqlite3 concurrency issue #1179](https://github.com/mattn/go-sqlite3/issues/1179)

---

## 3. Implementation Plan (Phases)

You said:
1. let's do both but let's start with one shot cli tool, then after this is implemented and tested, an extra mile will be to migrate this to a persistent worker process -> but only after in the end after the one shot cli is working -> we will have the 2 modes
2. yes
3. i dont think arm64 is able to, please research a 100% confident way of me being able to test this in a linux vm (maybe lima)
4. do both, explain begginer way then go back to the devops familiarity basics (so ill read the explanation begginer then i will be able to follow the more advancced explanaitions)

Fly.io Platform Machines Challenge: Study & Implementation Guide
1. Digest of User Questions
  • What is the Fly.io FSM library (superfly/fsm)? – The Fly platform orchestrator (flyd) uses a custom Go-based Finite State Machine (FSM) library to manage machine lifecycles. Each worker runs flyd, which “durably tracks the steps in a series of finite state machines” for actions like creating or stopping a Fly Machine. The superfly/fsm library provides the state-machine framework under flyd. In other words, complex operations are broken into states/transitions that this library executes and persists.
  • How do I validate an S3 object? – In this challenge, S3 “objects” refer to container image blobs stored in an S3 bucket. AWS S3 supports checksums (MD5, SHA-, CRC32, etc.) to verify object integrity. When uploading or downloading, S3 can automatically check the checksum of each object. A common approach is to compare the object’s ETag* or request an explicit checksum value (MD5 or SHA-256) to ensure the downloaded data hasn’t been corrupted. In practice, you’d use the AWS SDK (e.g. Go’s AWS SDK v2) to fetch the object and verify it matches the expected checksum or handle upload confirmations.
  • What is the OCI image format, and how are layers applied? – OCI (Open Container Initiative) defines the standard container image format. An OCI image consists of a manifest (JSON), configuration JSON, and one or more layer blobs (usually tar archives) stored in a content-addressable store. Layers are applied sequentially “on top of each other to create a complete filesystem”. In effect, each layer tarball contains filesystem changes (file additions/updates and “whiteout” deletions) that modify the filesystem from the previous layer. Understanding this format is critical: your code will need to read the manifest, iterate layers in order, and extract or apply them correctly.
  • What are whiteout files and how do they work? – A “whiteout” is a special empty file used in container image layers to signal deletion of files/directories from lower layers. By OCI spec (adopted from Docker/AUFS), when a file is removed in a higher layer, the layer includes an empty file named .wh.<basename> in place of the deleted entry. For example, deleting /app/foo adds .wh.foo in the layer tar. When applying layers, the logic must check for any .wh.* files and remove the corresponding entry from the merged filesystem (and then hide the .wh file itself). Whiteouts ensure that a delete in a later layer truly removes the item from the combined filesystem.
  • How does SQLite handle concurrency? – SQLite is a file-based database with single-writer, multiple-reader locking. By default (rollback journal mode), only one writer can write at a time and it locks the entire DB file. However, in WAL (Write-Ahead Logging) mode, readers and writers do not block each other: “WAL provides more concurrency as readers do not block writers and a writer does not block readers”. In practice, this means you can have many concurrent SELECTs, but writes still serialize. For a Go program, it’s important to use WAL mode (PRAGMA journal_mode=WAL) if multiple processes or threads might write, and to handle “database is locked” errors or use busy timeouts. Without WAL, long transactions or simultaneous writes will fail with lock errors.
  • What is device-mapper thin provisioning? – Device-mapper is a Linux kernel subsystem for abstracting block devices. Thin provisioning is a feature where virtual block volumes can be created that allocate space on-demand from an underlying pool. In thin provisioning, many “thin” devices share one data volume and metadata volume, so storage is overcommitted similarly to virtual memory. A thin pool allows blocks to be allocated to each thin volume only when actually written, saving space. Practically, you create a thin pool device backed by a data file (and a metadata file) using dmsetup, then create thin volumes (and snapshots) within that pool. This is often used by Docker’s devicemapper driver in “direct-lvm” mode. For this challenge, thin provisioning lets you apply layered image changes at the block level (e.g. creating a snapshot for each new container image).
  • How can I test device-mapper on macOS/ARM64? – Since macOS does not natively include Linux device-mapper or loop devices, you must run a Linux VM. A common solution is Lima (“Linux on Mac”). For example, install Lima (brew install lima), then run limactl start default and lima to get a shell in a Linux VM. Inside that VM (Ubuntu by default), install LVM or dmsetup (apt-get install lvm2 or device-mapper-persistent-data) and test dmsetup create, dmsetup ls, etc. Alternatively, use UTM or VirtualBox to run an x86_64 or aarch64 Linux ISO. In any case, ensure the Linux VM has /dev/loop and /dev/mapper enabled so dmsetup works.
2. Study Guide with Conceptual Explanations
Fly.io FSM Library (superfly/fsm)
  • What it is: The Fly.io FSM library is an internal Go package (github.com/superfly/fsm) that implements finite state machine logic. It underpins the flyd orchestrator, letting it sequence multi-step operations on Machines (e.g. creation, deletion, migration) in a reliable way.
  • Why it matters: The challenge likely expects you to model a task as a series of states (like “downloading image”, “unpacking”, “mounting device”, etc.) so that it can recover from failures and resume where it left off. Using an FSM ensures each step is atomic and tracked.
  • Beginner perspective: Imagine a simple workflow “STEP1 ➔ STEP2 ➔ ...”. An FSM library lets you define these steps (states) and transitions between them. If something fails mid-way, the FSM can retry or roll back. For example, when creating a machine, steps might be “allocate volume”, “load image”, “start machine”. Each is a state, and the library keeps track of which has completed.
  • DevOps detail: Under the hood, flyd treats each machine operation as a transaction log of states, stored durably (often in an embedded DB). The FSM library provides primitives like “Enter(state)”, “Exit(state)”, timeouts, and concurrency control. For complex infrastructure tasks (snapshotting a volume, migrating a VM), using FSM prevents half-applied changes. When you implement the challenge, you can use the same library (import superfly/fsm) or mimic its pattern to ensure idempotence.
  • Caveats: The Fly FSM library is undocumented; you’ll need to read its API or examples. Be careful: improper use can deadlock or leave a Machine in an inconsistent state. Tests and timeouts are important. Also, an FSM stores state in a key-value store, so you’ll need to set up a Store (often using SQLite or Badger). The library expects that each state transition is durable.
  • Resources: The Fly company blog and infra-log mention FSM concepts. (For example, Fly describes flyd as tracking steps in FSMs.) Also see the flyd Operator Sim article for high-level insight on how Fly uses FSMs.
AWS S3 Object Validation
  • What it is: In AWS S3, each uploaded object can have a checksum (ETag or explicit checksums like MD5, SHA256, etc.) that clients can use to verify data integrity. The challenge’s S3 object validation likely means checking that the downloaded object (an OCI image tar) is complete and uncorrupted.
  • Why it matters: Without validation, a partially downloaded or corrupted image could produce mysterious failures later (wrong files, extraction errors, etc.). Ensuring integrity early helps detect network or storage issues.
  • Beginner perspective: Think of S3 like a big file cabinet. Each file (object) has a unique fingerprint (ETag). After you download, you compare fingerprints (or use a known checksum) to confirm it’s the same. In code, you might fetch the object’s metadata and verify its ETag (for a single PUT, the ETag is often the MD5) matches an expected value, or you recalc MD5 of the downloaded bytes.
  • DevOps detail: S3 now supports multiple checksum algorithms (MD5, SHA-1, SHA-256, CRC32C, CRC64). For multipart uploads, ETag is a composite MD5 which is harder to verify manually. In Go, using the AWS SDK’s GetObject includes the ETag in the response. If the image was uploaded as a single part with MD5, you can compare MD5. Otherwise, you might rely on S3’s server-side validation (AWS automatically checks on upload/download). For strict validation, you can use S3’s “ComputeChecksum” operation or compare file size + ETag. Also consider using Content-MD5 headers on upload to force validation.
  • Caveats: ETags are not reliable MD5 for multipart uploads (their format changes). Also S3’s eventual consistency on listing might delay object visibility. In Go, always read the full GetObjectOutput.Body to ensure errors (like checksum mismatch) surface. Handle errors like InvalidChecksumException. Ensure network retries as S3 downloads can be interrupted.
  • Resources: AWS S3 Documentation “Checking object integrity in Amazon S3” explains checksum usage. A StackOverflow or AWS blog often explains ETags vs checksums. Also the AWS SDK Go docs show how to retrieve and compare checksums.
OCI Container Image Format
  • What it is: OCI images are Docker-compatible container images defined by the Open Container Initiative. An image consists of:
  • An image index (for multi-architecture images),
  • A manifest (JSON describing layers and config),
  • A config (JSON with metadata), and
  • One or more layer blobs (tar archives of filesystem diffs).Each layer tar contains files to add or modify. Layers are applied in order to build the final filesystem.
  • Why it matters: The challenge likely involves fetching an OCI image tarball (perhaps stored in S3 as blobs) and unpacking it. You need to interpret the manifest/config and correctly apply each layer to assemble the root filesystem. Understanding how layers stack is key, especially handling file additions vs. deletions (see whiteouts below).
  • Beginner perspective: Imagine an image like a zip file for a container’s filesystem. The manifest lists layer1.tar, layer2.tar, etc. You unzip layer1 into an empty folder, then apply layer2 on top (overwriting or adding files), and so on. After all layers, the folder represents the final disk contents.
  • DevOps detail: OCI uses a content-addressable layout: layer files are identified by sha256 digest. Tools like skopeo and umoci use this format. In Go, you can parse the manifest (in /manifests or index.json) to get the MediaType and Digest of each layer, then download each blob by digest. Each layer’s media type (e.g. application/vnd.oci.image.layer.v1.tar) indicates it’s a tar stream (often gzip-compressed as .tar.gz). According to the OCI spec, layers must be extracted as tar archives. Often layers come inside an overall tar (like a Docker “export”). The spec forbids duplicate entries in one layer tar.
  • Caveats: Tar format differences matter. As Cyphar notes, using tar for layers means whiteout files get special handling. Some tools compress layers with gzip or zstd; your code should handle that (go’s compress/gzip or compress/zstd). Watch out for symlinks and file permissions in the tar. The tar may contain an oci-layout file and blobs/sha256/<hash> directories if using an OCI layout on disk. If your input is a single compressed tar (not an OCI layout dir), you may need to untar it and read index.json. Also, some images use foreign layers (nondistributable blobs), but likely not in a coding challenge.
  • Resources: The OCI Image Spec (v1.0.0) defines layers (search “layer.v1.tar” for details). The Cyphar blog “Road to OCIv2” explains container image internals well. The go-containerregistry library (Google) has examples of reading OCI images in Go. For simpler reading, the standard archive/tar can extract a layer given a stream.
Whiteout Files in Overlay Filesystems
  • What it is: A whiteout file is an empty file with a special name (prefix .wh.) that indicates deletion of a path from a lower layer. It’s a convention from Docker/AUFS overlay drivers that OCI adopted. There is also the “opaque directory” concept using .wh..wh..opq to remove an entire directory’s children.
  • Why it matters: When unpacking layers, you must interpret whiteouts correctly. If a higher layer tar contains .wh.foo, it means “remove foo that existed in previous layers”. You must delete that file/dir from the merged tree, not keep the .wh.foo. This ensures that an image can override or remove content from its base images. The challenge solution must implement this logic when applying layers.
  • Beginner perspective: Think of layers like transparent sheets. If an upper sheet has a black blob labeled “DELETE X”, you erase X from the stack below. In practice, if you see a file named .wh.oldfile in the tar, you remove oldfile from whatever you’ve already unpacked, and ignore the .wh.oldfile itself.
  • DevOps detail: According to the OCI spec and blogs, the rule is: Whiteout files only apply to parent layers and once applied they are hidden. Also, files added in the same layer cannot be whited-out by that same layer—only subsequent layers. The opaque whiteout (.wh..wh..opq) signals that all existing children under this directory in lower layers should be ignored, effectively deleting the directory’s content wholesale. For robust handling, iterate tar entries in order: if you see a .wh.<name>, perform os.Remove() on the extracted path from previous layers. Then skip writing that entry. Implement directory opaqueness by checking for .wh..wh..opq and deleting all existing children from lower layers.
  • Caveats: Make sure not to treat .wh. files as actual content. Do not allow any real file/directory whose name starts with .wh. (spec forbids that). Ensure your untar code processes entries sequentially (whiteouts should ideally appear before siblings in the tar, but always check). Watch out for platform differences: some filesystems may not let you create a character device 0/0 like AUFS uses, but OCI whiteouts are regular empty files (per spec). Also note: docker’s overlayfs implementations changed whiteouts in the kernel, but OCI sticks with the AUFS-like format, so don’t use Linux’s overlayfs device nodes unless deliberately mimicking overlayfs.
SQLite Concurrency and Use
  • What it is: SQLite is a self-contained, serverless SQL database stored in a file. It is often used for simple state tracking. In this challenge, SQLite might be used to store Machine state or steps completed.
  • Why it matters: When multiple goroutines or processes need to record progress (e.g. “image downloaded”, “device created”), SQLite provides a convenient persistent ledger. However, SQLite’s concurrency model (single-writer) can be a bottleneck. Understanding it ensures you don’t deadlock your application.
  • Beginner perspective: SQLite allows many readers at once, but only one writer. In default journal mode, a write locks the whole file briefly. In WAL mode, readers can continue during a write. As a developer, you should open the DB in WAL mode (PRAGMA journal_mode=WAL), and use short transactions. In Go, use a connection pool (or better, a single sql.DB with db.SetMaxOpenConns(1) for single-connection writer).
  • DevOps detail: For higher throughput, use WAL as recommended by SQLite docs. This creates -wal and -shm files. All processes must be on the same host (WAL won’t work on NFS). Ensure timely checkpoints (automatic or manual). Long-running transactions will block new writes, so commit quickly. Handle SQLITE_BUSY by setting PRAGMA busy_timeout. Also, if you use the Fly FSM library or any multi-connection code, serialize DB access carefully. Some developers use a separate update goroutine to avoid multiple writers.
  • Caveats: SQLite file locks can appear if processes crash (journal left behind). Always defer tx.Rollback() to avoid dangling locks. If testing on a remote FS, know WAL fails. Do not set too high MaxOpenConns (or use the default of 1 in Go’s lib). Be aware that VACUUM or changing page size is not allowed in WAL mode. For the challenge, ensure only one instance of your program uses the DB (or that you handle locking errors).
  • Resources: SQLite’s official WAL documentation highlights concurrency benefits. StackOverflow and the SQLite FAQ have many Q&As on “database is locked” and best practices. Also see the Fly.io docs or forum for any SQLite advice (the community often mentions using WAL for Fly apps).
Device-Mapper Thin Provisioning
  • What it is: Device-mapper is a Linux kernel framework for creating virtual block devices. Thin provisioning is a feature (device-mapper target) that lets you carve out thinly-provisioned volumes from a single “thin pool” of storage. A thin pool consists of a data file (where contents go) and a metadata file (which tracks block mappings). Thin volumes (and snapshots) use the pool to allocate blocks on demand.
  • Why it matters: In this challenge, you may need to mount an OCI image as a block device (for example, to launch a VM image or perform block-level operations). Using thin devices can be much faster than file-based extraction or overlayfs because you can snap and mount block volumes. Thin snapshots also make it easy to roll back or share common blocks between images.
  • Beginner perspective: Thin provisioning is like memory overcommit: you promise a large virtual disk, but only physically consume space when writing. For example, to create a 20 GB thin volume, you define it in a pool of 100 GB. Only the parts you write actually take up space on the pool. In Go, you won’t manipulate device-mapper directly; instead you’d run commands like dmsetup or use an exec wrapper. The workflow is: create a pool (dmsetup create with “thin-pool”), then dmsetup create thin volumes inside it.
  • DevOps detail: Setting up a thin pool (direct-lvm mode) usually looks like:
  • Create a data file (dd if=/dev/zero of=data.raw bs=1G count=100) and a metadata file (dd if=/dev/zero of=meta.raw bs=1M count=1024).
  • Initialize with dmsetup create pool --table "0 $(block_size) thin-pool data.raw meta.raw ...".
  • Create a thin volume with dmsetup message /dev/mapper/pool 0 "create_thin <id>". Then dmsetup create thin-vol --table "0 <size> thin /dev/mapper/pool <id>".Once created, /dev/mapper/thin-vol acts like a raw disk of the given size. You can format and mount it, or attach it to a VM. Docker uses similar steps for its devicemapper storage. Key parameters include the data_block_size and low_water_mark (for pool auto-extension).
  • Caveats: Device-mapper requires root privileges and direct block devices. Loopback modes exist (dm-loop) but are slower. Be very careful with the thin pool’s free space: if you fill it, I/O will error. Monitor both metadata and data usage. If the metadata device is full or corrupt, the pool goes read-only and must be repaired. Also note: thin provisioning adds slight overhead (see fragmentation warnings in the kernel docs). On some systems, fallocate or losetup might be needed to create backing files. Finally, Linux 6.x changed overlay whiteouts (xattrs) but OCI still uses AUFS-style, so use file deletes as per spec (whiteouts) rather than relying on overlay-specific features.
  • Resources: The Linux kernel Device Mapper docs provide a thorough introduction (see “thin-provisioning.txt”). The StackOverflow answer in [49] explains thin provisioning in plain language (it notes thin pools behave like overcommitted memory). Docker’s docs on devicemapper (though deprecated) also illustrate the direct-LVM setup. Practical guides (like LINBIT’s KB or LVM thin man pages) can help with dmsetup commands.
3. Implementation Plan
We break the solution into sequential phases. Each phase builds on the previous, ensuring correctness before moving on.
  • CLI Skeleton and Argument Parsing
  • Goal: Set up the basic command-line interface. At minimum, define a main command (e.g. fly-machine-manager) and subcommands/flags (like download, unpack, apply, etc., or a single command with multiple flags).
  • Why: A well-structured CLI lets you run each phase independently and accept parameters (S3 bucket/key, local DB path, etc.). It’s also the user’s entry point.
  • Output: Code files (e.g. main.go, cli.go). On running --help, it prints usage. If using a library like cobra or Go’s flag, it should register flags (for example, --bucket, --key, --image-name).
  • Success: Running the program with no arguments shows usage, with --help shows flags. No errors on startup.
  • Files/Modules: github.com/spf13/cobra (if chosen) or flag/pflag. Structure: perhaps cmd/ folder with subcommands.
  • Go Packages/Techniques: flag or cobra for args; fmt for help text; possibly os.Exit on errors.
  • Resources: Look at examples of Go CLIs (the Cobra README or Go’s flag docs).
  • S3 Download Logic
  • Goal: Retrieve an OCI image tarball (or blobs) from S3 given bucket and key.
  • Why: We need the image data locally to process it. Doing this early verifies network/config.
  • Output: A local file (e.g. image.tar.gz) or a stream ready to unpack. Potentially also the manifest JSON.
  • Success: The file is present and its size or checksum matches what was expected. Logging prints “Download successful.”
  • Files/Modules: Use AWS SDK for Go (e.g. github.com/aws/aws-sdk-go-v2/service/s3). Authentication can use env vars or IAM role.
  • Go Packages/Techniques: s3.GetObject to fetch, then io.Copy to a local file. Use a context with timeout. If using AWS Go v2, create a config and S3Client.
  • Resources: AWS SDK documentation (https://aws.github.io/aws-sdk-go-v2/docs/). Tutorials on S3 downloads in Go.
  • Tarball Verification and Validation
  • Goal: Verify the downloaded image is a valid OCI image tar (correct format, uncorrupted).
  • Why: Catch corrupt or wrong files before deep extraction. For example, ensure the file is a tar (or gzip) and contains an index.json or manifest.json.
  • Output: No new files, but errors if invalid. Possibly log sizes or checksums.
  • Success: If a manifest or index is found and parsed correctly. If the tar is corrupt, fail gracefully.
  • Files/Modules: Use Go’s archive/tar and compress/gzip (or compress/zstd if needed). Possibly github.com/opencontainers/image-spec/specs-go/v1 for JSON structs.
  • Go Packages/Techniques: Open the file (or reader). Peek first bytes to detect gzip (magic header \x1f\x8b). If gzipped, wrap in gzip.NewReader. Then use tar.NewReader to iterate entries. Look for index.json or manifest.json. Parse with encoding/json.
  • Resources: Go’s tar and gzip docs. The OCI spec for manifest/index formats. If using a library like go-containerregistry, see examples of reading OCI tar archives.
  • Unpacking Layers with Whiteout Handling
  • Goal: Extract each filesystem layer in order, applying additions and whiteouts to a local directory (or block device) representing the rootfs.
  • Why: This reconstructs the container’s filesystem. It allows you to then use the resulting files or create a block device from them.
  • Output: A directory tree (e.g. workdir/rootfs/) or a populated thin-volume. Key artifacts: final file list reflecting all layers, minus deleted files.
  • Success: After processing all layers, the expected final files exist. Whiteouted files are removed. For example, if a layer had .wh.app, then /app should not exist. You can verify by listing the directory.
  • Files/Modules: Continue using archive/tar to extract each layer’s tar entries. For each entry:
  • If the name starts with .wh., delete the corresponding name (strip .wh. prefix) from the output dir.
  • Otherwise, create the file/dir/symlink as usual (tar.Header contains type and data).
  • Go Packages/Techniques: os.MkdirAll, os.RemoveAll, ioutil.WriteFile, os.Symlink as needed. Preserve permissions/modes from tar.Header. Use header.FileInfo() for convenience.
  • Resources: OCI spec on whiteouts in layers (e.g. the Cyphar blog snippet). Go tar extraction tutorials. For example, see Docker’s or runc’s code for layer unpacking.
  • SQLite State Tracking
  • Goal: Record each major step’s status in a SQLite database. For example, tables like images(id, status, last_updated) or tasks(task_id, state, updated_at).
  • Why: If the process restarts or is retried, the system knows which steps completed. E.g., if download succeeded but unpack failed, don’t download again but proceed to unpack. FSM logic often requires persisting states.
  • Output: A .db file (e.g. state.db) with tables. After each phase, insert or update rows (e.g. INSERT INTO progress VALUES ('download', 'done')).
  • Success: Queries on the DB reflect accurate state: e.g., SELECT status FROM progress WHERE phase='download' returns "done" after download. The program should skip steps already marked as done.
  • Files/Modules: Use modernc.org/sqlite or github.com/mattn/go-sqlite3. Create DB schema in code or via .sql file. Wrap DB writes in transactions. Ensure to PRAGMA journal_mode=WAL.
  • Go Packages/Techniques: The standard database/sql with a SQLite driver. Use db.Exec for DDL and updates. For concurrency, db.SetMaxOpenConns(1) can serialize access.
  • Resources: SQLite docs on WAL mode (to avoid lock problems). Example schemas for job tracking. Consider the Fly FSM design: it often writes progress as it moves through states.
  • Device-Mapper Integration (Thin Pool Setup)
  • Goal: Use Linux device-mapper to create a thin-provisioned block device that represents the unpacked image. Then populate it with the filesystem from step 4.
  • Why: Launching a Fly Machine might require a block-level image (as these “Machines” are VMs/VM-like). Thin devices allow mounting or attaching the image as a disk efficiently.
  • Output: One or more /dev/mapper/ block devices. For example, a device /dev/mapper/fly-machine-root that you can mount -o loop or use with a VM. You may also generate a logical volume if using LVM (though plain dm is fine).
  • Success: The device is created (check dmsetup ls). When you read from the device (e.g. mount it to a directory), you see the files from the layers. You could compare checksums of files before/after.
  • Files/Modules: Likely no Go library; use exec.Command("dmsetup", ...) or call syscall if confident. Steps include:
  • If not existing, create a new thin pool: run dmsetup create pool --table "0 <blocks> thin-pool datafile metadatafile <blocksize> <lowater>".
  • Create a thin volume: dmsetup message /dev/mapper/pool 0 "create_thin <id>" then dmsetup create volumeX --table "0 <blocks> thin /dev/mapper/pool <id>".
  • Use losetup if needed to attach a loop device for the data file (some workflows do this).
  • Format the new device (e.g. mkfs.ext4 /dev/mapper/volumeX) and mount it. Copy the filesystem from rootfs/ directory into it (cp -a). Then unmount.
  • Go Packages/Techniques: Use os/exec to run dmsetup, mkfs, mount, etc. Handle permissions (this part must run as root or via sudo). Alternatively, use CGO to call libdevmapper (complex). Ensure proper cleanup on errors (remove mappings with dmsetup remove).
  • Resources: The Linux Device Mapper Thin Provisioning doc details the dmsetup commands. The Docker device-mapper docs (though archived) outline direct-lvm setup (also present in [49] references). A practical guide: https://github.com/lvmteam/lvm2/blob/master/doc/ thinprovisioninghowto or LINBIT docs.
  • FSM Orchestration of Phases
  • Goal: Tie all the above steps into a coherent workflow using an FSM approach. For example, states might be Initialized, Downloaded, Validated, Unpacked, Mounted, Started. Each step is a transition.
  • Why: Using an FSM means if the program restarts mid-way, it picks up the correct state. It also structures the code cleanly.
  • Output: The actual running process will transition through states. You should log or print state changes. In DB, a table might store current state.
  • Success: The program finishes with final state (e.g. Completed) and all intermediate states done exactly once. If an error occurs at any state, it should not move past that state until fixed.
  • Files/Modules: Import github.com/superfly/fsm. Define FSM states and transitions in code. For example, on startup check DB for last state and advance. If using native Go without the library, you could implement state logic manually (a switch or chained functions guided by the DB).
  • Go Packages/Techniques: If using superfly/fsm, you’ll set up a fsm.Builder, define states and enter/exit hooks for each. Otherwise use a simple state machine design pattern. Use the DB to persist state after each transition. Handle signals (SIGINT) to allow clean exit (so state is not lost).
  • Resources: Although sparse, see the GitHub code for superfly/fsm for examples, and Fly’s job page for motivation. Developer blogs like the Flyd Operator Sim highlight the importance of robust state management.
  • Additional CLI Arguments and Logging
  • Goal: Expand the CLI to accept arguments for all needed parameters (e.g. S3 credentials, local paths, pool names) and integrate structured logging for observability.
  • Why: Users must configure the program; logs help debug. Also, after initial skeleton, add any flags discovered to be necessary (like verbose mode).
  • Output: Updated CLI --flags and a logging component. E.g. logs written to stdout or a file, including time, state, errors.
  • Success: Running with a --verbose flag shows detailed logs. Errors are clearly reported. Flag validation is done (missing required flags causes usage message).
  • Files/Modules: Perhaps use github.com/sirupsen/logrus or Go’s built-in log package. Organize flags for S3 endpoint, bucket, keys, database file, etc.
  • Go Packages/Techniques: log or logrus; os.Getenv for defaults; filepath for path handling.
  • Resources: Standard CLI design guides (e.g. how to use Cobra flags).
Testing Device-Mapper on macOS/ARM64
Since macOS lacks Linux’s dmsetup, use a Linux VM. A convenient method is Lima. For example, on macOS run: 1. Install Lima: e.g. brew install lima (Julia Evans notes this step).2. Start a VM: limactl start default. This auto-downloads and boots Ubuntu.3. Enter the VM: lima gives you a shell. Now you have a Linux (ARM64 on M1) environment.4. Prepare device-mapper: sudo apt-get update && sudo apt-get install -y lvm2. This provides dmsetup and related tools. Ensure /dev/loop0 and /dev/mapper exist (ls /dev).5. Set up thin pool: Inside VM, use the dmsetup commands as in step 6 above. For example, create files data.raw and meta.raw, then sudo dmsetup create pool --table "0 2097152 thin-pool data.raw meta.raw 128 32768" (adjust sizes). Then sudo dmsetup message /dev/mapper/pool 0 "create_thin 0", then sudo dmsetup create thin0 --table "0 2097152 thin /dev/mapper/pool 0".6. Mount and test: sudo mkfs.ext4 /dev/mapper/thin0 && sudo mount /dev/mapper/thin0 /mnt. Now copy in some test files (e.g., untar an OCI layer), then ls /mnt. Everything worked only on Linux!
For ARM64 compatibility, Lima’s default Ubuntu should handle it. If you need x86 images, Lima supports QEMU with Rosetta (see Lima docs for arch: "x86_64" in the YAML). Once the Linux VM is running, the device-mapper logic operates just as it would on a Linux server, giving you confidence before deploying on a real Fly worker.

INFO[2025-11-11T07:15:04-03:00] Starting S3 Trivy Detective - Investigating flyio-platform-hiring-challenge/images/ 
INFO[2025-11-11T07:15:04-03:00] Initializing Trivy scanner...                
INFO[2025-11-11T07:15:04-03:00] Checking Trivy vulnerability database...     
INFO[2025-11-11T07:15:04-03:00] ✓ Vulnerability database ready               
INFO[2025-11-11T07:15:04-03:00] ✓ Trivy scanner ready                        
INFO[2025-11-11T07:15:04-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:15:04-03:00] Investigating: images/golang/1.tar           
INFO[2025-11-11T07:15:04-03:00] Size: 143595520 bytes (136.9 MB)             
INFO[2025-11-11T07:17:56-03:00] Downloaded: 143595520 bytes                  
INFO[2025-11-11T07:17:56-03:00] Computed SHA256: PzPjLlXCrvsmjeEB...         
INFO[2025-11-11T07:17:56-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:17:56-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:17:56-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:17:57-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:17:57-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:17:57-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:17:57-03:00] ✓ Scan complete: found 12 HIGH/CRITICAL vulnerabilities 
WARN[2025-11-11T07:17:57-03:00] ⚠️  Found 12 HIGH/CRITICAL vulnerabilities   
INFO[2025-11-11T07:17:57-03:00]                                              
INFO[2025-11-11T07:17:57-03:00] Vulnerability Summary                        
INFO[2025-11-11T07:17:57-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:17:57-03:00] CVE ID               Severity     Package                   Fixed Version   
INFO[2025-11-11T07:17:57-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:17:57-03:00] CVE-2025-4802        HIGH         libc-bin                  2.36-9+deb12u11 
INFO[2025-11-11T07:17:57-03:00] CVE-2025-4802        HIGH         libc6                     2.36-9+deb12u11 
INFO[2025-11-11T07:17:57-03:00] CVE-2025-32988       HIGH         libgnutls30               3.7.9-2+deb12u5 
INFO[2025-11-11T07:17:57-03:00] CVE-2025-32990       HIGH         libgnutls30               3.7.9-2+deb12u5 
INFO[2025-11-11T07:17:57-03:00] CVE-2025-31115       HIGH         liblzma5                  5.4.1-1         
INFO[2025-11-11T07:17:57-03:00] CVE-2025-6020        HIGH         libpam-modules            N/A             
INFO[2025-11-11T07:17:57-03:00] CVE-2025-6020        HIGH         libpam-modules-bin        N/A             
INFO[2025-11-11T07:17:57-03:00] CVE-2025-6020        HIGH         libpam-runtime            N/A             
INFO[2025-11-11T07:17:57-03:00] CVE-2025-6020        HIGH         libpam0g                  N/A             
INFO[2025-11-11T07:17:57-03:00] CVE-2023-31484       HIGH         perl-base                 5.36.0-7+deb... 
INFO[2025-11-11T07:17:57-03:00] CVE-2024-56406       HIGH         perl-base                 5.36.0-7+deb... 
INFO[2025-11-11T07:17:57-03:00] CVE-2023-45853       CRITICAL     zlib1g                    N/A             
INFO[2025-11-11T07:17:57-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:17:57-03:00]                                              
INFO[2025-11-11T07:17:57-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:17:57-03:00] Investigating: images/golang/2.tar           
INFO[2025-11-11T07:17:57-03:00] Size: 49670144 bytes (47.4 MB)               
INFO[2025-11-11T07:18:58-03:00] Downloaded: 49670144 bytes                   
INFO[2025-11-11T07:18:58-03:00] Computed SHA256: A0VB9R/xQ1VEMBJr...         
INFO[2025-11-11T07:18:58-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:18:58-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:18:58-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:18:58-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:18:58-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:18:58-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:18:58-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:18:58-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:18:58-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:18:58-03:00] Investigating: images/golang/3.tar           
INFO[2025-11-11T07:18:58-03:00] Size: 188129280 bytes (179.4 MB)             
INFO[2025-11-11T07:19:16-03:00] Downloaded: 188129280 bytes                  
INFO[2025-11-11T07:19:16-03:00] Computed SHA256: 3rMBn7Lg6udi9XI1...         
INFO[2025-11-11T07:19:16-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:19:16-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:19:16-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:19:17-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:19:17-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:19:17-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:19:17-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:19:17-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:19:17-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:19:17-03:00] Investigating: images/golang/4.tar           
INFO[2025-11-11T07:19:17-03:00] Size: 242172416 bytes (231.0 MB)             
INFO[2025-11-11T07:19:37-03:00] Downloaded: 242172416 bytes                  
INFO[2025-11-11T07:19:37-03:00] Computed SHA256: vo7/7dlYjVUipjn9...         
INFO[2025-11-11T07:19:37-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:19:37-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:19:37-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:19:38-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:19:38-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:19:38-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:19:38-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:19:38-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:19:38-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:19:38-03:00] Investigating: images/golang/5.tar           
INFO[2025-11-11T07:19:38-03:00] Size: 260531712 bytes (248.5 MB)             
INFO[2025-11-11T07:20:00-03:00] Downloaded: 260531712 bytes                  
INFO[2025-11-11T07:20:00-03:00] Computed SHA256: LsDUu1pjcp+z0Eov...         
INFO[2025-11-11T07:20:00-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:20:00-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:20:00-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:20:02-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:20:02-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:20:03-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:20:03-03:00] ✓ Scan complete: found 4 HIGH/CRITICAL vulnerabilities 
WARN[2025-11-11T07:20:03-03:00] ⚠️  Found 4 HIGH/CRITICAL vulnerabilities    
INFO[2025-11-11T07:20:03-03:00]                                              
INFO[2025-11-11T07:20:03-03:00] Vulnerability Summary                        
INFO[2025-11-11T07:20:03-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:20:03-03:00] CVE ID               Severity     Package                   Fixed Version   
INFO[2025-11-11T07:20:03-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:20:03-03:00] CVE-2024-45337       CRITICAL     golang.org/x/crypto       0.31.0          
INFO[2025-11-11T07:20:03-03:00] CVE-2025-22869       HIGH         golang.org/x/crypto       0.35.0          
INFO[2025-11-11T07:20:03-03:00] CVE-2024-45337       CRITICAL     golang.org/x/crypto       0.31.0          
INFO[2025-11-11T07:20:03-03:00] CVE-2025-22869       HIGH         golang.org/x/crypto       0.35.0          
INFO[2025-11-11T07:20:03-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:20:03-03:00]                                              
INFO[2025-11-11T07:20:03-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:20:03-03:00] Investigating: images/node/1.tar             
INFO[2025-11-11T07:20:03-03:00] Size: 143595520 bytes (136.9 MB)             
INFO[2025-11-11T07:20:16-03:00] Downloaded: 143595520 bytes                  
INFO[2025-11-11T07:20:16-03:00] Computed SHA256: PzPjLlXCrvsmjeEB...         
INFO[2025-11-11T07:20:16-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:20:16-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:20:16-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:20:17-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:20:17-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:20:17-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:20:17-03:00] ✓ Scan complete: found 12 HIGH/CRITICAL vulnerabilities 
WARN[2025-11-11T07:20:17-03:00] ⚠️  Found 12 HIGH/CRITICAL vulnerabilities   
INFO[2025-11-11T07:20:17-03:00]                                              
INFO[2025-11-11T07:20:17-03:00] Vulnerability Summary                        
INFO[2025-11-11T07:20:17-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:20:17-03:00] CVE ID               Severity     Package                   Fixed Version   
INFO[2025-11-11T07:20:17-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:20:17-03:00] CVE-2025-4802        HIGH         libc-bin                  2.36-9+deb12u11 
INFO[2025-11-11T07:20:17-03:00] CVE-2025-4802        HIGH         libc6                     2.36-9+deb12u11 
INFO[2025-11-11T07:20:17-03:00] CVE-2025-32988       HIGH         libgnutls30               3.7.9-2+deb12u5 
INFO[2025-11-11T07:20:17-03:00] CVE-2025-32990       HIGH         libgnutls30               3.7.9-2+deb12u5 
INFO[2025-11-11T07:20:17-03:00] CVE-2025-31115       HIGH         liblzma5                  5.4.1-1         
INFO[2025-11-11T07:20:17-03:00] CVE-2025-6020        HIGH         libpam-modules            N/A             
INFO[2025-11-11T07:20:17-03:00] CVE-2025-6020        HIGH         libpam-modules-bin        N/A             
INFO[2025-11-11T07:20:17-03:00] CVE-2025-6020        HIGH         libpam-runtime            N/A             
INFO[2025-11-11T07:20:17-03:00] CVE-2025-6020        HIGH         libpam0g                  N/A             
INFO[2025-11-11T07:20:17-03:00] CVE-2023-31484       HIGH         perl-base                 5.36.0-7+deb... 
INFO[2025-11-11T07:20:17-03:00] CVE-2024-56406       HIGH         perl-base                 5.36.0-7+deb... 
INFO[2025-11-11T07:20:17-03:00] CVE-2023-45853       CRITICAL     zlib1g                    N/A             
INFO[2025-11-11T07:20:17-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:20:17-03:00]                                              
INFO[2025-11-11T07:20:17-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:20:17-03:00] Investigating: images/node/2.tar             
INFO[2025-11-11T07:20:17-03:00] Size: 49670144 bytes (47.4 MB)               
INFO[2025-11-11T07:20:22-03:00] Downloaded: 49670144 bytes                   
INFO[2025-11-11T07:20:22-03:00] Computed SHA256: A0VB9R/xQ1VEMBJr...         
INFO[2025-11-11T07:20:22-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:20:22-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:20:22-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:20:22-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:20:22-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:20:22-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:20:22-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:20:22-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:20:22-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:20:22-03:00] Investigating: images/node/3.tar             
INFO[2025-11-11T07:20:22-03:00] Size: 188129280 bytes (179.4 MB)             
INFO[2025-11-11T07:20:38-03:00] Downloaded: 188129280 bytes                  
INFO[2025-11-11T07:20:38-03:00] Computed SHA256: 3rMBn7Lg6udi9XI1...         
INFO[2025-11-11T07:20:38-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:20:38-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:20:38-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:20:39-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:20:39-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:20:39-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:20:39-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:20:39-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:20:39-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:20:39-03:00] Investigating: images/node/4.tar             
INFO[2025-11-11T07:20:39-03:00] Size: 569617408 bytes (543.2 MB)             
INFO[2025-11-11T07:21:25-03:00] Downloaded: 569617408 bytes                  
INFO[2025-11-11T07:21:25-03:00] Computed SHA256: 7n416Txmgy55Vf5o...         
INFO[2025-11-11T07:21:25-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:21:25-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:21:25-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:21:27-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:21:27-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:21:28-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:21:28-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:21:28-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:21:28-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:21:28-03:00] Investigating: images/node/5.tar             
INFO[2025-11-11T07:21:28-03:00] Size: 21504 bytes (0.0 MB)                   
INFO[2025-11-11T07:21:28-03:00] Downloaded: 21504 bytes                      
INFO[2025-11-11T07:21:28-03:00] Computed SHA256: oDonC8z89KsZI7Lk...         
INFO[2025-11-11T07:21:28-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:21:28-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:21:28-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:21:28-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:21:28-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:21:28-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:21:28-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:21:28-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:21:28-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:21:28-03:00] Investigating: images/python/1.tar           
INFO[2025-11-11T07:21:28-03:00] Size: 143595520 bytes (136.9 MB)             
INFO[2025-11-11T07:21:40-03:00] Downloaded: 143595520 bytes                  
INFO[2025-11-11T07:21:40-03:00] Computed SHA256: PzPjLlXCrvsmjeEB...         
INFO[2025-11-11T07:21:40-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:21:40-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:21:40-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:21:41-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:21:41-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:21:42-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:21:42-03:00] ✓ Scan complete: found 12 HIGH/CRITICAL vulnerabilities 
WARN[2025-11-11T07:21:42-03:00] ⚠️  Found 12 HIGH/CRITICAL vulnerabilities   
INFO[2025-11-11T07:21:42-03:00]                                              
INFO[2025-11-11T07:21:42-03:00] Vulnerability Summary                        
INFO[2025-11-11T07:21:42-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:21:42-03:00] CVE ID               Severity     Package                   Fixed Version   
INFO[2025-11-11T07:21:42-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:21:42-03:00] CVE-2025-4802        HIGH         libc-bin                  2.36-9+deb12u11 
INFO[2025-11-11T07:21:42-03:00] CVE-2025-4802        HIGH         libc6                     2.36-9+deb12u11 
INFO[2025-11-11T07:21:42-03:00] CVE-2025-32988       HIGH         libgnutls30               3.7.9-2+deb12u5 
INFO[2025-11-11T07:21:42-03:00] CVE-2025-32990       HIGH         libgnutls30               3.7.9-2+deb12u5 
INFO[2025-11-11T07:21:42-03:00] CVE-2025-31115       HIGH         liblzma5                  5.4.1-1         
INFO[2025-11-11T07:21:42-03:00] CVE-2025-6020        HIGH         libpam-modules            N/A             
INFO[2025-11-11T07:21:42-03:00] CVE-2025-6020        HIGH         libpam-modules-bin        N/A             
INFO[2025-11-11T07:21:42-03:00] CVE-2025-6020        HIGH         libpam-runtime            N/A             
INFO[2025-11-11T07:21:42-03:00] CVE-2025-6020        HIGH         libpam0g                  N/A             
INFO[2025-11-11T07:21:42-03:00] CVE-2023-31484       HIGH         perl-base                 5.36.0-7+deb... 
INFO[2025-11-11T07:21:42-03:00] CVE-2024-56406       HIGH         perl-base                 5.36.0-7+deb... 
INFO[2025-11-11T07:21:42-03:00] CVE-2023-45853       CRITICAL     zlib1g                    N/A             
INFO[2025-11-11T07:21:42-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:21:42-03:00]                                              
INFO[2025-11-11T07:21:42-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:21:42-03:00] Investigating: images/python/2.tar           
INFO[2025-11-11T07:21:42-03:00] Size: 49670144 bytes (47.4 MB)               
INFO[2025-11-11T07:21:46-03:00] Downloaded: 49670144 bytes                   
INFO[2025-11-11T07:21:46-03:00] Computed SHA256: A0VB9R/xQ1VEMBJr...         
INFO[2025-11-11T07:21:46-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:21:46-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:21:46-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:21:46-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:21:46-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:21:46-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:21:46-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:21:46-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:21:46-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:21:46-03:00] Investigating: images/python/3.tar           
INFO[2025-11-11T07:21:46-03:00] Size: 188129280 bytes (179.4 MB)             
INFO[2025-11-11T07:22:02-03:00] Downloaded: 188129280 bytes                  
INFO[2025-11-11T07:22:02-03:00] Computed SHA256: 3rMBn7Lg6udi9XI1...         
INFO[2025-11-11T07:22:02-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:22:02-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:22:02-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:22:02-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:22:03-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:22:03-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:22:03-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:22:03-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:22:03-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:22:03-03:00] Investigating: images/python/4.tar           
INFO[2025-11-11T07:22:03-03:00] Size: 569617408 bytes (543.2 MB)             
INFO[2025-11-11T07:22:49-03:00] Downloaded: 569617408 bytes                  
INFO[2025-11-11T07:22:49-03:00] Computed SHA256: 7n416Txmgy55Vf5o...         
INFO[2025-11-11T07:22:49-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:22:49-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:22:49-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:22:51-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:22:51-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:22:52-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:22:52-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:22:52-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:22:52-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:22:52-03:00] Investigating: images/python/5.tar           
INFO[2025-11-11T07:22:52-03:00] Size: 18808832 bytes (17.9 MB)               
INFO[2025-11-11T07:22:55-03:00] Downloaded: 18808832 bytes                   
INFO[2025-11-11T07:22:55-03:00] Computed SHA256: SdWDGPoIh2qi2dqg...         
INFO[2025-11-11T07:22:55-03:00] ℹ S3 metadata does not contain SHA256 checksum 
INFO[2025-11-11T07:22:55-03:00] ━━━ Trivy Vulnerability Scanning ━━━         
INFO[2025-11-11T07:22:55-03:00] Extracting tarball for scanning...           
INFO[2025-11-11T07:22:55-03:00] Generating SBOM with Trivy...                
INFO[2025-11-11T07:22:55-03:00] ✓ SBOM generated successfully                
INFO[2025-11-11T07:22:55-03:00] Scanning SBOM for vulnerabilities...         
INFO[2025-11-11T07:22:55-03:00] ✓ Scan complete: found 0 HIGH/CRITICAL vulnerabilities 
INFO[2025-11-11T07:22:55-03:00] ✓ No HIGH/CRITICAL vulnerabilities found     
INFO[2025-11-11T07:22:55-03:00] ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 
INFO[2025-11-11T07:22:55-03:00] ✅ Investigation complete: 15 objects processed 
WARN[2025-11-11T07:22:55-03:00] Vulnerabilities found: 4 images flagged with HIGH/CRITICAL vulnerabilities

### Phase 0: Investigative Scanner

- **Problem**: We don’t know what’s in these tarballs.
- **Output**: For each S3 object, compute SHA256, CRC64, scan tar headers, log suspicious markers.
- **Success**: Logs match expected structure. Tar scanners flag `.wh.` and unsafe paths.
- **Tools**: `net/http`, `logrus`, `crypto/sha256`, `hash/crc64`, `archive/tar`
- **Run**: `go run s3_detective.go -bucket flyio-platform-hiring-challenge -prefix images/`

---

### Phase 1: CLI Skeleton + Flags + Logging

- **Problem**: Need a command to drive this orchestration.
- **Output**: CLI binary that accepts image name, path, debug mode.
- **Success**: Prints command help. Recognizes and validates inputs.
- **Tools**: `flag`, `logrus`, `cobra` (optional)

---

### Phase 2: S3 Blob Downloader

- **Problem**: Need to download image tarballs safely.
- **Output**: Local copy of image files.
- **Success**: Temp file created with verified hash. Idempotent.
- **Tools**: `aws-sdk-go-v2`, anonymous credentials, context-aware GET

---

### Phase 3: Tarball Validator and Extractor

- **Problem**: Need to unpack tar into canonical rootfs.
- **Output**: Directory representing a container root filesystem.
- **Success**: Whiteout files handled correctly. Filesystem matches expectation.
- **Tools**: `archive/tar`, whiteout logic, symlink safety

---

### Phase 4: SQLite Tracker

- **Problem**: Track what image IDs exist, downloaded, unpacked, activated.
- **Output**: `images.db` with schema `images(id TEXT PRIMARY KEY, status TEXT, path TEXT)`
- **Success**: Concurrent reads OK. Writers don’t lock out readers.
- **Tools**: `go-sqlite3`, connection pool

---

### Phase 5: Device Mapper Integration

- **Problem**: Create thinpool, thin devices, and snapshots.
- **Output**: Logical device mounted and ready.
- **Success**: Can run `mount /dev/mapper/thin-1` and see rootfs.
- **Tools**: `dmsetup` via `os/exec`, `losetup`, fallback shell scripts
- **Test Env**: Lima VM on macOS with nested loopback enabled

---

### Phase 6: FSM Integration

- **Problem**: Stitch all steps into retryable state transitions.
- **Output**: FSM per image: states like FETCH → UNPACK → ACTIVATE
- **Success**: FSM resumes from partial failure. Clean step transitions.
- **Tools**: `github.com/superfly/fsm`, context-bound steps

---

### Phase 7: Logs, Observability, Flags

- **Problem**: Need debuggable, testable output.
- **Output**: Flags for dry-run, verbose mode. JSON logs if needed.
- **Success**: All operations logged. Errors wrapped.

---

### Phase 8: Extra — Persistent Worker Mode

- **Problem**: One-shot isn't enough for production.
- **Output**: Long-running binary that pulls tasks from queue or watches directory.
- **Success**: Can run continuously, pick up new images.
- **Tools**: `fsnotify`, long-lived FSM loop

---

### Phase 9: Testing, CLI polish, Docs

- **Problem**: Users/devs need clear UX.
- **Output**: Man page, `--help`, example usage, test images
- **Success**: Readable README, reproducible run

---

### Appendix: Devicemapper Testing on macOS

- **Tool**: [Lima](https://github.com/lima-vm/lima) (Linux VM on macOS)
- **Setup**:
  1. Install Lima: `brew install lima`
  2. Start VM: `limactl start default`
  3. Enable nested loop devices:
     ```yaml
     images:
       - location: "https://cloud-images.ubuntu.com/...img"
     mounts:
       - location: "~"
         writable: true
     containerd:
       system: false
       user: false
     ```
  4. SSH in: `lima shell default`
  5. Run setup script to configure thinpools

---

Let me know when you're ready to start coding from this plan or if you’d like a script template for any specific phase.
