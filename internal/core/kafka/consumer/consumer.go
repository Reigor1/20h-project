package kafka_consumer

import (
	"fmt"
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

type Message struct {
	Value     []byte
	Topic     string
	Partition int32
	Offset    int64

	native *kafka.Message
}

type PollResult struct {
	Message *Message
	Err     error
	Fatal   bool
}

type Consumer struct {
	client *kafka.Consumer
}

func NewConsumer(config Config) (*Consumer, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate Kafka Consumer config: %w", err)
	}

	client, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": config.BootstrapServers,
		"group.id":          config.GroupID,
		"client.id":         config.ClientID,

		// Manual offset management.
		"enable.auto.commit":       false,
		"enable.auto.offset.store": false,

		// Used when the group has no committed offset.
		"auto.offset.reset": config.AutoOffsetReset,
	})
	if err != nil {
		return nil, fmt.Errorf("create Kafka Consumer: %w", err)
	}

	return &Consumer{
		client: client,
	}, nil
}

func (c *Consumer) Subscribe(topic string) error {
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("Kafka topic must not be empty")
	}

	if err := c.client.SubscribeTopics([]string{topic}, nil); err != nil {
		return fmt.Errorf("subscribe to Kafka topic %q: %w", topic, err)
	}

	return nil
}

func (c *Consumer) Poll(timeout time.Duration) PollResult {
	event := c.client.Poll(int(timeout.Milliseconds()))

	switch e := event.(type) {
	case nil:
		return PollResult{}

	case *kafka.Message:
		if e.TopicPartition.Error != nil {
			return PollResult{
				Err: e.TopicPartition.Error,
			}
		}

		var topic string

		if e.TopicPartition.Topic != nil {
			topic = *e.TopicPartition.Topic
		}

		return PollResult{
			Message: &Message{
				Value:     e.Value,
				Topic:     topic,
				Partition: e.TopicPartition.Partition,
				Offset:    int64(e.TopicPartition.Offset),
				native:    e,
			},
		}

	case kafka.Error:
		return PollResult{
			Err:   e,
			Fatal: e.IsFatal(),
		}

	default:
		// Ignore events that do not require application processing.
		return PollResult{}
	}
}

func (c *Consumer) CommitMessage(message *Message) error {
	if message == nil || message.native == nil {
		return fmt.Errorf("Kafka message must not be nil")
	}

	_, err := c.client.CommitMessage(message.native)
	if err != nil {
		return fmt.Errorf("commit Kafka message offset: %w", err)
	}

	return nil
}

func (c *Consumer) Close() error {
	if c == nil || c.client == nil {
		return nil
	}

	return c.client.Close()
}
