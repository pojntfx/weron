package process

import (
	"context"
	"errors"

	"github.com/dustin/go-broadcast"
	"github.com/pojntfx/webrtcfd/internal/brokers"
)

var (
	ErrCouldNotUnmarshalKick  = errors.New("could not unmarshal kick")
	ErrCouldNotUnmarshalInput = errors.New("could not unmarshal input")
)

type CommunitiesBroker struct {
	kicks  broadcast.Broadcaster
	inputs broadcast.Broadcaster
}

func NewCommunitiesBroker() *CommunitiesBroker {
	return &CommunitiesBroker{
		kicks:  broadcast.NewBroadcaster(0),
		inputs: broadcast.NewBroadcaster(0),
	}
}

func (c *CommunitiesBroker) Open(ctx context.Context, brokerURL string) error {
	return nil
}

func (c *CommunitiesBroker) SubscribeToKicks(ctx context.Context, errs chan error) (chan brokers.Kick, func() error) {
	kicks := make(chan brokers.Kick)

	rawKicks := make(chan interface{})
	c.kicks.Register(rawKicks)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case rawKick := <-rawKicks:
				kick, ok := rawKick.(brokers.Kick)
				if !ok {
					errs <- ErrCouldNotUnmarshalKick

					return
				}

				kicks <- kick
			}
		}
	}()

	return kicks, func() error {
		c.inputs.Unregister(rawKicks)

		return nil
	}
}

func (c *CommunitiesBroker) SubscribeToInputs(ctx context.Context, errs chan error, community string) (chan brokers.Input, func() error) {
	inputs := make(chan brokers.Input)

	rawInputs := make(chan interface{})
	c.inputs.Register(rawInputs)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case rawInput := <-rawInputs:
				input, ok := rawInput.(brokers.Input)
				if !ok {
					errs <- ErrCouldNotUnmarshalInput

					return
				}

				inputs <- input
			}
		}
	}()

	return inputs, func() error {
		c.inputs.Unregister(rawInputs)

		return nil
	}
}

func (c *CommunitiesBroker) PublishInput(ctx context.Context, input brokers.Input, community string) error {
	c.inputs.Submit(input)

	return nil
}

func (c *CommunitiesBroker) PublishKick(ctx context.Context, kick brokers.Kick) error {
	c.kicks.Submit(kick)

	return nil
}

func (c *CommunitiesBroker) Close() error {
	if err := c.inputs.Close(); err != nil {
		return err
	}

	return c.kicks.Close()
}
