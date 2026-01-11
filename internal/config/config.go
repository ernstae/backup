package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the YAML configuration schema for the Go rewrite.
type Config struct {
	Version int `yaml:"version"`
	Paths   struct {
		StateDir string `yaml:"state_dir"`
		LockFile string `yaml:"lock_file"`
	} `yaml:"paths"`

	Source struct {
		BackupDirs  []string `yaml:"backup_dirs"`
		TarOptions  string   `yaml:"tar_options"`
		BackupMySQL bool     `yaml:"backup_mysql"`
	} `yaml:"source"`

	MySQL struct {
		DefaultsFile         string `yaml:"defaults_file"`
		MySQLBinary          string `yaml:"mysql_binary"`
		MySQLBinlogBinary    string `yaml:"mysqlbinlog_binary"`
		XtrabackupBinary     string `yaml:"xtrabackup_binary"`
		XbcloudBinary        string `yaml:"xbcloud_binary"`
		XbstreamBinary       string `yaml:"xbstream_binary"`
		FullBackup           string `yaml:"full_backup"`
		ExpireLogDays        int    `yaml:"expire_log_days"`
		CaptureRuntimeConfig bool   `yaml:"capture_runtime_config"`
		SkipBackupLocks      bool   `yaml:"skip_backup_locks"`
		RedoLogArchiveDir    string `yaml:"redo_log_archive_dir"`
	} `yaml:"mysql"`

	Backup struct {
		Intervals struct {
			Hourly  bool `yaml:"hourly"`
			Daily   bool `yaml:"daily"`
			Weekly  bool `yaml:"weekly"`
			Monthly bool `yaml:"monthly"`
			Yearly  bool `yaml:"yearly"`
		} `yaml:"intervals"`
	} `yaml:"backup"`

	Compression struct {
		Method  string `yaml:"method"`
		Level   int    `yaml:"level"`
		Threads int    `yaml:"threads"`
	} `yaml:"compression"`

	Encryption struct {
		GPG struct {
			Enabled       bool   `yaml:"enabled"`
			Recipient     string `yaml:"recipient"`
			Keyring       string `yaml:"keyring"`
			SecretKeyring string `yaml:"secret_keyring"`
			Binary        string `yaml:"binary"`
		} `yaml:"gpg"`
	} `yaml:"encryption"`

	Destination struct {
		Kind          string `yaml:"kind"`
		Provider      string `yaml:"provider"`
		RemotePath    string `yaml:"remote_path"`
		KeepLocalPath string `yaml:"keep_local_path"`
	} `yaml:"destination"`

	S3 struct {
		Bucket    string `yaml:"bucket"`
		Region    string `yaml:"region"`
		AccessKey string `yaml:"access_key"`
		SecretKey string `yaml:"secret_key"`
		Endpoint  string `yaml:"endpoint"`
	} `yaml:"s3"`

	Azure struct {
		ConnectionString string `yaml:"connection_string"`
		Container        string `yaml:"container"`
	} `yaml:"azure"`

	GCS struct {
		Bucket      string `yaml:"bucket"`
		Credentials string `yaml:"credentials_file"`
		GsutilBin   string `yaml:"gsutil_binary"`
	} `yaml:"gcs"`

	SSH struct {
		Host                  string `yaml:"host"`
		User                  string `yaml:"user"`
		Port                  int    `yaml:"port"`
		KeyPath               string `yaml:"key_path"`
		StrictHostKeyChecking *bool  `yaml:"strict_host_key_checking"`
		KnownHostsFile        string `yaml:"known_hosts_file"`
	} `yaml:"ssh"`

	Retention struct {
		Remote struct {
			Hourly  int `yaml:"hourly"`
			Daily   int `yaml:"daily"`
			Weekly  int `yaml:"weekly"`
			Monthly int `yaml:"monthly"`
			Yearly  int `yaml:"yearly"`
		} `yaml:"remote"`
		Local struct {
			Hourly  int `yaml:"hourly"`
			Daily   int `yaml:"daily"`
			Weekly  int `yaml:"weekly"`
			Monthly int `yaml:"monthly"`
			Yearly  int `yaml:"yearly"`
		} `yaml:"local"`
	} `yaml:"retention"`

	Metrics struct {
		Transport       string   `yaml:"transport"`
		Address         string   `yaml:"address"`
		Prefix          string   `yaml:"prefix"`
		Tags            []string `yaml:"tags"`
		IntervalSeconds int      `yaml:"interval_seconds"`
	} `yaml:"metrics"`

	Verify struct {
		DepthBytes int `yaml:"depth_bytes"`
	} `yaml:"verify"`
}

// Load reads a YAML config file into a Config struct.
func Load(path string) (*Config, error) {
	if path == "" {
		return nil, errors.New("config path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
