package sdk

import (
	"context"
	"log/slog"
	"time"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/events/consumers"
	"github.com/alipourhabibi/Hades/utils/log"
	amqp "github.com/rabbitmq/amqp091-go"
)

type SDKConsumer struct {
	delivery  <-chan amqp.Delivery
	logger    *slog.Logger
	msgsQueue chan amqp.Delivery
	handlers  map[string]consumers.HandlerFunc
	channel   *amqp.Channel
	hostUrl   string
}

const msgsQueueLength = 10
const workersCount = 5

func NewSDKConsumer(config config.Events, l *log.LoggerWrapper) *SDKConsumer {
	return &SDKConsumer{
		hostUrl:   config.Host,
		logger:    l.With("layer", "SDKConsumer"),
		handlers:  map[string]consumers.HandlerFunc{},
		msgsQueue: make(chan amqp.Delivery, msgsQueueLength),
	}
}

func (s *SDKConsumer) Setup() error {
	//TODO config channel here
	conn, err := amqp.Dial(s.hostUrl)
	if err != nil {
		return err
	}

	s.channel, err = conn.Channel()
	if err != nil {
		return err
	}

	q, err := s.channel.QueueDeclare("hades.sdk", true, false,
		false, false, nil)
	if err != nil {
		return err
	}
	err = s.channel.QueueBind(q.Name, "hades.sdk.repositoryPushed", "hades_sdk", false, nil)
	if err != nil {
		return err
	}
	d, err := s.channel.Consume(q.Name, "sdk_consumer", false,
		false, false, false, nil)
	if err != nil {
		return err
	}
	s.delivery = d

	s.registerHandler("hades.sdk.repositoryPushed", s.repositoryPushed)

	return nil
}

func (s *SDKConsumer) Consume() {
	for i := 0; i < workersCount; i++ {
		go s.worker()
	}
	for msg := range s.delivery {
		s.msgsQueue <- msg
	}
}

func (s *SDKConsumer) registerHandler(routingKey string, handler consumers.HandlerFunc) {
	s.handlers[routingKey] = handler
}

func (s *SDKConsumer) worker() {
	lg := s.logger.With("method", "worker")
	for msg := range s.msgsQueue {
		func(msg amqp.Delivery) {
			lg.Info("rabbit message received in msg queue go channel", slog.String("routingKey", msg.RoutingKey))

			handler, ok := s.handlers[msg.RoutingKey]
			if !ok {
				lg.Warn("no handler found for routingKey", slog.String("routingKey", msg.RoutingKey))
				if err := msg.Ack(false); err != nil {
					lg.Error("failed to ack message", slog.Any("error", err))
				}
				lg.Warn("rabbit message acked(no handler found)", slog.String("routingKey", msg.RoutingKey))
				return
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel()
				err := handler(ctx, msg.Body)
				if err != nil {
					if err := msg.Nack(false, false); err != nil {
						lg.Error("failed to nack message", slog.Any("error", err))
						return
					}
					lg.Warn("rabbit message nacked", slog.String("routingKey", msg.RoutingKey))
					return
				}
				if err := msg.Ack(false); err != nil {
					lg.Error("failed to ack message", slog.Any("error", err))
					return
				}
				lg.Info("rabbit message acked", slog.String("routingKey", msg.RoutingKey))
				return
			}

		}(msg)
	}
}
