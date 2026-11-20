package kafka_consumer

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

var ErrAssignmentLost = errors.New("Kafka partition assignment lost")

type partitionKey struct {
	topic     string
	partition int32
}

type Message struct {
	Value     []byte
	Key       []byte
	Headers   []kafka.Header
	Topic     string
	Partition int32
	Offset    int64

	assignmentEpoch uint64

	native *kafka.Message
}

type PollResult struct {
	Message *Message
	Err     error
	Fatal   bool
}

type Consumer struct {
	client *kafka.Consumer

	assigned        map[partitionKey]struct{}
	assignmentEpoch map[partitionKey]uint64

	paused       bool
	rebalanceErr error
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

		"auto.offset.reset": config.AutoOffsetReset,

		// librdkafka reconnect policy.
		"reconnect.backoff.ms":     1000,
		"reconnect.backoff.max.ms": 10000,
		"socket.keepalive.enable":  true,

		// Worker continues polling during retry backoff.
		"max.poll.interval.ms": 300000,
	})
	if err != nil {
		return nil, fmt.Errorf("create Kafka Consumer: %w", err)
	}

	return &Consumer{
		client:          client,
		assigned:        make(map[partitionKey]struct{}),
		assignmentEpoch: make(map[partitionKey]uint64),
	}, nil
}

func (c *Consumer) Subscribe(topic string) error {
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("Kafka topic must not be empty")
	}

	if err := c.client.SubscribeTopics([]string{topic}, c.handleRebalance); err != nil {
		return fmt.Errorf("subscribe to Kafka topic %q: %w", topic, err)
	}

	return nil
}

func (c *Consumer) handleRebalance(
	client *kafka.Consumer,
	event kafka.Event,
) error {
	switch e := event.(type) {
	case kafka.AssignedPartitions:
		protocol := client.GetRebalanceProtocol()

		var err error

		if protocol == "COOPERATIVE" {
			err = client.IncrementalAssign(e.Partitions)
		} else {
			err = client.Assign(e.Partitions)
		}

		if err != nil {
			c.rebalanceErr = fmt.Errorf(
				"assign Kafka partitions: %w",
				err,
			)
			return c.rebalanceErr
		}

		if protocol != "COOPERATIVE" {
			c.assigned = make(map[partitionKey]struct{})
		}

		for _, partition := range e.Partitions {
			key := getPartitionKey(partition)

			c.assigned[key] = struct{}{}

			if _, exists := c.assignmentEpoch[key]; !exists {
				c.assignmentEpoch[key] = 0
			}
		}

		// Keep newly assigned partitions paused while retry is in progress.
		if c.paused && len(e.Partitions) > 0 {
			if err := client.Pause(e.Partitions); err != nil {
				c.rebalanceErr = fmt.Errorf(
					"pause newly assigned Kafka partitions: %w",
					err,
				)
				return c.rebalanceErr
			}
		}

	case kafka.RevokedPartitions:
		protocol := client.GetRebalanceProtocol()
		assignmentLost := client.AssignmentLost()

		var err error

		if assignmentLost || protocol != "COOPERATIVE" {
			// Eager rebalance revokes the whole assignment.
			// A lost assignment must also be treated as fully revoked.
			for key := range c.assigned {
				c.assignmentEpoch[key]++
				delete(c.assigned, key)
			}

			err = client.Unassign()
		} else {
			// Cooperative rebalance revokes only the listed partitions.
			for _, partition := range e.Partitions {
				key := getPartitionKey(partition)

				c.assignmentEpoch[key]++
				delete(c.assigned, key)
			}

			err = client.IncrementalUnassign(e.Partitions)
		}

		if err != nil {
			c.rebalanceErr = fmt.Errorf(
				"unassign revoked Kafka partitions: %w",
				err,
			)
			return c.rebalanceErr
		}
	}

	return nil
}

func getPartitionKey(partition kafka.TopicPartition) partitionKey {
	var topic string

	if partition.Topic != nil {
		topic = *partition.Topic
	}

	return partitionKey{
		topic:     topic,
		partition: partition.Partition,
	}
}

func (c *Consumer) Pause() error {
	if c.paused {
		return nil
	}

	partitions, err := c.client.Assignment()
	if err != nil {
		return fmt.Errorf("get Kafka Consumer assignment: %w", err)
	}

	if len(partitions) == 0 {
		return fmt.Errorf(
			"Kafka Consumer has no assigned partitions to pause",
		)
	}

	if err := c.client.Pause(partitions); err != nil {
		return fmt.Errorf("pause Kafka Consumer: %w", err)
	}

	c.paused = true

	return nil
}

func (c *Consumer) Resume() error {
	if !c.paused {
		return nil
	}

	partitions, err := c.client.Assignment()
	if err != nil {
		return fmt.Errorf("get Kafka Consumer assignment: %w", err)
	}

	if len(partitions) > 0 {
		if err := c.client.Resume(partitions); err != nil {
			return fmt.Errorf("resume Kafka Consumer: %w", err)
		}
	}

	c.paused = false

	return nil
}

// Assign manually assigns one partition at a specific offset.
// It is used by the DLQ replay command.
func (c *Consumer) Assign(
	topic string,
	partition int32,
	offset int64,
) error {
	topic = strings.TrimSpace(topic)

	if topic == "" {
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

	c.assigned = make(map[partitionKey]struct{})

	key := partitionKey{
		topic:     topic,
		partition: partition,
	}

	c.assigned[key] = struct{}{}

	if _, exists := c.assignmentEpoch[key]; !exists {
		c.assignmentEpoch[key] = 0
	}

	return nil
}

func (c *Consumer) Poll(timeout time.Duration) PollResult {
	c.rebalanceErr = nil

	event := c.client.Poll(int(timeout.Milliseconds()))

	if c.rebalanceErr != nil {
		err := c.rebalanceErr
		c.rebalanceErr = nil

		return PollResult{
			Err:   err,
			Fatal: true,
		}
	}

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

		key := partitionKey{
			topic:     topic,
			partition: e.TopicPartition.Partition,
		}
		epoch := c.assignmentEpoch[key]

		return PollResult{
			Message: &Message{
				Value:           e.Value,
				Key:             e.Key,
				Headers:         e.Headers,
				Topic:           topic,
				Partition:       e.TopicPartition.Partition,
				Offset:          int64(e.TopicPartition.Offset),
				assignmentEpoch: epoch,
				native:          e,
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

func (c *Consumer) IsAssigned(message *Message) bool {
	if message == nil {
		return false
	}

	key := partitionKey{
		topic:     message.Topic,
		partition: message.Partition,
	}

	_, assigned := c.assigned[key]
	if !assigned {
		return false
	}

	return c.assignmentEpoch[key] == message.assignmentEpoch
}

func (c *Consumer) CommitMessage(message *Message) error {
	if message == nil || message.native == nil {
		return fmt.Errorf("Kafka message must not be nil")
	}

	if !c.IsAssigned(message) {
		return ErrAssignmentLost
	}

	_, err := c.client.CommitMessage(message.native)
	if err != nil {
		if !c.IsAssigned(message) {
			return fmt.Errorf("%w: %v", ErrAssignmentLost, err)
		}

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

func isFatalKafkaError(err error) bool {
	var kafkaErr kafka.Error

	return errors.As(err, &kafkaErr) && kafkaErr.IsFatal()
}

func IsRetriableError(err error) bool {
	var kafkaErr kafka.Error

	if !errors.As(err, &kafkaErr) {
		return false
	}

	if kafkaErr.IsFatal() {
		return false
	}

	switch kafkaErr.Code() {
	case kafka.ErrTransport,
		kafka.ErrBrokerNotAvailable,
		kafka.ErrLeaderNotAvailable,
		kafka.ErrNotLeaderForPartition,
		kafka.ErrRequestTimedOut,
		kafka.ErrNetworkException,
		kafka.ErrCoordinatorLoadInProgress,
		kafka.ErrCoordinatorNotAvailable,
		kafka.ErrNotCoordinator,
		kafka.ErrRebalanceInProgress,
		kafka.ErrIllegalGeneration,
		kafka.ErrUnknownMemberID:
		return true

	default:
		return false
	}
}
