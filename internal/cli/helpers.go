package cli

import (
	"context"
	"os"

	"github.com/twindb/backup/internal/app"
	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/status"
)

func loadStatusStore(ctx context.Context, configPath string) (*status.Store, string, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, "", err
	}
	dst, err := app.BuildDestination(cfg)
	if err != nil {
		return nil, "", err
	}
	host, _ := os.Hostname()
	// Respect worm.enabled configuration
	wormEnabled := cfg.IsWORMEnabled()
	return status.NewStoreWithWORM(dst, wormEnabled, host), host, nil
}
