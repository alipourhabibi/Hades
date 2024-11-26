package config

type Logger struct {
	Engine string `json:"engine" yaml:"engine"` // slog, zap

	// General configs
	Level     string `json:"level" yaml:"level"`         // debug, info, warn, error
	Format    string `json:"format" yaml:"format"`       // json or text
	Output    string `json:"output" yaml:"output"`       // stdout, stderr, or file path
	AddSource bool   `json:"addSource" yaml:"addSource"` // Include caller information

	ZapLogger `yaml:",inline"`
}

type ZapLogger struct {
	// Encoder settings
	TimeFormat     string `json:"timeFormat" yaml:"timeFormat"`         // ISO8601, RFC3339, RFC3339Nano, epoch, epoch_millis, epoch_nanos
	LevelFormat    string `json:"levelFormat" yaml:"levelFormat"`       // lowercase, capital, capitalColor, color
	DurationFormat string `json:"durationFormat" yaml:"durationFormat"` // string, nanos, ms
	CallerFormat   string `json:"callerFormat" yaml:"callerFormat"`     // full, short

	// Custom key names
	TimeKey       string `json:"timeKey" yaml:"timeKey"`
	LevelKey      string `json:"levelKey" yaml:"levelKey"`
	NameKey       string `json:"nameKey" yaml:"nameKey"`
	CallerKey     string `json:"callerKey" yaml:"callerKey"`
	MessageKey    string `json:"messageKey" yaml:"messageKey"`
	StacktraceKey string `json:"stacktraceKey" yaml:"stacktraceKey"`

	// Sampling configuration
	Sampling *SamplingConfig `json:"sampling" yaml:"sampling"`
}

type SamplingConfig struct {
	Initial    int `json:"initial" yaml:"initial"`       // Sample the first N entries
	Thereafter int `json:"thereafter" yaml:"thereafter"` // Sample every Nth entry after Initial
}
