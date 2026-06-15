package config

// TelemetryConfig holds OpenTelemetry, Prometheus, and pprof settings.
type TelemetryConfig struct {
	Enabled        bool    `yaml:"enabled"`        // start the OTel trace/metric pipeline
	ServiceName    string  `yaml:"serviceName"`    // logical service name for the OTel backend
	OTLPEndpoint   string  `yaml:"otlpEndpoint"`   // gRPC endpoint of an OTel collector; empty disables OTLP export
	PrometheusPort int     `yaml:"prometheusPort"` // port for /metrics; 0 disables
	PprofPort      int     `yaml:"pprofPort"`      // port for /debug/pprof; 0 disables
	SamplingRatio  float64 `yaml:"samplingRatio"`  // fraction of traces to sample (0.0–1.0)
}
