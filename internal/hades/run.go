package hades

import (
	"context"
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
	if s.config.Telemetry.PprofPort > 0 {
		go func() {
			pprofMux := http.NewServeMux()
			pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
			pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
			addr := fmt.Sprintf(":%d", s.config.Telemetry.PprofPort)
			s.logger.Info("pprof server listening", "addr", addr)
			_ = http.ListenAndServe(addr, pprofMux)
		}()
	}

	if s.config.Telemetry.PrometheusPort > 0 {
		go func() {
			promMux := http.NewServeMux()
			promMux.Handle("/metrics", promhttp.Handler())
			addr := fmt.Sprintf(":%d", s.config.Telemetry.PrometheusPort)
			s.logger.Info("prometheus metrics server listening", "addr", addr)
			_ = http.ListenAndServe(addr, promMux)
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
	if s.certFile == "" {
		s.logger.Info("starting h2c server (no TLS)", "port", s.listenPort)
		err = http.ListenAndServe(fmt.Sprintf(":%d", s.listenPort), handler)
	} else {
		s.logger.Info("starting TLS server", "port", s.listenPort)
		err = http.ListenAndServeTLS(fmt.Sprintf(":%d", s.listenPort), s.certFile, s.keyFile, handler)
	}
	if err != nil {
		s.logger.Error("server stopped", "error", err)
		cancel()
	}
}

// newSDKWorker builds a worker that generates SDK artifacts after each push.
func newSDKWorker(s *SchemaRegistryServer, backend sdkstorage.Backend) (*worker.Worker, error) {
	cfg := s.config.SDK
	generators := make(map[string]*generate.Generator, len(cfg.Generators))
	for _, g := range cfg.Generators {
		generators[g.Plugin] = generate.New(cfg.ProtocBin, g)
	}
	return worker.New(
		s.db.SDKJobStorage,
		s.db.CommitStorage,
		s.gitStorage,
		generators,
		backend,
		s.logger,
		10*time.Second,
		4,
	), nil
}
