package brokers

import "context"

type Kick struct {
	Community string `json:"community"`
}

type Input struct {
	Raddr       string `json:"raddr"`
	MessageType int    `json:"messageType"`
	P           []byte `json:"p"`
}

type CommunitiesBroker interface {
	Open(ctx context.Context, brokerURL string) error
	SubscribeToKicks(ctx context.Context, errs chan error) (kicks chan Kick, close func() error)
	SubscribeToInputs(ctx context.Context, errs chan error, community string) (kicks chan Input, close func() error)
	PublishInput(ctx context.Context, input Input, community string) error
	PublishKick(ctx context.Context, kick Kick) error
	Close() error
}
