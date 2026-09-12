package hades

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/alipourhabibi/Hades/internal/sdk/generate"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/internal/sdk/worker"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Run starts background goroutines (pprof, Prometheus, SDK worker) and the
// HTTP listener. It cancels the context on fatal errors.
func (s *SchemaRegistryServer) Run(ctx context.Context, cancel context.CancelFunc) {
	// Diagnostic listeners bind to loopback unless a bind address is configured.
	// /debug/pprof/heap dumps process memory, which routinely contains session
	// tokens and secrets, and neither listener has any authentication.
	if s.config.Telemetry.PprofPort > 0 {
		go func() {
			pprofMux := http.NewServeMux()
			pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
			pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
			addr := diagnosticAddr(s.config.Telemetry.BindAddr, s.config.Telemetry.PprofPort)
			s.logger.Info("pprof server listening", "addr", addr)
			srv := &http.Server{Addr: addr, Handler: pprofMux, ReadHeaderTimeout: 10 * time.Second}
			_ = srv.ListenAndServe()
		}()
	}

	if s.config.Telemetry.PrometheusPort > 0 {
		go func() {
			promMux := http.NewServeMux()
			promMux.Handle("/metrics", promhttp.Handler())
			addr := diagnosticAddr(s.config.Telemetry.BindAddr, s.config.Telemetry.PrometheusPort)
			s.logger.Info("prometheus metrics server listening", "addr", addr)
			srv := &http.Server{Addr: addr, Handler: promMux, ReadHeaderTimeout: 10 * time.Second}
			_ = srv.ListenAndServe()
		}()
	}

	if s.config.SDK.Enabled {
		w, err := newSDKWorker(s, s.serverSet.SDKBackend)
		if err != nil {
			s.logger.Error("failed to create SDK worker", "error", err)
		} else {
			go w.Run(ctx)
		}
	}

	mux, err := s.newServerMux()
	if err != nil {
		s.logger.Error("failed to create server mux", "error", err)
		cancel()
		return
	}

	handler := h2c.NewHandler(mux, &http2.Server{})

	// Explicit timeouts. With http.ListenAndServe every timeout is unset, so a
	// client that opens a connection and sends headers one byte at a time holds
	// it open indefinitely.
	//
	// WriteTimeout is deliberately absent: the Go module proxy streams SDK zips
	// whose size is not known in advance, and a write deadline would truncate
	// large downloads. ReadHeaderTimeout and IdleTimeout are what bound the
	// slowloris shapes.
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.listenPort),
		Handler:           handler,
		ReadHeaderTimeout: durationOr(s.config.Server.ReadHeaderTimeout, 15*time.Second),
		ReadTimeout:       durationOr(s.config.Server.ReadTimeout, 5*time.Minute),
		IdleTimeout:       durationOr(s.config.Server.IdleTimeout, 120*time.Second),
	}

	// Stop accepting and drain in-flight requests when the context is cancelled,
	// so shutdown does not sever an upload midway through its transaction.
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		grace := durationOr(s.config.Server.ShutdownTimeout, 30*time.Second)
		s.logger.Info("shutting down server", "grace", grace)
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), grace)
		defer cancelShutdown()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("graceful shutdown failed", "error", err)
		}
	}()

	if s.certFile == "" {
		s.logger.Info("starting h2c server (no TLS)", "port", s.listenPort)
		err = srv.ListenAndServe()
	} else {
		s.logger.Info("starting TLS server", "port", s.listenPort)
		err = srv.ListenAndServeTLS(s.certFile, s.keyFile)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("server stopped", "error", err)
		cancel()
	}

	// Cancel so the shutdown goroutine is released even when the listener died
	// on its own, then wait for the drain to finish before returning.
	cancel()
	<-shutdownDone
}

// durationOr returns d when it is positive, otherwise fallback.
func durationOr(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}

// diagnosticAddr builds the listen address for pprof and Prometheus.
// The default bind address is loopback: both endpoints are unauthenticated and
// expose process internals, so they must be opted in to a public interface.
func diagnosticAddr(bindAddr string, port int) string {
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", bindAddr, port)
}

// newSDKWorker builds a worker that generates SDK artifacts after each push.
func newSDKWorker(s *SchemaRegistryServer, backend sdkstorage.Backend) (*worker.Worker, error) {
	cfg := s.config.SDK
	generators := make(map[string]*generate.Generator, len(cfg.Generators))
	for _, g := range cfg.Generators {
		generators[g.Plugin] = generate.New(cfg.BufBin, g)
	}
	return worker.New(
		s.db.SDKJob(),
		s.db.Commit(),
		s.gitStorage,
		generators,
		backend,
		s.logger,
		10*time.Second,
		4,
	).WithNotifications(s.db.Notification()), nil
}
