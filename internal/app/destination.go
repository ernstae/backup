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
func BuildDestination(cfg *config.Config) (destination.Destination, error) {
	if cfg.Destination.Kind == "xbcloud" {
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
		return xbcloud.New(xbCfg)
	}
	if cfg.Destination.Kind == "local" {
		if cfg.Destination.RemotePath == "" {
			return nil, fmt.Errorf("destination.remote_path is required for local")
		}
		return local.New(cfg.Destination.RemotePath), nil
	}
	if cfg.Destination.Kind == "ssh" {
		sshCfg := ssh.Config{
			Host:                  cfg.SSH.Host,
			User:                  cfg.SSH.User,
			Port:                  cfg.SSH.Port,
			KeyPath:               cfg.SSH.KeyPath,
			StrictHostKeyChecking: cfg.SSH.StrictHostKeyChecking,
			KnownHostsFile:        cfg.SSH.KnownHostsFile,
			RemotePath:            cfg.Destination.RemotePath,
		}
		return ssh.New(sshCfg)
	}
	if cfg.Destination.Kind == "gcs" {
		gcsCfg := gcs.Config{
			Bucket:      cfg.GCS.Bucket,
			RemotePath:  cfg.Destination.RemotePath,
			Binary:      cfg.GCS.GsutilBin,
			Credentials: cfg.GCS.Credentials,
		}
		return gcs.New(gcsCfg)
	}
	return nil, fmt.Errorf("unsupported destination kind %q", cfg.Destination.Kind)
}
