package activity_consumer

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	kafka_consumer "github.com/Reigor1/20h-project/internal/core/kafka/consumer"
	kafka_producer "github.com/Reigor1/20h-project/internal/core/kafka/producer"
	activity_dlq "github.com/Reigor1/20h-project/internal/features/activity/dlq"
	"go.uber.org/zap"
)

type fakeConsumer struct {
	sequence *[]string
	results  []kafka_consumer.PollResult

	committed int
	paused    int
	resumed   int
	polls     int

	commitErrors         []error
	assignmentLost       bool
	loseAssignmentOnPoll bool
}

func (c *fakeConsumer) Subscribe(string) error {
	return nil
}

func (c *fakeConsumer) Poll(time.Duration) kafka_consumer.PollResult {
	c.polls++
	if c.loseAssignmentOnPoll {
		c.assignmentLost = true
		c.loseAssignmentOnPoll = false
	}

	if len(c.results) == 0 {
		return kafka_consumer.PollResult{}
	}

	result := c.results[0]
	c.results = c.results[1:]

	return result
}

func (c *fakeConsumer) CommitMessage(
	_ *kafka_consumer.Message,
) error {
	if len(c.commitErrors) > 0 {
		err := c.commitErrors[0]
		c.commitErrors = c.commitErrors[1:]

		return err
	}

	c.committed++

	if c.sequence != nil {
		*c.sequence = append(*c.sequence, "commit")
	}

	return nil
}

func (c *fakeConsumer) IsAssigned(
	_ *kafka_consumer.Message,
) bool {
	return !c.assignmentLost
}

func (c *fakeConsumer) Pause() error {
	c.paused++
	return nil
}

func (c *fakeConsumer) Resume() error {
	c.resumed++
	return nil
}

type fakeProducer struct {
	sequence  *[]string
	errors    []error
	published [][]byte
}

func (p *fakeProducer) Publish(
	_ context.Context,
	_ string,
	message kafka_producer.Message,
) error {
	if p.sequence != nil {
		*p.sequence = append(*p.sequence, "publish")
	}

	p.published = append(p.published, message.Value)

	if len(p.errors) == 0 {
		return nil
	}

	err := p.errors[0]
	p.errors = p.errors[1:]

	return err
}

type fakeActivityService struct {
	sequence  *[]string
	errors    []error
	duplicate bool
	calls              int
	onRecord           func()
	waitForContextDone bool
}

func (s *fakeActivityService) Record(
	ctx context.Context,
	_ core_events.Envelope,
) (bool, error) {
	s.calls++

	if s.sequence != nil {
		*s.sequence = append(*s.sequence, "record")
	}

	if s.onRecord != nil {
		s.onRecord()
	}

	if s.waitForContextDone {
		<-ctx.Done()
		return false, ctx.Err()
	}

	if len(s.errors) == 0 {
		return !s.duplicate, nil
	}

	err := s.errors[0]
	s.errors = s.errors[1:]

	return false, err
}

func newTestWorker(
	t *testing.T,
	consumer *fakeConsumer,
	producer *fakeProducer,
	service *fakeActivityService,
) *Worker {
	t.Helper()

	worker, err := NewWorker(
		"task-events",
		"task-events.dlq",
		time.Millisecond,
		2*time.Millisecond,
		time.Second,
		1000,
		consumer,
		producer,
		service,
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}

	return worker
}

func newTestEventMessage(t *testing.T) *kafka_consumer.Message {
	t.Helper()

	event := core_events.Envelope{
		EventID:          "123e4567-e89b-12d3-a456-426614174000",
		EventType:        core_events.EventTypeTaskCreated,
		SchemaVersion:    core_events.CurrentSchemaVersion,
		OccurredAt:       time.Now().UTC(),
		AggregateType:    core_events.AggregateTypeTask,
		AggregateID:      1,
		AggregateVersion: 1,
		Payload:          json.RawMessage(`{"task":{"id":1}}`),
	}

	payload, err := core_events.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	return &kafka_consumer.Message{
		Value:     payload,
		Key:       []byte("1"),
		Topic:     "task-events",
		Partition: 0,
		Offset:    10,
	}
}

func TestProcessMessageRetriesPostgresBeforeCommit(t *testing.T) {
	var sequence []string

	consumer := &fakeConsumer{
		sequence: &sequence,
	}

	producer := &fakeProducer{
		sequence: &sequence,
	}

	service := &fakeActivityService{
		sequence: &sequence,
		errors: []error{
			errors.New("postgres unavailable"),
			errors.New("postgres timeout"),
		},
	}

	worker := newTestWorker(t, consumer, producer, service)

	var pending []*kafka_consumer.Message

	err := worker.processMessage(
		context.Background(),
		newTestEventMessage(t),
		&pending,
	)
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}

	if service.calls != 3 {
		t.Errorf("Record() calls = %d, want 3", service.calls)
	}

	if consumer.committed != 1 {
		t.Errorf("committed messages = %d, want 1", consumer.committed)
	}

	wantSequence := []string{
		"record",
		"record",
		"record",
		"commit",
	}

	if !reflect.DeepEqual(sequence, wantSequence) {
		t.Errorf("operation sequence = %v, want %v", sequence, wantSequence)
	}

	if consumer.paused != 1 || consumer.resumed != 1 {
		t.Errorf(
			"Pause()/Resume() = %d/%d, want 1/1",
			consumer.paused,
			consumer.resumed,
		)
	}
}

func TestProcessMessagePublishesDLQBeforeCommit(t *testing.T) {
	var sequence []string

	consumer := &fakeConsumer{
		sequence: &sequence,
	}

	producer := &fakeProducer{
		sequence: &sequence,
	}

	worker := newTestWorker(
		t,
		consumer,
		producer,
		&fakeActivityService{},
	)

	message := &kafka_consumer.Message{
		Value:     []byte(`{"event_id":`),
		Key:       []byte("original-key"),
		Topic:     "task-events",
		Partition: 2,
		Offset:    125,
	}

	var pending []*kafka_consumer.Message

	if err := worker.processMessage(
		context.Background(),
		message,
		&pending,
	); err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}

	if len(producer.published) != 1 {
		t.Fatalf("published messages = %d, want 1", len(producer.published))
	}

	record, err := activity_dlq.Unmarshal(producer.published[0])
	if err != nil {
		t.Fatalf("Unmarshal() DLQ record: %v", err)
	}

	if !reflect.DeepEqual(record.OriginalPayload, message.Value) {
		t.Errorf("OriginalPayload = %q, want %q", record.OriginalPayload, message.Value)
	}

	if !reflect.DeepEqual(record.OriginalKey, message.Key) {
		t.Errorf("OriginalKey = %q, want %q", record.OriginalKey, message.Key)
	}

	if record.SourcePartition != message.Partition ||
		record.SourceOffset != message.Offset {
		t.Errorf(
			"source position = %d/%d, want %d/%d",
			record.SourcePartition,
			record.SourceOffset,
			message.Partition,
			message.Offset,
		)
	}

	if consumer.committed != 1 {
		t.Errorf("committed messages = %d, want 1", consumer.committed)
	}

	wantSequence := []string{"publish", "commit"}

	if !reflect.DeepEqual(sequence, wantSequence) {
		t.Errorf("operation sequence = %v, want %v", sequence, wantSequence)
	}
}

func TestProcessMessageRetriesDLQPublishBeforeCommit(t *testing.T) {
	var sequence []string

	consumer := &fakeConsumer{
		sequence: &sequence,
	}

	producer := &fakeProducer{
		sequence: &sequence,
		errors: []error{
			errors.New("Kafka unavailable"),
		},
	}

	worker := newTestWorker(
		t,
		consumer,
		producer,
		&fakeActivityService{},
	)

	message := &kafka_consumer.Message{
		Value:     []byte(`{"event_id":`),
		Topic:     "task-events",
		Partition: 0,
		Offset:    10,
	}

	var pending []*kafka_consumer.Message

	if err := worker.processMessage(
		context.Background(),
		message,
		&pending,
	); err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}

	if len(producer.published) != 2 {
		t.Errorf("publish attempts = %d, want 2", len(producer.published))
	}

	if consumer.committed != 1 {
		t.Errorf("committed messages = %d, want 1", consumer.committed)
	}

	wantSequence := []string{
		"publish",
		"publish",
		"commit",
	}

	if !reflect.DeepEqual(sequence, wantSequence) {
		t.Errorf("operation sequence = %v, want %v", sequence, wantSequence)
	}
}

func TestProcessMessageDoesNotCommitWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	consumer := &fakeConsumer{}
	service := &fakeActivityService{
		onRecord: cancel,
		errors: []error{
			errors.New("postgres unavailable"),
		},
	}

	worker := newTestWorker(
		t,
		consumer,
		&fakeProducer{},
		service,
	)

	var pending []*kafka_consumer.Message

	err := worker.processMessage(
		ctx,
		newTestEventMessage(t),
		&pending,
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("processMessage() error = %v, want context.Canceled", err)
	}

	if consumer.committed != 0 {
		t.Errorf("committed messages = %d, want 0", consumer.committed)
	}
}

func TestProcessMessageKeepsFetchedMessagesDuringRetry(t *testing.T) {
	firstMessage := newTestEventMessage(t)
	secondMessage := newTestEventMessage(t)
	secondMessage.Offset = 11

	consumer := &fakeConsumer{
		results: []kafka_consumer.PollResult{
			{Message: secondMessage},
		},
	}

	service := &fakeActivityService{
		errors: []error{
			errors.New("postgres unavailable"),
		},
	}

	worker := newTestWorker(
		t,
		consumer,
		&fakeProducer{},
		service,
	)

	var pending []*kafka_consumer.Message

	if err := worker.processMessage(
		context.Background(),
		firstMessage,
		&pending,
	); err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("pending messages = %d, want 1", len(pending))
	}

	if pending[0].Offset != secondMessage.Offset {
		t.Errorf(
			"pending offset = %d, want %d",
			pending[0].Offset,
			secondMessage.Offset,
		)
	}

	if consumer.committed != 1 {
		t.Errorf("committed messages = %d, want 1", consumer.committed)
	}
}

func TestProcessMessageMovesPermanentErrorsToDLQ(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*core_events.Envelope)
	}{
		{
			name: "unknown event type",
			modify: func(event *core_events.Envelope) {
				event.EventType = core_events.EventType("task.unknown")
			},
		},
		{
			name: "unsupported schema version",
			modify: func(event *core_events.Envelope) {
				event.SchemaVersion = core_events.CurrentSchemaVersion + 1
			},
		},
		{
			name: "invalid aggregate ID",
			modify: func(event *core_events.Envelope) {
				event.AggregateID = 0
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			eventMessage := newTestEventMessage(t)

			var event core_events.Envelope

			if err := json.Unmarshal(eventMessage.Value, &event); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			testCase.modify(&event)

			payload, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			eventMessage.Value = payload

			consumer := &fakeConsumer{}
			producer := &fakeProducer{}
			service := &fakeActivityService{}

			worker := newTestWorker(t, consumer, producer, service)

			var pending []*kafka_consumer.Message

			if err := worker.processMessage(
				context.Background(),
				eventMessage,
				&pending,
			); err != nil {
				t.Fatalf("processMessage() error = %v", err)
			}

			if len(producer.published) != 1 {
				t.Errorf(
					"DLQ messages = %d, want 1",
					len(producer.published),
				)
			}

			if service.calls != 0 {
				t.Errorf(
					"ActivityService.Record() calls = %d, want 0",
					service.calls,
				)
			}

			if consumer.committed != 1 {
				t.Errorf(
					"committed messages = %d, want 1",
					consumer.committed,
				)
			}
		})
	}
}

func TestRetryDelay(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{name: "first attempt", attempt: 1, want: time.Second},
		{name: "second attempt", attempt: 2, want: 2 * time.Second},
		{name: "third attempt", attempt: 3, want: 4 * time.Second},
		{name: "fourth attempt", attempt: 4, want: 8 * time.Second},
		{name: "fifth attempt", attempt: 5, want: 16 * time.Second},
		{name: "sixth attempt", attempt: 6, want: 30 * time.Second},
		{name: "max delay", attempt: 7, want: 30 * time.Second},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := retryDelay(
				time.Second,
				30*time.Second,
				testCase.attempt,
			)

			if got != testCase.want {
				t.Errorf(
					"retryDelay() = %v, want %v",
					got,
					testCase.want,
				)
			}
		})
	}
}

func TestProcessMessageRetriesCommit(t *testing.T) {
	consumer := &fakeConsumer{
		commitErrors: []error{
			kafka.NewError(
				kafka.ErrTransport,
				"temporary broker transport error",
				false,
			),
		},
	}

	service := &fakeActivityService{}
	worker := newTestWorker(
		t,
		consumer,
		&fakeProducer{},
		service,
	)

	var pending []*kafka_consumer.Message

	err := worker.processMessage(
		context.Background(),
		newTestEventMessage(t),
		&pending,
	)
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}

	if consumer.committed != 1 {
		t.Errorf("committed messages = %d, want 1", consumer.committed)
	}

	if consumer.paused != 1 || consumer.resumed != 1 {
		t.Errorf(
			"Pause()/Resume() = %d/%d, want 1/1",
			consumer.paused,
			consumer.resumed,
		)
	}
}

func TestProcessMessageStopsRetryAfterAssignmentLoss(t *testing.T) {
	consumer := &fakeConsumer{
		loseAssignmentOnPoll: true,
	}

	service := &fakeActivityService{
		errors: []error{
			errors.New("postgres unavailable"),
		},
	}

	worker := newTestWorker(
		t,
		consumer,
		&fakeProducer{},
		service,
	)

	var pending []*kafka_consumer.Message

	err := worker.processMessage(
		context.Background(),
		newTestEventMessage(t),
		&pending,
	)

	if !errors.Is(err, kafka_consumer.ErrAssignmentLost) {
		t.Fatalf(
			"processMessage() error = %v, want ErrAssignmentLost",
			err,
		)
	}

	if consumer.committed != 0 {
		t.Errorf("committed messages = %d, want 0", consumer.committed)
	}

	if consumer.resumed != 1 {
		t.Errorf("Resume() calls = %d, want 1", consumer.resumed)
	}
}

func TestProcessMessageStopsWhenPendingQueueIsFull(t *testing.T) {
	firstMessage := newTestEventMessage(t)
	secondMessage := newTestEventMessage(t)
	secondMessage.Offset = 11
	thirdMessage := newTestEventMessage(t)
	thirdMessage.Offset = 12

	consumer := &fakeConsumer{
		results: []kafka_consumer.PollResult{
			{Message: secondMessage},
			{Message: thirdMessage},
		},
	}

	service := &fakeActivityService{
		errors: []error{
			errors.New("postgres unavailable"),
		},
	}

	worker := newTestWorker(
		t,
		consumer,
		&fakeProducer{},
		service,
	)
	worker.maxPendingMessages = 1
	worker.retryBaseDelay = time.Second

	var pending []*kafka_consumer.Message

	err := worker.processMessage(
		context.Background(),
		firstMessage,
		&pending,
	)
	if !errors.Is(err, ErrPendingQueueFull) {
		t.Fatalf(
			"processMessage() error = %v, want ErrPendingQueueFull",
			err,
		)
	}

	if len(pending) != 1 {
		t.Errorf("pending messages = %d, want 1", len(pending))
	}

	if consumer.committed != 0 {
		t.Errorf("committed messages = %d, want 0", consumer.committed)
	}
}


func TestRunCommitsCurrentMessageAfterShutdownSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first := newTestEventMessage(t)
	second := newTestEventMessage(t)
	second.Offset++

	consumer := &fakeConsumer{
		results: []kafka_consumer.PollResult{
			{Message: first},
			{Message: second},
		},
	}
	service := &fakeActivityService{
		onRecord: cancel,
	}
	worker := newTestWorker(t, consumer, &fakeProducer{}, service)

	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if service.calls != 1 {
		t.Errorf("Record() calls = %d, want 1", service.calls)
	}
	if consumer.committed != 1 {
		t.Errorf("committed messages = %d, want 1", consumer.committed)
	}
	if consumer.polls != 1 {
		t.Errorf("Poll() calls = %d, want 1; no further messages should be fetched", consumer.polls)
	}
}

func TestRunDoesNotCommitWhenCurrentMessageFailsDuringShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumer := &fakeConsumer{
		results: []kafka_consumer.PollResult{
			{Message: newTestEventMessage(t)},
		},
	}
	service := &fakeActivityService{
		onRecord: cancel,
		errors: []error{
			errors.New("postgres unavailable"),
		},
	}
	worker := newTestWorker(t, consumer, &fakeProducer{}, service)

	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v, want graceful stop", err)
	}

	if service.calls != 1 {
		t.Errorf("Record() calls = %d, want 1; no retries should start after shutdown", service.calls)
	}
	if consumer.committed != 0 {
		t.Errorf("committed messages = %d, want 0", consumer.committed)
	}
}

func TestRunLeavesMessageUncommittedWhenShutdownGraceExpires(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumer := &fakeConsumer{
		results: []kafka_consumer.PollResult{
			{Message: newTestEventMessage(t)},
		},
	}
	service := &fakeActivityService{
		onRecord:           cancel,
		waitForContextDone: true,
	}
	worker := newTestWorker(t, consumer, &fakeProducer{}, service)
	worker.shutdownTimeout = 10 * time.Millisecond

	started := time.Now()
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v, want graceful stop after timeout", err)
	}

	if time.Since(started) < 10*time.Millisecond {
		t.Fatal("Run() returned before the shutdown grace period expired")
	}
	if consumer.committed != 0 {
		t.Errorf("committed messages = %d, want 0", consumer.committed)
	}
}

func TestProcessMessageRecognizesDuplicateDelivery(t *testing.T) {
	consumer := &fakeConsumer{}
	service := &fakeActivityService{duplicate: true}
	worker := newTestWorker(t, consumer, &fakeProducer{}, service)

	var pending []*kafka_consumer.Message
	if err := worker.processMessage(
		context.Background(),
		newTestEventMessage(t),
		&pending,
	); err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}

	if consumer.committed != 1 {
		t.Errorf("committed messages = %d, want 1", consumer.committed)
	}
}
