package main

import (
	"context"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/server"
	"github.com/alipourhabibi/Hades/storage/db"
	"github.com/alipourhabibi/Hades/storage/gitaly"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var (
		configFile string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "serve the Hades server",
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

			gitalyStorage, err := gitaly.NewStorage(configs.Gitaly)
			if err != nil {
				return err
			}

			ctx := context.Background()
			server, err := server.NewServer(
				ctx,
				configs,
				server.WithDB(db),
				server.WithLogger(log),
				server.WithGitaly(gitalyStorage),
			)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(ctx)
			go server.Run(ctx, cancel)
			select {
			case <-ctx.Done():
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "config/config.yaml", "Config file path to which the configs are loaded from")

	return cmd
}
