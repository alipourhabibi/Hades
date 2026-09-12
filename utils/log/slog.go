package log

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alipourhabibi/Hades/config"
)

// logFileMode is the permission a log file is created with.
//
// 0600, not 0666. Debug-level output carries request metadata, and a
// world-writable log file is also a world-writable audit trail.
const logFileMode = 0o600

// replaceAttr honours the key and format settings the config file documents.
//
// Without it the slog engine read four fields out of the whole logger block and
// silently ignored the rest, so a file asking for messageKey: message and
// levelFormat: lowercase produced "msg" and "INFO". The same configuration
// therefore produced two different log schemas depending on which engine was
// selected, and anything parsing the output, a shipper, an alert rule, a test
// harness, had to know which.
//
// Four of the documented keys have no slog equivalent and are left to zap:
// nameKey (slog has no logger name), stacktraceKey (no automatic stack
// capture), durationFormat (slog renders a Duration by its String method), and
// the sampling block (zap's sampler has no counterpart). They are marked
// zap-only in config/sample.yaml rather than left to look effective.
func replaceAttr(c config.Logger) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		// Only the built-in top-level attributes are renamed. A user attribute
		// that happens to be called "time" inside a group is not ours to touch.
		if len(groups) > 0 {
			return a
		}
		switch a.Key {
		case slog.TimeKey:
			if c.TimeKey != "" {
				a.Key = c.TimeKey
			}
			if t, ok := a.Value.Any().(time.Time); ok {
				a.Value = formatTime(c.TimeFormat, t)
			}
		case slog.LevelKey:
			if c.LevelKey != "" {
				a.Key = c.LevelKey
			}
			if lvl, ok := a.Value.Any().(slog.Level); ok {
				a.Value = formatLevel(c.LevelFormat, lvl)
			}
		case slog.MessageKey:
			if c.MessageKey != "" {
				a.Key = c.MessageKey
			}
		case slog.SourceKey:
			if c.CallerKey != "" {
				a.Key = c.CallerKey
			}
			if src, ok := a.Value.Any().(*slog.Source); ok && c.CallerFormat == "short" {
				a.Value = slog.StringValue(fmt.Sprintf("%s:%d", filepath.Base(src.File), src.Line))
			}
		}
		return a
	}
}

// formatTime renders a timestamp in the configured spelling. An unrecognised
// value keeps slog's default, which is RFC3339 with nanoseconds.
func formatTime(format string, t time.Time) slog.Value {
	switch format {
	case "ISO8601":
		return slog.StringValue(t.Format("2006-01-02T15:04:05.000Z0700"))
	case "RFC3339":
		return slog.StringValue(t.Format(time.RFC3339))
	case "RFC3339Nano":
		return slog.StringValue(t.Format(time.RFC3339Nano))
	case "epoch":
		return slog.Int64Value(t.Unix())
	case "epoch_millis":
		return slog.Int64Value(t.UnixMilli())
	case "epoch_nanos":
		return slog.Int64Value(t.UnixNano())
	default:
		return slog.TimeValue(t)
	}
}

// formatLevel renders a level in the configured spelling.
//
// The two colour spellings zap offers are treated as their plain equivalents:
// ANSI escapes in a structured log are a terminal convenience that corrupts the
// field for anything that reads it, and slog has no colour support to inherit.
func formatLevel(format string, lvl slog.Level) slog.Value {
	switch format {
	case "lowercase":
		return slog.StringValue(strings.ToLower(lvl.String()))
	case "capital", "capitalColor", "color":
		return slog.StringValue(strings.ToUpper(lvl.String()))
	default:
		return slog.StringValue(lvl.String())
	}
}

// NewWithConfig constructs an slog-backed LoggerWrapper from the given config.
func NewWithConfig(c config.Logger) (*LoggerWrapper, error) {
	var level slog.Level
	err := level.UnmarshalText([]byte(c.Level))
	if err != nil {
		return nil, err
	}

	opts := slog.HandlerOptions{
		AddSource:   c.AddSource,
		Level:       level,
		ReplaceAttr: replaceAttr(c),
	}

	var output *os.File

	switch c.Output {
	case Stdout, "":
		output = os.Stdout
	case Stderr:
		output = os.Stderr
	default:
		output, err = os.OpenFile(c.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
		// 0600, not 0666. Debug-level output carries request metadata, and a
		// world-writable log file is also a world-writable audit trail.
		if err != nil {
			return nil, fmt.Errorf("failed to open output file: %w", err)
		}
	}

	var handler slog.Handler

	switch c.Format {
	case JsonFormat:
		handler = slog.NewJSONHandler(output, &opts)
	case TextFormat:
		handler = slog.NewTextHandler(output, &opts)
	default:
		handler = slog.NewTextHandler(output, &opts)
	}

	logger := slog.New(handler)

	return &LoggerWrapper{
		Logger: logger,
		file:   output,
	}, nil

}
