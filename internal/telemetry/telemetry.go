// Package telemetry initialises OpenTelemetry tracing and metrics for the
// Hades server. It supports both OTLP push (traces and metrics) and
// Prometheus pull (metrics). When telemetry is disabled Setup returns a
// no-op shutdown function so callers need no nil check.
package telemetry

import (
	"context"
	"errors"
	"time"

	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/alipourhabibi/Hades/config"
)

// Setup initialises the global OTel trace and metric providers.
// It returns a shutdown function that must be called before the process exits
// to flush in-flight telemetry data.
func Setup(ctx context.Context, cfg config.TelemetryConfig) (shutdown func(context.Context) error, err error) {
	if !cfg.Enabled {
		// Still initialise metrics with the default no-op global provider so
		// package-level metric variables are never nil.
		_ = InitMetrics()
		return func(context.Context) error { return nil }, nil
	}

	if cfg.SamplingRatio == 0 {
		cfg.SamplingRatio = 1.0
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "hades"
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, err
	}

	var shutdownFns []func(context.Context) error
	// shutdown calls all cleanup functions and joins errors.
	shutdown = func(ctx context.Context) error {
		var errs []error
		for _, fn := range shutdownFns {
			errs = append(errs, fn(ctx))
		}
		return errors.Join(errs...)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tp, err := newTraceProvider(ctx, cfg, res)
	if err != nil {
		return shutdown, err
	}
	shutdownFns = append(shutdownFns, tp.Shutdown)
	otel.SetTracerProvider(tp)

	mp, err := newMeterProvider(ctx, cfg, res)
	if err != nil {
		return shutdown, err
	}
	shutdownFns = append(shutdownFns, mp.Shutdown)
	otel.SetMeterProvider(mp)

	if err := InitMetrics(); err != nil {
		return shutdown, err
	}

	// Start Go runtime metrics (heap, goroutines, GC).
	if err := otelruntime.Start(otelruntime.WithMinimumReadMemStatsInterval(15 * time.Second)); err != nil {
		return shutdown, err
	}

	return shutdown, nil
}

func newTraceProvider(ctx context.Context, cfg config.TelemetryConfig, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SamplingRatio)),
	}

	if cfg.OTLPEndpoint != "" {
		conn, err := grpc.NewClient(cfg.OTLPEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, err
		}
		exp, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithGRPCConn(conn),
			otlptracegrpc.WithTimeout(5*time.Second),
		)
		if err != nil {
			return nil, err
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	}

	return sdktrace.NewTracerProvider(opts...), nil
}

func newMeterProvider(ctx context.Context, cfg config.TelemetryConfig, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	var readers []sdkmetric.Option

	// Prometheus reader (pull-based scrape).
	if cfg.PrometheusPort > 0 {
		promExp, err := prometheus.New()
		if err != nil {
			return nil, err
		}
		readers = append(readers, sdkmetric.WithReader(promExp))
	}

	// OTLP push-based reader.
	if cfg.OTLPEndpoint != "" {
		conn, err := grpc.NewClient(cfg.OTLPEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, err
		}
		exp, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithGRPCConn(conn),
			otlpmetricgrpc.WithTimeout(5*time.Second),
		)
		if err != nil {
			return nil, err
		}
		readers = append(readers, sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(15*time.Second)),
		))
	}

	opts := append([]sdkmetric.Option{sdkmetric.WithResource(res)}, readers...)
	return sdkmetric.NewMeterProvider(opts...), nil
}

// Tracer returns a named tracer from the global provider.
func Tracer(name string) trace.Tracer {
	return otel.GetTracerProvider().Tracer(name)
}

// Meter returns a named meter from the global provider.
func Meter(name string) metric.Meter {
	return otel.GetMeterProvider().Meter(name)
}
