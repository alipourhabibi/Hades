package main

import (
	"context"
	"os/exec"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/events"
	"github.com/spf13/cobra"
)

func newSdkCmd() *cobra.Command {
	var (
		configFile string
	)
	cmd := &cobra.Command{
		Use:   "sdk",
		Short: "run sdk service",
		RunE: func(_ *cobra.Command, _ []string) error {
			err := exec.Command("buf", "--version").Run()
			if err != nil {
				return err
			}

			configs, err := config.LoadFile(configFile)
			if err != nil {
				return err
			}

			log, err := getLogger(configs.Logger)
			if err != nil {
				return err
			}

			events, err := events.NewEventServer(configs.Events, log)
			if err != nil {
				return err
			}

			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			go events.Run(ctx, cancel)
			select {
			case <-ctx.Done():
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return nil
			}

		},
	}

	cmd.Flags().StringVar(&configFile, "config", "config/config.yaml", "Config file path to which the configs are loaded from")

	return cmd
}
