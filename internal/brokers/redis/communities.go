package redis

import (
	"context"
	"encoding/json"

	"github.com/go-redis/redis/v8"
	"github.com/pojntfx/weron/internal/brokers"
)

const (
	topicKick           = "kick"
	topicMessagesPrefix = "messages."
)

type CommunitiesBroker struct {
	client *redis.Client
}

func NewCommunitiesBroker() *CommunitiesBroker {
	return &CommunitiesBroker{}
}

func (c *CommunitiesBroker) Open(ctx context.Context, brokerURL string) error {
	u, err := redis.ParseURL(brokerURL)
	if err != nil {
		return err
	}

	c.client = redis.NewClient(u).WithContext(ctx)

	return nil
}

func (c *CommunitiesBroker) SubscribeToKicks(ctx context.Context, errs chan error) (chan brokers.Kick, func() error) {
	kicks := make(chan brokers.Kick)

	kickPubsub := c.client.Subscribe(ctx, topicKick)
	rawKicks := kickPubsub.Channel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case rawKick := <-rawKicks:
				if rawKick == nil {
					close(kicks)

					// Channel closed
					return
				}

				var kick brokers.Kick
				if err := json.Unmarshal([]byte(rawKick.Payload), &kick); err != nil {
					errs <- err

					return
				}

				kicks <- kick
			}
		}
	}()

	return kicks, kickPubsub.Close
}

func (c *CommunitiesBroker) SubscribeToInputs(ctx context.Context, errs chan error, community string) (chan brokers.Input, func() error) {
	inputs := make(chan brokers.Input)

	inputsPubsub := c.client.Subscribe(ctx, topicMessagesPrefix+community)
	rawKicks := inputsPubsub.Channel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case rawInput := <-rawKicks:
				if rawInput == nil {
					close(inputs)

					// Channel closed
					return
				}

				var input brokers.Input
				if err := json.Unmarshal([]byte(rawInput.Payload), &input); err != nil {
					errs <- err

					return
				}

				inputs <- input
			}
		}
	}()

	return inputs, inputsPubsub.Close
}

func (c *CommunitiesBroker) PublishInput(ctx context.Context, input brokers.Input, community string) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}

	return c.client.Publish(ctx, topicMessagesPrefix+community, data).Err()
}

func (c *CommunitiesBroker) PublishKick(ctx context.Context, kick brokers.Kick) error {
	data, err := json.Marshal(kick)
	if err != nil {
		return err
	}

	return c.client.Publish(ctx, topicKick, data).Err()
}

func (c *CommunitiesBroker) Close() error {
	return c.client.Close()
}
