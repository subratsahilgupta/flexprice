package kafka

import (
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-kafka/v2/pkg/kafka"
	"github.com/ThreeDotsLabs/watermill/message"
)

type Producer struct {
	publisher message.Publisher
}

func NewProducer(brokers []string) (*Producer, error) {
	publisher, err := kafka.NewPublisher(
		kafka.PublisherConfig{
			Brokers:   brokers,
			Marshaler: kafka.DefaultMarshaler{},
		},
		watermill.NewStdLogger(false, false),
	)
	if err != nil {
		return nil, err
	}

	return &Producer{publisher: publisher}, nil
}

func (p *Producer) Publish(topic string, payload []byte) error {
	return p.PublishWithID(topic, payload, watermill.NewUUID())
}

func (p *Producer) PublishWithID(topic string, payload []byte, id string) error {
	if id == "" {
		id = watermill.NewUUID()
	}

	msg := message.NewMessage(id, payload)
	return p.publisher.Publish(topic, msg)
}

func (p *Producer) Close() error {
	return p.publisher.Close()
}
