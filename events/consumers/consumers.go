package consumers

import "context"

type HandlerFunc func(context.Context, []byte) error

type Consumer interface {
	Setup() error
	Consume()
}
