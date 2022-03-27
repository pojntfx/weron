package process

import (
	"context"
	"errors"

	"github.com/pojntfx/webrtcfd/internal/brokers"
	"github.com/teivah/broadcast"
)

var (
	ErrCouldNotUnmarshalKick  = errors.New("could not unmarshal kick")
	ErrCouldNotUnmarshalInput = errors.New("could not unmarshal input")
)

type CommunitiesBroker struct {
	kicks  *broadcast.Relay[brokers.Kick]
	inputs *broadcast.Relay[brokers.Input]
}

func NewCommunitiesBroker() *CommunitiesBroker {
	return &CommunitiesBroker{
		kicks:  broadcast.NewRelay[brokers.Kick](),
		inputs: broadcast.NewRelay[brokers.Input](),
	}
}

func (c *CommunitiesBroker) Open(ctx context.Context, brokerURL string) error {
	return nil
}

func (c *CommunitiesBroker) SubscribeToKicks(ctx context.Context, errs chan error) (chan brokers.Kick, func() error) {
	kicks := make(chan brokers.Kick)

	l := c.kicks.Listener(1)
	rawKicks := l.Ch()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case kick := <-rawKicks:
				kicks <- kick
			}
		}
	}()

	return kicks, func() error {
		l.Close()

		return nil
	}
}

func (c *CommunitiesBroker) SubscribeToInputs(ctx context.Context, errs chan error, community string) (chan brokers.Input, func() error) {
	inputs := make(chan brokers.Input)

	l := c.inputs.Listener(1)
	rawInputs := l.Ch()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case input := <-rawInputs:
				inputs <- input
			}
		}
	}()

	return inputs, func() error {
		l.Close()

		return nil
	}
}

func (c *CommunitiesBroker) PublishInput(ctx context.Context, input brokers.Input, community string) error {
	c.inputs.Broadcast(input)

	return nil
}

func (c *CommunitiesBroker) PublishKick(ctx context.Context, kick brokers.Kick) error {
	c.kicks.Broadcast(kick)

	return nil
}

func (c *CommunitiesBroker) Close() error {
	c.inputs.Close()
	c.kicks.Close()

	return nil
}
