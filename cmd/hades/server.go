package main

import (
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
			return nil
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "config/config.yaml", "Config file path to which the configs are loaded from")

	return cmd
}
