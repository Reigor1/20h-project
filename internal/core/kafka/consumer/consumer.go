package kafka_consumer

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

type Message struct {
	Value     []byte
	Key       []byte
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

	pausedPartitions []kafka.TopicPartition
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

		// Connection recovery: initial and maximum reconnect backoff.
		"reconnect.backoff.ms":     1000,
		"reconnect.backoff.max.ms": 10000,

		// Enable TCP keepalive for broker connections.
		"socket.keepalive.enable": true,

		// Maximum allowed interval between consecutive Poll() calls.
		"max.poll.interval.ms": 300000,
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

func (c *Consumer) Pause() error {
	if len(c.pausedPartitions) > 0 {
		return nil
	}

	partitions, err := c.client.Assignment()
	if err != nil {
		return fmt.Errorf("get Kafka Consumer assignment: %w", err)
	}

	if len(partitions) == 0 {
		return fmt.Errorf("Kafka Consumer has no assigned partitions to pause")
	}

	if err := c.client.Pause(partitions); err != nil {
		return fmt.Errorf("pause Kafka Consumer: %w", err)
	}

	c.pausedPartitions = partitions

	return nil
}

func (c *Consumer) Resume() error {
	if len(c.pausedPartitions) == 0 {
		return nil
	}

	if err := c.client.Resume(c.pausedPartitions); err != nil {
		return fmt.Errorf("resume Kafka Consumer: %w", err)
	}

	c.pausedPartitions = nil

	return nil
}

func (c *Consumer) Assign(
	topic string,
	partition int32,
	offset int64,
) error {
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("Kafka topic must not be empty")
	}

	if partition < 0 {
		return fmt.Errorf("Kafka partition must not be negative")
	}

	if offset < 0 {
		return fmt.Errorf("Kafka offset must not be negative")
	}

	topicPartition := kafka.TopicPartition{
		Topic:     &topic,
		Partition: partition,
		Offset:    kafka.Offset(offset),
	}

	if err := c.client.Assign([]kafka.TopicPartition{topicPartition}); err != nil {
		return fmt.Errorf("assign Kafka partition: %w", err)
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
				Err:   e.TopicPartition.Error,
				Fatal: isFatalKafkaError(e.TopicPartition.Error),
			}
		}

		var topic string

		if e.TopicPartition.Topic != nil {
			topic = *e.TopicPartition.Topic
		}

		return PollResult{
			Message: &Message{
				Key:       e.Key,
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

func isFatalKafkaError(err error) bool {
	var kafkaErr kafka.Error

	return errors.As(err, &kafkaErr) && kafkaErr.IsFatal()
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
