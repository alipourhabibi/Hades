package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alipourhabibi/Hades/config"
	server "github.com/alipourhabibi/Hades/internal/hades"
	"github.com/alipourhabibi/Hades/internal/telemetry"
	"github.com/spf13/cobra"
)

// telemetryShutdownTimeout bounds the final span and metric flush. It runs on a
// fresh context because the request context is already cancelled by then.
const telemetryShutdownTimeout = 10 * time.Second

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

			// SIGINT and SIGTERM cancel the context, which is what releases the
			// graceful drain in SchemaRegistryServer.Run. Without this the
			// process dies immediately on Ctrl-C, docker stop or a pod
			// eviction, and the configured server.shutdownTimeout never runs.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			shutdownTelemetry, err := telemetry.Setup(ctx, configs.Telemetry)
			if err != nil {
				return fmt.Errorf("telemetry setup: %w", err)
			}
			defer func() {
				// A fresh context: ctx is cancelled by the time this runs, and
				// flushing with a cancelled context drops exactly the spans that
				// describe whatever caused the shutdown.
				flushCtx, cancelFlush := context.WithTimeout(context.Background(), telemetryShutdownTimeout)
				defer cancelFlush()
				if err := shutdownTelemetry(flushCtx); err != nil {
					fmt.Fprintf(os.Stderr, "telemetry shutdown: %v\n", err)
				}
			}()

			srv, err := server.NewServer(ctx, configs)
			if err != nil {
				return err
			}
			defer func() {
				if err := srv.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "server shutdown: %v\n", err)
				}
			}()

			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				srv.Run(runCtx, cancel)
			}()
			<-runCtx.Done()

			// Wait for Run to finish draining rather than returning as soon as
			// the context is cancelled, so in-flight requests complete.
			<-done
			return nil
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "config/dev.yaml", "path to the YAML configuration file")

	return cmd
}
