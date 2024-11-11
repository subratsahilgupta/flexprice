package testutil

import (
	"sync"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/flexprice/flexprice/internal/kafka"
)

var _ kafka.MessageProducer = (*InMemoryMessageBroker)(nil)

type InMemoryMessageBroker struct {
	mu       sync.RWMutex
	messages map[string]map[string]*message.Message
	subs     []chan *message.Message
}

func NewInMemoryMessageBroker() *InMemoryMessageBroker {
	return &InMemoryMessageBroker{
		messages: make(map[string]map[string]*message.Message),
		subs:     make([]chan *message.Message, 0),
	}
}

func (b *InMemoryMessageBroker) PublishWithID(topic string, payload []byte, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	msg := message.NewMessage(id, payload)
	
	if _, exists := b.messages[topic]; !exists {
		b.messages[topic] = make(map[string]*message.Message)
	}
	b.messages[topic][id] = msg

	// Notify subscribers
	for _, ch := range b.subs {
		ch <- msg
	}

	return nil
}

func (b *InMemoryMessageBroker) Subscribe() chan *message.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	ch := make(chan *message.Message, 10)
	b.subs = append(b.subs, ch)
	return ch
}

func (b *InMemoryMessageBroker) HasMessage(topic, id string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if messages, exists := b.messages[topic]; exists {
		_, exists := messages[id]
		return exists
	}
	return false
}

func (b *InMemoryMessageBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	for _, ch := range b.subs {
		close(ch)
	}
	b.subs = nil
	return nil
}
