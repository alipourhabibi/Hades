package main

import (
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/utils/log"
)

// getLogger builds a logger from the given configuration. Falls back
// to slog when the configured engine is not recognised.
func getLogger(loggerConfig config.Logger) (*log.LoggerWrapper, error) {
	switch loggerConfig.Engine {
	case log.Zap:
		return log.NewZapWithConfig(loggerConfig)
	default:
		return log.NewWithConfig(loggerConfig)
	}
}
