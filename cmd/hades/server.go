package main

import (
	"github.com/alipourhabibi/Hades/config"
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

			_ = configs

			return nil
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "config/config.yaml", "Config file path to which the configs are loaded from")

	return cmd
}
