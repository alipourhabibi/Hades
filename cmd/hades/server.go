package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"

	"github.com/alipourhabibi/Hades/config"
	server "github.com/alipourhabibi/Hades/internal/hades"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly"
	"github.com/alipourhabibi/Hades/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

			log, err := getLogger(configs.Logger)
			if err != nil {
				return err
			}

			ctx := context.Background()

			shutdownTelemetry, err := telemetry.Setup(ctx, configs.Telemetry)
			if err != nil {
				return fmt.Errorf("telemetry setup: %w", err)
			}
			defer func() { _ = shutdownTelemetry(ctx) }()

			if configs.Telemetry.PprofPort > 0 {
				go func() {
					pprofMux := http.NewServeMux()
					pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
					pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
					pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
					pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
					pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
					addr := fmt.Sprintf(":%d", configs.Telemetry.PprofPort)
					log.Info("pprof server listening", "addr", addr)
					_ = http.ListenAndServe(addr, pprofMux)
				}()
			}

			if configs.Telemetry.PrometheusPort > 0 {
				go func() {
					promMux := http.NewServeMux()
					promMux.Handle("/metrics", promhttp.Handler())
					addr := fmt.Sprintf(":%d", configs.Telemetry.PrometheusPort)
					log.Info("prometheus metrics server listening", "addr", addr)
					_ = http.ListenAndServe(addr, promMux)
				}()
			}

			db, err := db.New(configs.DB, log)
			if err != nil {
				return err
			}

			gitalyStorage, err := gitaly.NewStorage(configs.Gitaly)
			if err != nil {
				return err
			}

			srv, err := server.NewServer(
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
			go srv.Run(ctx, cancel)
			<-ctx.Done()
			return nil
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "config/config.yaml", "path to the YAML configuration file")

	return cmd
}
