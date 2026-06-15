// Package log provides a pluggable structured logging abstraction backed
// by either slog or zap. The LoggerWrapper satisfies the *slog.Logger
// interface and can optionally close an output file on shutdown.
package log

import (
	"log/slog"
	"os"
)

// Format constants.
const (
	JsonFormat = "json"
	TextFormat = "text"
)

// Output targets.
const (
	Stdout = "stdout"
	Stderr = "stderr"
)

// Engine identifiers.
const (
	Slog = "slog"
	Zap  = "zap"
)

// Zap time encoding names.
const (
	ISO8601     = "ISO8601"
	RFC3339     = "RFC3339"
	RFC3339Nano = "RFC3339Nano"
	Epoch       = "epoch"
	EpochMillis = "epoch_millis"
	EpochNanos  = "epoch_nanos"
)

// Zap level encoding names.
const (
	Capital      = "capital"
	CapitalColor = "capitalColor"
	Color        = "color"
)

// Zap duration encoding names.
const (
	String = "string"
	Nanos  = "nanos"
	MS     = "ms"
)

// Zap caller encoding names.
const (
	Full  = "full"
	Short = "short"
)

// LoggerWrapper wraps an slog.Logger and holds a reference to an output
// file, if any, so it can be closed cleanly on shutdown.
type LoggerWrapper struct {
	*slog.Logger
	file *os.File
}

// Close closes the output file, if one was opened.
func (l *LoggerWrapper) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// DefaultLogger returns a LoggerWrapper backed by the default slog logger.
func DefaultLogger() *LoggerWrapper {
	return &LoggerWrapper{
		Logger: slog.Default(),
	}
}
