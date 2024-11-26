package main

import (
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/storage/db"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var (
		configFile string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "serve the Hades server",
		RunE: func(cmd *cobra.Command, args []string) error {
			configs, err := config.LoadFile(configFile)
			if err != nil {
				return err
			}

			log, err := getLogger(configs.Logger)
			if err != nil {
				return err
			}

			db, err := db.New(configs.DB)
			if err != nil {
				return err
			}

			_ = db

			log.Info("Server Running...")

			return nil
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "config/config.yaml", "Config file path to which the configs are loaded from")

	return cmd
}
