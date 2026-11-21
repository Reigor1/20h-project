package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	kafka_consumer "github.com/Reigor1/20h-project/internal/core/kafka/consumer"
	kafka_producer "github.com/Reigor1/20h-project/internal/core/kafka/producer"
	activity_dlq "github.com/Reigor1/20h-project/internal/features/activity/dlq"
)

const (
	pollTimeout = 100 * time.Millisecond
	readTimeout = 30 * time.Second
)

// main runs the DLQ replay command and exits with a non-zero status on failure.
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Activity DLQ Replay failed:", err)
		os.Exit(1)
	}
}

// run reads one DLQ record at the requested partition and offset, validates its source metadata,
// and republishes the original key and payload to the recorded source topic and partition.
func run() error {
	partition := flag.Int("partition", -1, "DLQ partition")
	offset := flag.Int64("offset", -1, "DLQ offset")
	flag.Parse()

	if *partition < 0 {
		return fmt.Errorf("partition must be provided and non-negative")
	}

	if *offset < 0 {
		return fmt.Errorf("offset must be provided and non-negative")
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer cancel()

	consumerConfig, err := kafka_consumer.NewConfig()
	if err != nil {
		return fmt.Errorf("load Kafka Consumer config: %w", err)
	}

	consumerConfig.GroupID = fmt.Sprintf(
		"activity-dlq-replay-%d",
		os.Getpid(),
	)
	consumerConfig.ClientID = "20h-project-activity-dlq-replay"

	consumer, err := kafka_consumer.NewConsumer(consumerConfig)
	if err != nil {
		return fmt.Errorf("initialize DLQ Consumer: %w", err)
	}
	defer consumer.Close()

	if err := consumer.Assign(
		consumerConfig.DLQTopic,
		int32(*partition),
		*offset,
	); err != nil {
		return fmt.Errorf("assign DLQ partition: %w", err)
	}

	readCtx, cancelRead := context.WithTimeout(ctx, readTimeout)
	defer cancelRead()

	var message *kafka_consumer.Message

	for message == nil {
		if err := readCtx.Err(); err != nil {
			return fmt.Errorf("read DLQ message: %w", err)
		}

		result := consumer.Poll(pollTimeout)

		if result.Err != nil {
			if result.Fatal {
				return fmt.Errorf(
					"fatal DLQ Consumer error: %w",
					result.Err,
				)
			}

			fmt.Fprintf(
				os.Stderr,
				"recoverable DLQ Consumer error: %v\n",
				result.Err,
			)

			continue
		}

		if result.Message != nil {
			message = result.Message
		}
	}

	if message.Topic != consumerConfig.DLQTopic {
		return fmt.Errorf(
			"unexpected topic: got %q, want %q",
			message.Topic,
			consumerConfig.DLQTopic,
		)
	}

	if message.Partition != int32(*partition) ||
		message.Offset != *offset {
		return fmt.Errorf(
			"unexpected DLQ position: got partition=%d offset=%d, want partition=%d offset=%d",
			message.Partition,
			message.Offset,
			*partition,
			*offset,
		)
	}

	record, err := activity_dlq.Unmarshal(message.Value)
	if err != nil {
		return fmt.Errorf("decode DLQ record: %w", err)
	}

	if record.SourceTopic == consumerConfig.DLQTopic {
		return fmt.Errorf("replay target must not be the DLQ topic")
	}

	producerConfig, err := kafka_producer.NewConfig()
	if err != nil {
		return fmt.Errorf("load Kafka Producer config: %w", err)
	}

	producer, err := kafka_producer.NewProducer(producerConfig, nil)
	if err != nil {
		return fmt.Errorf("initialize replay Producer: %w", err)
	}
	defer producer.Close(30 * time.Second)

	replayCtx, cancelReplay := context.WithTimeout(ctx, 60*time.Second)
	defer cancelReplay()

	sourcePartition := record.SourcePartition

	if err := producer.Publish(
		replayCtx,
		record.SourceTopic,
		kafka_producer.Message{
			Key:       record.OriginalKey,
			Value:     record.OriginalPayload,
			Partition: &sourcePartition,
		},
	); err != nil {
		return fmt.Errorf("publish replayed event: %w", err)
	}

	fmt.Printf(
		"DLQ message replayed: event_id=%q source_topic=%q source_partition=%d source_offset=%d\n",
		record.EventID,
		record.SourceTopic,
		record.SourcePartition,
		record.SourceOffset,
	)

	return nil
}
