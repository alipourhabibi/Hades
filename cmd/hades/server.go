package main

import (
	"context"
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	server "github.com/alipourhabibi/Hades/internal/hades"
	"github.com/alipourhabibi/Hades/internal/telemetry"
	"github.com/spf13/cobra"
)

// newServeCmd returns the "serve" subcommand that starts the registry server.
func newServeCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Hades schema registry server",
		RunE: func(_ *cobra.Command, _ []string) error {
			configs, err := config.LoadFile(configFile)
			if err != nil {
				return err
			}

			ctx := context.Background()

			shutdownTelemetry, err := telemetry.Setup(ctx, configs.Telemetry)
			if err != nil {
				return fmt.Errorf("telemetry setup: %w", err)
			}
			defer func() { _ = shutdownTelemetry(ctx) }()

			srv, err := server.NewServer(ctx, configs)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(ctx)
			go srv.Run(ctx, cancel)
			<-ctx.Done()
			return nil
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "config/config.yaml", "path to the YAML configuration file")

	return cmd
}
