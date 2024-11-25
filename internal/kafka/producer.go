package kafka

import (
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-kafka/v2/pkg/kafka"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/types"
)

// Update the kafka producer to implement an interface
type MessageProducer interface {
	PublishWithID(topic string, payload []byte, id string) error
	Close() error
}

// Update the Producer struct to implement MessageProducer
type Producer struct {
	publisher message.Publisher
}

func NewProducer(cfg *config.Configuration) (MessageProducer, error) {
	enableDebugLogs := cfg.Logging.Level == types.LogLevelDebug

	saramaConfig := GetSaramaConfig(cfg)
	if saramaConfig != nil {
		// add producer configs
		saramaConfig.Producer.Return.Successes = true
		saramaConfig.Producer.Return.Errors = true
	}

	publisher, err := kafka.NewPublisher(
		kafka.PublisherConfig{
			Brokers:               cfg.Kafka.Brokers,
			Marshaler:             kafka.DefaultMarshaler{},
			OverwriteSaramaConfig: saramaConfig,
		},
		watermill.NewStdLogger(enableDebugLogs, enableDebugLogs),
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
