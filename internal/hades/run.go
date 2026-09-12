package hades

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/alipourhabibi/Hades/internal/sdk/generate"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/internal/sdk/worker"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Run starts background goroutines (pprof, Prometheus, SDK worker) and the
// HTTP listener. It cancels the context on fatal errors.
func (s *SchemaRegistryServer) Run(ctx context.Context, cancel context.CancelFunc) {
	// Diagnostic listeners.
	//
	// They start only when telemetry is enabled, not merely when a port is set.
	// dev.yaml sets enabled: false and both ports, so both used to start
	// anyway, and /metrics served an empty registry because InitMetrics had
	// bound to the no-op provider.
	//
	// /debug/pprof/heap dumps process memory, which routinely contains live
	// session tokens and personal access tokens, and neither listener
	// authenticates. Binding either to a non-loopback address therefore needs
	// telemetry.allowPublicDiagnostics as well as a bind address.
	var diagnostics []*http.Server
	if s.config.Telemetry.Enabled {
		if s.config.Telemetry.PprofPort > 0 {
			pprofMux := http.NewServeMux()
			pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
			pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
			addr, err := s.diagnosticAddr(s.config.Telemetry.PprofPort)
			if err != nil {
				s.logger.Error("refusing to start the pprof listener", "error", err)
				cancel()
				return
			}
			diagnostics = append(diagnostics, s.startDiagnostic("pprof", addr, pprofMux))
		}

		if s.config.Telemetry.PrometheusPort > 0 {
			promMux := http.NewServeMux()
			promMux.Handle("/metrics", promhttp.Handler())
			addr, err := s.diagnosticAddr(s.config.Telemetry.PrometheusPort)
			if err != nil {
				s.logger.Error("refusing to start the metrics listener", "error", err)
				cancel()
				return
			}
			diagnostics = append(diagnostics, s.startDiagnostic("prometheus metrics", addr, promMux))
		}
	} else if s.config.Telemetry.PprofPort > 0 || s.config.Telemetry.PrometheusPort > 0 {
		s.logger.Info("diagnostic ports are configured but telemetry.enabled is false; not starting them")
	}

	if s.config.SDK.Enabled {
		go newSDKWorker(s, s.serverSet.SDKBackend).Run(ctx)
	}

	mux, err := s.newServerMux()
	if err != nil {
		s.logger.Error("failed to create server mux", "error", err)
		cancel()
		return
	}

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
		Handler:           mux,
		ReadHeaderTimeout: durationOr(s.config.Server.ReadHeaderTimeout, 15*time.Second),
		ReadTimeout:       durationOr(s.config.Server.ReadTimeout, 5*time.Minute),
		IdleTimeout:       durationOr(s.config.Server.IdleTimeout, 120*time.Second),
	}

	// Protocols, not h2c.NewHandler.
	//
	// golang.org/x/net/http2/h2c is deprecated in favour of the standard
	// library's Protocols field, which is also more honest about what it does:
	// unencrypted HTTP/2 is enabled only on the plaintext listener. The
	// previous code wrapped the handler unconditionally, so the TLS path also
	// carried the h2c upgrade machinery it can never use.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	if s.certFile == "" {
		protocols.SetUnencryptedHTTP2(true)
	} else {
		protocols.SetHTTP2(true)
	}
	srv.Protocols = protocols

	if s.certFile != "" {
		// TLS 1.2 floor. Without a TLSConfig the standard library accepts
		// whatever the runtime default is, which is not a decision this server
		// has made.
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	// Stop accepting and drain in-flight requests when the context is cancelled,
	// so shutdown does not sever an upload midway through its transaction.
	shutdownDone := make(chan struct{})
	// #nosec G118 -- deliberate: the drain must outlive the cancelled request context, and it is bounded by shutdownTimeout.
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
		// The diagnostic listeners are drained too. Only the main server was,
		// so pprof and /metrics stayed up after shutdown had begun.
		for _, d := range diagnostics {
			if err := d.Shutdown(shutdownCtx); err != nil {
				s.logger.Error("diagnostic listener shutdown failed", "addr", d.Addr, "error", err)
			}
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

// startDiagnostic runs one diagnostic listener and returns it so it can be
// drained on shutdown.
//
// The ListenAndServe error is logged. Discarding it, which is what this did,
// made a port conflict completely silent: the endpoint simply was not there.
func (s *SchemaRegistryServer) startDiagnostic(name, addr string, handler http.Handler) *http.Server {
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	s.logger.Info(name+" server listening", "addr", addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error(name+" server stopped", "addr", addr, "error", err)
		}
	}()
	return srv
}

// diagnosticAddr builds the listen address for pprof and Prometheus.
//
// The default bind is loopback. A non-loopback bind additionally requires
// telemetry.allowPublicDiagnostics, because a pprof heap dump contains live
// credentials and neither endpoint authenticates: publishing them has to be a
// stated decision rather than a side effect of setting an address.
func (s *SchemaRegistryServer) diagnosticAddr(port int) (string, error) {
	bindAddr := s.config.Telemetry.BindAddr
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	if !isLoopback(bindAddr) && !s.config.Telemetry.AllowPublicDiagnostics {
		return "", fmt.Errorf(
			"telemetry.bindAddr is %q, which is not loopback: set telemetry.allowPublicDiagnostics to publish "+
				"unauthenticated diagnostics that expose process memory", bindAddr)
	}
	return fmt.Sprintf("%s:%d", bindAddr, port), nil
}

// isLoopback reports whether addr names only the local machine.
func isLoopback(addr string) bool {
	if addr == "localhost" {
		return true
	}
	ip := net.ParseIP(addr)
	return ip != nil && ip.IsLoopback()
}

// newSDKWorker builds a worker that generates SDK artifacts after each push.
func newSDKWorker(s *SchemaRegistryServer, backend sdkstorage.Backend) *worker.Worker {
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
	).WithNotifications(s.db.Notification())
}
