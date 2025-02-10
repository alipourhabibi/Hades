package events

import (
	"context"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/events/consumers/sdk"
	"github.com/alipourhabibi/Hades/utils/log"
)

type EventServer struct {
	sdk *sdk.SDKConsumer

	logger *log.LoggerWrapper
}

func NewEventServer(config config.Events, l *log.LoggerWrapper) (*EventServer, error) {
	sdkConsumer := sdk.NewSDKConsumer(config, l)
	err := sdkConsumer.Setup()
	if err != nil {
		return nil, err
	}

	return &EventServer{
		logger: l,
		sdk:    sdkConsumer,
	}, nil
}

func (e *EventServer) Run(ctx context.Context, cancel context.CancelFunc) {
	go e.sdk.Consume()
	<-ctx.Done()
}
