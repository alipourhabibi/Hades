package log

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/alipourhabibi/Hades/config"
)

func NewWithConfig(c config.Logger) (*LoggerWrapper, error) {

	var level slog.Level
	err := level.UnmarshalText([]byte(c.Level))
	if err != nil {
		return nil, err
	}

	opts := slog.HandlerOptions{
		AddSource: c.AddSource,
		Level:     level,
	}

	var output *os.File

	switch c.Output {
	case Stdout, "":
		output = os.Stdout
	case Stderr:
		output = os.Stderr
	default:
		output, err = os.OpenFile(c.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
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
