package app

import (
	"fmt"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/destination"
	"github.com/twindb/backup/internal/destination/gcs"
	"github.com/twindb/backup/internal/destination/local"
	"github.com/twindb/backup/internal/destination/ssh"
	"github.com/twindb/backup/internal/destination/xbcloud"
)

// BuildDestination constructs a destination based on config.
// Wraps the destination with WORM enforcement if WORM is enabled (default: true).
func BuildDestination(cfg *config.Config) (destination.Destination, error) {
	var dst destination.Destination
	var err error

	switch cfg.Destination.Kind {
	case "xbcloud":
		xbCfg := xbcloud.Config{
			Binary:     cfg.MySQL.XbcloudBinary,
			Provider:   xbcloud.Provider(cfg.Destination.Provider),
			RemotePath: cfg.Destination.RemotePath,
		}
		xbCfg.S3.Bucket = cfg.S3.Bucket
		xbCfg.S3.Region = cfg.S3.Region
		xbCfg.S3.AccessKey = cfg.S3.AccessKey
		xbCfg.S3.SecretKey = cfg.S3.SecretKey
		xbCfg.S3.Endpoint = cfg.S3.Endpoint
		xbCfg.Azure.ConnectionString = cfg.Azure.ConnectionString
		xbCfg.Azure.Container = cfg.Azure.Container
		dst, err = xbcloud.New(xbCfg)
		if err != nil {
			return nil, err
		}
	case "local":
		if cfg.Destination.RemotePath == "" {
			return nil, fmt.Errorf("destination.remote_path is required for local")
		}
		dst = local.New(cfg.Destination.RemotePath)
	case "ssh":
		sshCfg := ssh.Config{
			Host:                  cfg.SSH.Host,
			User:                  cfg.SSH.User,
			Port:                  cfg.SSH.Port,
			KeyPath:               cfg.SSH.KeyPath,
			StrictHostKeyChecking: cfg.SSH.StrictHostKeyChecking,
			KnownHostsFile:        cfg.SSH.KnownHostsFile,
			RemotePath:            cfg.Destination.RemotePath,
		}
		dst, err = ssh.New(sshCfg)
		if err != nil {
			return nil, err
		}
	case "gcs":
		gcsCfg := gcs.Config{
			Bucket:      cfg.GCS.Bucket,
			RemotePath:  cfg.Destination.RemotePath,
			Binary:      cfg.GCS.GsutilBin,
			Credentials: cfg.GCS.Credentials,
		}
		dst, err = gcs.New(gcsCfg)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported destination kind %q", cfg.Destination.Kind)
	}

	// Wrap with WORM enforcement (default: enabled)
	wormEnabled := cfg.IsWORMEnabled()

	return destination.NewWormDestination(dst, wormEnabled), nil
}
