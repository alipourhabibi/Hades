package main

import (
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/utils/log"
)

func getLogger(loggerConfig config.Logger) (*log.LoggerWrapper, error) {
	switch loggerConfig.Engine {
	case log.Slog:
		return log.NewWithConfig(loggerConfig)
	case log.Zap:
		return log.NewZapWithConfig(loggerConfig)
	default:
		return log.NewWithConfig(loggerConfig)
	}
}
