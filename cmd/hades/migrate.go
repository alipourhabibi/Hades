package main

import (
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/storage/db"
	"github.com/spf13/cobra"
)

func newMigrateCmd() *cobra.Command {
	var (
		configFile string
	)
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "migrate the Hades DBs",
		RunE: func(_ *cobra.Command, _ []string) error {
			configs, err := config.LoadFile(configFile)
			if err != nil {
				return err
			}

			log, err := getLogger(configs.Logger)
			if err != nil {
				return err
			}

			db, err := db.New(configs.DB, log)
			if err != nil {
				return err
			}

			err = db.AutoMigrate()
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "config/config.yaml", "Config file path to which the configs are loaded from")

	return cmd
}
