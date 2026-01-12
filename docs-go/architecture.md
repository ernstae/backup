# Architecture

## Overview

TwinDB Backup v5.0.0 is a streaming backup system that processes data through a pipeline of sources, modifiers, and destinations. The architecture is designed for high performance, reliability, and compliance with security standards.

## Components

- `cmd/twindb-backup`: CLI entrypoint using Cobra
- `internal/config`: YAML configuration parser and validator
- `internal/logging`: structured logging with JSON file output
- `internal/metrics`: statsd/datadog metrics emitters
- `internal/pipeline`: backup pipeline orchestrator (sources → modifiers → destinations)
- `internal/destination`: pluggable storage backends (xbcloud, ssh, gcs, local)
- `internal/status`: status metadata storage with WORM append-only logs
- `internal/restore`: restore and PITR (Point-in-Time Recovery) logic
- `internal/verify`: backup integrity verification
- `internal/lock`: file-based locking with queue support (one active, one waiting, rest rejected)
- `internal/mysql`: MySQL client and configuration discovery
- `internal/crypto`: GPG encryption/decryption support

## System Architecture Diagram

```mermaid
graph TB
    subgraph "CLI Layer"
        CLI[twindb-backup CLI]
    end
    
    subgraph "Configuration"
        CONFIG[YAML Config]
        LOCK[Lock Manager]
    end
    
    subgraph "Backup Pipeline"
        SOURCE[Source<br/>MySQL/Files]
        COMPRESS[Compression<br/>gzip/bzip2]
        ENCRYPT[Encryption<br/>GPG]
        DEST[Destination<br/>xbcloud/ssh/gcs/local]
    end
    
    subgraph "Storage"
        REMOTE[Remote Storage<br/>S3/Azure/GCS/SSH]
        LOCAL[Local Storage<br/>Optional]
        STATUS[Status Store<br/>WORM Logs]
    end
    
    subgraph "Restore Pipeline"
        RESTORE[Restore Engine]
        PITR[PITR Engine<br/>GTID/Time]
        PREPARE[xtrabackup prepare]
    end
    
    CLI --> CONFIG
    CLI --> LOCK
    CLI --> SOURCE
    SOURCE --> COMPRESS
    COMPRESS --> ENCRYPT
    ENCRYPT --> DEST
    DEST --> REMOTE
    DEST --> LOCAL
    DEST --> STATUS
    CLI --> RESTORE
    RESTORE --> PITR
    PITR --> PREPARE
    REMOTE --> RESTORE
    STATUS --> RESTORE
```

## Backup Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Lock
    participant Pipeline
    participant MySQL
    participant Compress
    participant Encrypt
    participant Destination
    participant Storage
    participant Status

    User->>CLI: backup daily
    CLI->>Lock: Acquire lock
    Lock-->>CLI: Lock acquired
    CLI->>Pipeline: Run backup
    Pipeline->>MySQL: xtrabackup stream
    MySQL-->>Pipeline: Backup stream
    Pipeline->>Compress: Compress stream
    Compress-->>Pipeline: Compressed stream
    Pipeline->>Encrypt: Encrypt stream
    Encrypt-->>Pipeline: Encrypted stream
    Pipeline->>Destination: Upload stream
    Destination->>Storage: Write backup
    Pipeline->>Status: Save metadata
    Status->>Storage: Append log entry
    Pipeline-->>CLI: Backup complete
    CLI->>Lock: Release lock
    CLI-->>User: Success
```

## Restore Flow (PITR)

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Restore
    participant Status
    participant Storage
    participant Extract
    participant Prepare
    participant Binlog
    participant MySQL

    User->>CLI: restore mysql --pitr-gtid
    CLI->>Restore: Start restore
    Restore->>Status: Load backup metadata
    Status-->>Restore: Backup list
    Restore->>Storage: Download base backup
    Storage-->>Extract: Backup stream
    Extract->>Prepare: Extract files
    loop For each incremental
        Restore->>Storage: Download incremental
        Storage-->>Prepare: Incremental stream
        Prepare->>Prepare: Apply incremental
    end
    Prepare->>Prepare: Final prepare
    Restore->>Binlog: Apply binlogs to GTID
    Binlog->>MySQL: Apply transactions
    Restore-->>CLI: Restore complete
    CLI-->>User: Success
```

## Data Flow Architecture

```mermaid
graph LR
    subgraph "Source"
        MYSQL[(MySQL Server)]
        FILES[(File System)]
    end
    
    subgraph "Stream Processing"
        STREAM[Stream Pipeline]
        TEE[Tee Reader<br/>Keep Local]
    end
    
    subgraph "Modifiers"
        COMP[Compression]
        ENC[Encryption]
    end
    
    subgraph "Destinations"
        XBCLOUD[xbcloud<br/>S3/Azure]
        SSH[SSH Server]
        GCS[GCS Bucket]
        LOCAL_DST[Local Path]
    end
    
    subgraph "Metadata"
        STATUS[Status Store<br/>WORM Logs]
        SNAPSHOT[Snapshots]
    end
    
    MYSQL -->|xtrabackup| STREAM
    FILES -->|tar| STREAM
    STREAM --> TEE
    TEE --> COMP
    TEE --> LOCAL_DST
    COMP --> ENC
    ENC --> XBCLOUD
    ENC --> SSH
    ENC --> GCS
    STREAM --> STATUS
    STATUS --> SNAPSHOT
```

## Lock Mechanism

The lock system prevents concurrent backup runs and resource waste:

```mermaid
stateDiagram-v2
    [*] --> Idle
    Idle --> LockAcquired: Process 1 arrives
    LockAcquired --> Waiting: Process 2 arrives
    Waiting --> Rejected: Process 3+ arrives
    Waiting --> LockAcquired: Process 1 completes
    LockAcquired --> [*]: Backup complete
    Waiting --> Timeout: Timeout exceeded
    Timeout --> [*]
    Rejected --> [*]
```

## WORM Storage Architecture

```mermaid
graph TB
    subgraph "Backup Artifacts"
        BACKUP[Backup Files<br/>Immutable]
    end
    
    subgraph "Status Storage"
        LOGS[Append-Only Logs<br/>Immutable]
        SNAPSHOT[Periodic Snapshots<br/>Overwritable]
    end
    
    subgraph "WORM Enforcement"
        WORM[WormDestination<br/>Wrapper]
        CHECK{File Exists?}
    end
    
    BACKUP --> WORM
    LOGS --> WORM
    SNAPSHOT --> WORM
    WORM --> CHECK
    CHECK -->|Yes| BLOCK[Block Overwrite]
    CHECK -->|No| ALLOW[Allow Write]
    CHECK -->|Snapshot| ALLOW
```

## Component Details

### Pipeline (`internal/pipeline`)

The pipeline orchestrates the backup process:

1. **Lock Acquisition**: Prevents concurrent runs
2. **Source Selection**: MySQL (xtrabackup) or files (tar)
3. **Stream Processing**: Pipes data through modifiers
4. **Modifiers**: Compression (gzip/bzip2) and encryption (GPG)
5. **Destination Upload**: Streams to remote storage
6. **Status Update**: Records metadata in WORM logs
7. **Retention**: Applies retention policies (if WORM disabled)

### Status Store (`internal/status`)

Manages backup metadata with WORM semantics:

- **Append-Only Logs**: Immutable event log for each backup
- **Snapshots**: Periodic summaries for fast loading
- **Checksums**: SHA-256 verification for integrity
- **GTID Tracking**: Enables PITR recovery

### Destination Interface (`internal/destination`)

Pluggable storage backends:

- **xbcloud**: S3/Azure via Percona's xbcloud tool
- **ssh**: Remote server via SSH/SCP
- **gcs**: Google Cloud Storage via gsutil
- **local**: Local filesystem
- **WORM Wrapper**: Enforces immutability

### Restore Engine (`internal/restore`)

Handles full restore and PITR:

1. **Backup Selection**: Finds base + incrementals
2. **Extraction**: Downloads and extracts backups
3. **Incremental Apply**: Applies incrementals in order
4. **Final Prepare**: Runs `xtrabackup --prepare`
5. **PITR**: Applies binlogs to GTID or timestamp
6. **Config Restore**: Restores MySQL config files
