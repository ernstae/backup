package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/verify"
)

func newVerifyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a backup",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return nil
		},
	}

	mysqlCmd := &cobra.Command{
		Use:   "mysql <backup-id>",
		Short: "Verify a MySQL backup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			debug, _ := cmd.Flags().GetBool("debug")
			return verify.VerifyMySQL(context.Background(), cfg, cmd.Flags().Arg(0), verify.Options{Debug: debug})
		},
	}

	cmd.AddCommand(mysqlCmd)
	return cmd
}
