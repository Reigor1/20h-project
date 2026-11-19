package kafka_producer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

type Message struct {
	Key       []byte
	Value     []byte
	Partition *int32
}

type Producer struct {
	client *kafka.Producer

	onError func(error)

	stopEvents chan struct{}
	eventsDone chan struct{}

	closeOnce sync.Once
	closeErr  error
}

func NewProducer(
	config Config,
	onError func(error),
) (*Producer, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate Kafka producer config: %w", err)
	}

	client, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": config.BootstrapServers,
		"client.id":         config.ClientID,

		// Reliability.
		"acks":                                  "all",
		"enable.idempotence":                    true,
		"max.in.flight.requests.per.connection": 5,

		// Batching.
		"linger.ms":          int(config.Linger.Milliseconds()),
		"batch.size":         config.BatchSizeBytes,
		"batch.num.messages": config.BatchNumMessages,
		"compression.type":   config.CompressionType,

		// Delivery timeout and memory limits.
		"message.timeout.ms":           int(config.MessageTimeout.Milliseconds()),
		"queue.buffering.max.messages": config.QueueMaxMessages,
		"queue.buffering.max.kbytes":   config.QueueMaxKilobytes,
	})
	if err != nil {
		return nil, fmt.Errorf("create Kafka producer: %w", err)
	}

	producer := &Producer{
		client:     client,
		onError:    onError,
		stopEvents: make(chan struct{}),
		eventsDone: make(chan struct{}),
	}

	go producer.watchEvents()

	return producer, nil
}

func (p *Producer) watchEvents() {
	defer close(p.eventsDone)

	for {
		select {
		case event, ok := <-p.client.Events():
			if !ok {
				return
			}

			if kafkaError, ok := event.(kafka.Error); ok {
				if p.onError != nil {
					p.onError(kafkaError)
				}
			}

		case <-p.stopEvents:
			return
		}
	}
}

// PublishBatch enqueues messages asynchronously and waits for their
// individual Delivery Reports.
func (p *Producer) PublishBatch(
	ctx context.Context,
	topic string,
	messages []Message,
) []error {
	results := make([]error, len(messages))

	if len(messages) == 0 {
		return results
	}

	deliveryChannel := make(chan kafka.Event, len(messages))
	pending := make(map[int]struct{}, len(messages))

	for index, message := range messages {
		partition := kafka.PartitionAny

		if message.Partition != nil {
			if *message.Partition < 0 {
				results[index] = fmt.Errorf(
					"Kafka partition must not be negative: %d",
					*message.Partition,
				)
				continue
			}

			partition = *message.Partition
		}

		err := p.client.Produce(
			&kafka.Message{
				TopicPartition: kafka.TopicPartition{
					Topic:     &topic,
					Partition: partition,
				},
				Key:    message.Key,
				Value:  message.Value,
				Opaque: index,
			},
			deliveryChannel,
		)
		if err != nil {
			// The message was not accepted into the local producer queue.
			results[index] = err
			continue
		}

		pending[index] = struct{}{}
	}

	for len(pending) > 0 {
		select {
		case event := <-deliveryChannel:
			delivery, ok := event.(*kafka.Message)
			if !ok {
				for index := range pending {
					results[index] = fmt.Errorf(
						"unexpected Kafka delivery event %T",
						event,
					)
				}
				return results
			}

			index, ok := delivery.Opaque.(int)
			if !ok {
				for pendingIndex := range pending {
					results[pendingIndex] = fmt.Errorf(
						"Kafka delivery report has invalid message metadata",
					)
				}
				return results
			}

			if _, exists := pending[index]; !exists {
				continue
			}

			results[index] = delivery.TopicPartition.Error
			delete(pending, index)

		case <-ctx.Done():
			for index := range pending {
				results[index] = ctx.Err()
			}

			return results
		}
	}

	return results
}

func (p *Producer) Publish(
	ctx context.Context,
	topic string,
	message Message,
) error {
	results := p.PublishBatch(
		ctx,
		topic,
		[]Message{message},
	)

	if len(results) != 1 {
		return fmt.Errorf(
			"unexpected Kafka publish result count: got %d, want 1",
			len(results),
		)
	}

	if results[0] != nil {
		return fmt.Errorf("publish Kafka message: %w", results[0])
	}

	return nil
}

func (p *Producer) Close(timeout time.Duration) error {
	if p == nil || p.client == nil {
		return nil
	}

	p.closeOnce.Do(func() {
		remaining := p.client.Flush(int(timeout.Milliseconds()))

		close(p.stopEvents)
		<-p.eventsDone

		p.client.Close()

		if remaining > 0 {
			p.closeErr = fmt.Errorf(
				"Kafka producer closed with %d outstanding messages",
				remaining,
			)
		}
	})

	return p.closeErr
}
