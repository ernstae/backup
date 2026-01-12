# Destinations

TwinDB Backup v5.0.0 uses a pluggable destination interface. Primary production
paths are xbcloud for S3/Azure, with local/ssh/gcs support.

## Destination Types

TwinDB Backup supports multiple destination types:

- **xbcloud**: S3 and Azure Blob Storage (recommended for production)
- **ssh**: Remote server via SSH/SCP
- **gcs**: Google Cloud Storage
- **local**: Local filesystem

## xbcloud (S3/Azure)

The `xbcloud` destination uses Percona's `xbcloud` tool for efficient streaming
to S3 or Azure Blob Storage. This is the recommended destination for production
environments.

### S3 Configuration

```yaml
destination:
  kind: xbcloud
  provider: s3
  remote_path: /backups/mysql

s3:
  bucket: twindb-backups
  region: us-east-1
  access_key: ${AWS_ACCESS_KEY_ID}
  secret_key: ${AWS_SECRET_ACCESS_KEY}
  endpoint: ""  # Optional: for S3-compatible services
```

**Requirements:**
- `xbcloud` binary installed and in PATH
- AWS credentials with S3 write permissions
- S3 bucket exists and is accessible

**IAM Permissions (S3):**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:GetObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::twindb-backups/*",
        "arn:aws:s3:::twindb-backups"
      ]
    }
  ]
}
```

**S3-Compatible Services:**
- MinIO
- DigitalOcean Spaces
- Wasabi
- Backblaze B2

Set `endpoint` to use S3-compatible services:
```yaml
s3:
  bucket: my-backups
  region: us-east-1
  endpoint: https://s3.example.com
  access_key: ${ACCESS_KEY}
  secret_key: ${SECRET_KEY}
```

### Azure Configuration

```yaml
destination:
  kind: xbcloud
  provider: azure
  remote_path: /backups/mysql

azure:
  connection_string: ${AZURE_STORAGE_CONNECTION_STRING}
  container: twindb-backups
```

**Requirements:**
- `xbcloud` binary installed and in PATH
- Azure Storage account connection string
- Container exists in storage account

**Connection String Format:**
```
DefaultEndpointsProtocol=https;AccountName=myaccount;AccountKey=mykey;EndpointSuffix=core.windows.net
```

**Azure Permissions:**
- Storage Blob Data Contributor role
- Or specific permissions: `Microsoft.Storage/storageAccounts/blobServices/containers/write`

## SSH

The SSH destination uploads backups to a remote server via SSH and `cat`.

### Configuration

```yaml
destination:
  kind: ssh
  remote_path: /backups/mysql

ssh:
  host: backup.example.com
  user: backup
  port: 22
  key_path: /root/.ssh/id_rsa
  strict_host_key_checking: true
  known_hosts_file: /root/.ssh/known_hosts
```

**Requirements:**
- SSH access to remote server
- SSH key with write permissions
- Remote directory exists and is writable

**Setup SSH Key:**
```sh
# Generate SSH key (if needed)
ssh-keygen -t rsa -b 4096 -f /root/.ssh/id_rsa

# Copy public key to remote server
ssh-copy-id -i /root/.ssh/id_rsa.pub backup@backup.example.com

# Test connection
ssh -i /root/.ssh/id_rsa backup@backup.example.com "mkdir -p /backups/mysql"
```

**Automated Environments:**
For automated deployments, disable strict host key checking:
```yaml
ssh:
  strict_host_key_checking: false
  known_hosts_file: ""  # Optional: skip known_hosts
```

**Security Note:** Disabling host key checking reduces security. Use only in
controlled environments.

## GCS (Google Cloud Storage)

The GCS destination uses `gsutil` for uploading to Google Cloud Storage.

### Configuration

```yaml
destination:
  kind: gcs
  remote_path: /backups/mysql

gcs:
  bucket: twindb-backups
  credentials_file: /etc/twindb/gcs.json
  gsutil_binary: gsutil
```

**Requirements:**
- `gsutil` installed and in PATH
- GCS service account credentials JSON file
- GCS bucket exists and is accessible

**Setup Service Account:**
1. Create service account in Google Cloud Console
2. Grant "Storage Admin" role
3. Download JSON key file
4. Place at `/etc/twindb/gcs.json`

**Permissions:**
- `storage.objects.create`
- `storage.objects.get`
- `storage.objects.delete`
- `storage.buckets.get`

## Local

The local destination stores backups on the local filesystem.

### Configuration

```yaml
destination:
  kind: local
  remote_path: /var/backup/mysql
```

**Requirements:**
- Directory exists and is writable
- Sufficient disk space

**Use Cases:**
- Development/testing
- Local backup staging
- Fast restore from local copy

## Keep Local

The `keep_local_path` option stores a local copy while uploading to the remote
destination. This is useful for:
- Fast local restores
- Redundancy
- Offline access

### Configuration

```yaml
destination:
  kind: xbcloud
  provider: s3
  remote_path: /backups/mysql
  keep_local_path: /var/backup/local
```

**How it works:**
- Backup stream is duplicated using `io.TeeReader`
- One stream goes to remote destination
- One stream goes to local path
- Both streams are processed in parallel

**Storage Considerations:**
- Local copy uses additional disk space
- Ensure sufficient space for local copies
- Consider retention policies for local storage

## Destination Comparison

| Destination | Performance | Cost | Complexity | Use Case |
|------------|-------------|------|-------------|----------|
| xbcloud (S3) | High | Pay-per-use | Low | Production, cloud-native |
| xbcloud (Azure) | High | Pay-per-use | Low | Production, Azure environments |
| SSH | Medium | Infrastructure | Medium | On-premise, simple setup |
| GCS | High | Pay-per-use | Medium | Google Cloud environments |
| Local | High | Infrastructure | Low | Development, staging |

## Best Practices

1. **Use xbcloud for production:**
   - Efficient streaming
   - Automatic retry
   - Native integration

2. **Enable keep_local_path for fast restores:**
   - Reduces restore time
   - Provides redundancy
   - Enables offline access

3. **Use separate buckets/containers:**
   - Separate production and staging
   - Easier management
   - Better security

4. **Monitor storage costs:**
   - Review retention policies
   - Consider lifecycle policies
   - Archive old backups if needed

5. **Test destination connectivity:**
   - Verify credentials
   - Test upload/download
   - Monitor for errors
