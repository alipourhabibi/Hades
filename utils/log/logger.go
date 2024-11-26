package log

import (
	"log/slog"
	"os"
)

const (
	JsonFormat = "json"
	TextFormat = "text"

	Stdout = "stdout"
	Stderr = "stderr"

	Slog = "slog"
	Zap  = "zap"

	// Time encoding
	ISO8601     = "ISO8601"
	RFC3339     = "RFC3339"
	RFC3339Nano = "RFC3339Nano"
	Epoch       = "epoch"
	EpochMillis = "epoch_millis"
	EpochNanos  = "epoch_nanos"

	// Level encoding
	Capital      = "capital"
	CapitalColor = "capitalColor"
	Color        = "color"

	// Duration encoding
	String = "string"
	Nanos  = "nanos"
	MS     = "ms"

	// Caller encoding
	Full  = "full"
	Short = "short"
)

// LoggerWrapper is a logger wrapper holding the logger and the file it opens
type LoggerWrapper struct {
	*slog.Logger
	file *os.File
}

func (l *LoggerWrapper) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// DefaultLogger return a logger wrapper with default logger
func DefaultLogger() *LoggerWrapper {
	return &LoggerWrapper{
		Logger: slog.Default(),
	}
}
