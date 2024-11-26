package log

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/alipourhabibi/Hades/config"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

// DefaultEncoderConfig returns a base encoder config with sensible defaults
func DefaultEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    "func",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}

// ConfigureEncoder customizes the encoder config based on the provided options
func ConfigureEncoder(cfg *zapcore.EncoderConfig, c config.Logger) {
	// Time encoding
	switch c.TimeFormat {
	case ISO8601:
		cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	case RFC3339:
		cfg.EncodeTime = zapcore.RFC3339TimeEncoder
	case RFC3339Nano:
		cfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	case Epoch:
		cfg.EncodeTime = zapcore.EpochTimeEncoder
	case EpochMillis:
		cfg.EncodeTime = zapcore.EpochMillisTimeEncoder
	case EpochNanos:
		cfg.EncodeTime = zapcore.EpochNanosTimeEncoder
	}

	// Level encoding
	switch c.LevelFormat {
	case Capital:
		cfg.EncodeLevel = zapcore.CapitalLevelEncoder
	case CapitalColor:
		cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	case Color:
		cfg.EncodeLevel = zapcore.LowercaseColorLevelEncoder
	default:
		cfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	}

	// Duration encoding
	switch c.DurationFormat {
	case String:
		cfg.EncodeDuration = zapcore.StringDurationEncoder
	case Nanos:
		cfg.EncodeDuration = zapcore.NanosDurationEncoder
	case MS:
		cfg.EncodeDuration = zapcore.MillisDurationEncoder
	}

	// Caller encoding
	switch c.CallerFormat {
	case Full:
		cfg.EncodeCaller = zapcore.FullCallerEncoder
	case Short:
		cfg.EncodeCaller = zapcore.ShortCallerEncoder
	}

	// Custom key names
	if c.TimeKey != "" {
		cfg.TimeKey = c.TimeKey
	}
	if c.LevelKey != "" {
		cfg.LevelKey = c.LevelKey
	}
	if c.NameKey != "" {
		cfg.NameKey = c.NameKey
	}
	if c.CallerKey != "" {
		cfg.CallerKey = c.CallerKey
	}
	if c.MessageKey != "" {
		cfg.MessageKey = c.MessageKey
	}
	if c.StacktraceKey != "" {
		cfg.StacktraceKey = c.StacktraceKey
	}
}

func NewZapWithConfig(c config.Logger) (*LoggerWrapper, error) {

	var level zapcore.Level
	err := level.UnmarshalText([]byte(c.Level))
	if err != nil {
		return nil, err
	}

	encoderConfig := DefaultEncoderConfig()

	ConfigureEncoder(&encoderConfig, c)

	var encoder zapcore.Encoder
	switch c.Format {
	case JsonFormat:
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	case TextFormat:
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	default:
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	var output zapcore.WriteSyncer
	var file *os.File

	switch c.Output {
	case Stdout, "":
		output = zapcore.AddSync(os.Stdout)
	case Stderr:
		output = zapcore.AddSync(os.Stderr)
	default:
		file, err = os.OpenFile(c.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("failed to open output file: %w", err)
		}
		output = zapcore.AddSync(file)

	}

	var core zapcore.Core
	if c.Sampling != nil {
		core = zapcore.NewSamplerWithOptions(
			zapcore.NewCore(encoder, output, level),
			time.Second,
			c.Sampling.Initial,
			c.Sampling.Thereafter,
		)
	} else {
		core = zapcore.NewCore(encoder, output, level)
	}

	zapOptions := []zapslog.HandlerOption{
		zapslog.WithCaller(c.AddSource),
	}

	logger := slog.New(zapslog.NewHandler(core, zapOptions...))

	return &LoggerWrapper{
		Logger: logger,
		file:   file,
	}, nil
}
