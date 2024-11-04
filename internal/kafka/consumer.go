package kafka

import (
	"context"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-kafka/v2/pkg/kafka"
	"github.com/ThreeDotsLabs/watermill/message"
)

type Consumer struct {
	subscriber message.Subscriber
}

func NewConsumer(brokers []string, consumerGroup string) (*Consumer, error) {
	subscriber, err := kafka.NewSubscriber(
		kafka.SubscriberConfig{
			Brokers:               brokers,
			Unmarshaler:           kafka.DefaultMarshaler{},
			OverwriteSaramaConfig: kafka.DefaultSaramaSubscriberConfig(),
			ConsumerGroup:         consumerGroup,
		},
		watermill.NewStdLogger(false, false),
	)
	if err != nil {
		return nil, err
	}

	return &Consumer{subscriber: subscriber}, nil
}

func (c *Consumer) Subscribe(topic string) (<-chan *message.Message, error) {
	return c.subscriber.Subscribe(context.Background(), topic)
}

func (c *Consumer) Close() error {
	return c.subscriber.Close()
}
