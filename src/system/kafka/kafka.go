// Package kafka is the Kafka producer of Qabz (task 045, supertask 043 decision C). It uses the
// official Confluent client (confluent-kafka-go, librdkafka; operator, 2026-11-15) in place of
// bill's kafka-go writer, and keeps bill's partition rule (leastBytes).
package kafka

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	ckafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"qabz/common/monitoring/jaeger"
)

const (
	// metadataTTL is how long the partition list of a topic is kept (the MetadataTTL default of the
	// kafka-go Transport that bill's writer uses).
	metadataTTL = 6 * time.Second
	// metadataTimeout limits a metadata read when ctx has no deadline.
	metadataTimeout = 5 * time.Second
)

// Options configure New.
type Options struct {
	// Brokers are the addresses of the cluster (kafka.brokers). At least one.
	Brokers []string
	// ClientID is the Kafka client ID (service.ecosystem_name, task 045 D17). Not empty.
	ClientID string
	// OnPublish gets the result of each Publish (ok, error) and its duration. Nil: nothing is
	// recorded.
	OnPublish func(result string, d time.Duration)
}

// Publish results, for Options.OnPublish (a fixed set: metric labels).
const (
	ResultOK    = "ok"
	ResultError = "error"
)

// Producer writes messages to Kafka. It is safe for concurrent use.
type Producer struct {
	p         *ckafka.Producer
	bal       leastBytes
	onPublish func(string, time.Duration)
	now       func() time.Time

	mu    sync.Mutex
	parts map[string]cachedPartitions
	done  chan struct{} // closed when the events loop ends
	once  sync.Once
}

type cachedPartitions struct {
	ids []int32
	at  time.Time
}

// @todo docs/this_vs_bill.md item 44: the Confluent client with acks=all, no batch wait and the
// client ID service.ecosystem_name (bill: kafka-go, no acks, a 1 s batch, an empty client ID).
//
// New creates the producer. It connects lazily: a wrong address fails on the first Publish.
// bootstrap.servers is the brokers joined with commas and client.id is opts.ClientID; every other
// setting is the librdkafka default (acks=all).
func New(opts Options) (*Producer, error) {
	if len(opts.Brokers) == 0 {
		return nil, errors.New("kafka: at least one broker is required")
	}
	if opts.ClientID == "" {
		return nil, errors.New("kafka: the client ID is required")
	}
	p, err := ckafka.NewProducer(&ckafka.ConfigMap{
		"bootstrap.servers": strings.Join(opts.Brokers, ","),
		"client.id":         opts.ClientID,
	})
	if err != nil {
		return nil, fmt.Errorf("kafka: producer: %w", err)
	}
	if p == nil {
		return nil, errors.New("kafka: NewProducer returned nil")
	}
	pr := &Producer{p: p, onPublish: opts.OnPublish, now: time.Now, parts: map[string]cachedPartitions{}, done: make(chan struct{})}
	go pr.drainEvents()
	return pr, nil
}

// drainEvents reads the events channel of the producer until it is closed. Every Publish gets its
// delivery report on its own channel, so this channel has only client-level events (for example
// "all brokers down"); a failed publish already reports its error, so they are dropped here.
func (p *Producer) drainEvents() {
	defer close(p.done)
	for range p.p.Events() {
	}
}

// Publish writes one message (key, value, no headers) to the topic, on the partition of bill's rule,
// and waits for its delivery report or for ctx. A failed metadata read, an error in the report, or
// a done ctx is an error. When ctx ends first, librdkafka can still deliver the message later. Each
// publish is a span kafka.Publish, a child of the span in ctx.
func (p *Producer) Publish(ctx context.Context, topic string, key, value []byte) error {
	if p == nil || p.p == nil {
		return errors.New("kafka: producer not created with New")
	}
	// The producer span (task 047 decision D9): the topic of the config and the result of
	// qabz_kafka_publish_total. The message gets no trace header (conventions § 17 exception).
	ctx, span := otel.Tracer("qabz/system/kafka").Start(ctx, "kafka.Publish", trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(attribute.String("messaging.system", "kafka"), attribute.String("messaging.destination.name", topic)))
	defer span.End()
	start := p.now()
	err := p.publish(ctx, topic, key, value)
	result := ResultOK
	if err != nil {
		result = ResultError
		jaeger.Fail(span, result, err)
	}
	span.SetAttributes(attribute.String("result", result))
	if p.onPublish != nil {
		p.onPublish(result, p.now().Sub(start))
	}
	return err
}

func (p *Producer) publish(ctx context.Context, topic string, key, value []byte) error {
	parts, err := p.partitions(ctx, topic)
	if err != nil {
		return err
	}
	partition := p.bal.balance(len(key)+len(value), parts)
	reports := make(chan ckafka.Event, 1) // buffered: a late report never blocks librdkafka
	err = p.p.Produce(&ckafka.Message{
		TopicPartition: ckafka.TopicPartition{Topic: &topic, Partition: partition},
		Key:            key,
		Value:          value,
	}, reports)
	if err != nil {
		return fmt.Errorf("kafka: produce to %s: %w", topic, err)
	}
	select {
	case e := <-reports:
		m, ok := e.(*ckafka.Message)
		if !ok || m == nil {
			return fmt.Errorf("kafka: produce to %s: unexpected report %v", topic, e)
		}
		if m.TopicPartition.Error != nil {
			return fmt.Errorf("kafka: produce to %s: %w", topic, m.TopicPartition.Error)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("kafka: produce to %s: %w", topic, ctx.Err())
	}
}

// partitions returns the partition IDs of the topic, read from the cluster metadata and kept for
// metadataTTL.
func (p *Producer) partitions(ctx context.Context, topic string) ([]int32, error) {
	p.mu.Lock()
	c, ok := p.parts[topic]
	p.mu.Unlock()
	if ok && p.now().Sub(c.at) < metadataTTL {
		return c.ids, nil
	}
	timeout := metadataTimeout
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("kafka: metadata of %s: %w", topic, err)
	}
	if timeout < time.Millisecond {
		timeout = time.Millisecond
	}
	md, err := p.p.GetMetadata(&topic, false, int(timeout/time.Millisecond))
	if err != nil {
		return nil, fmt.Errorf("kafka: metadata of %s: %w", topic, err)
	}
	if md == nil {
		return nil, fmt.Errorf("kafka: metadata of %s: no answer", topic)
	}
	tm, ok := md.Topics[topic]
	if !ok {
		return nil, fmt.Errorf("kafka: metadata of %s: the topic is not in the answer", topic)
	}
	if tm.Error.Code() != ckafka.ErrNoError {
		return nil, fmt.Errorf("kafka: metadata of %s: %w", topic, tm.Error)
	}
	if len(tm.Partitions) == 0 {
		return nil, fmt.Errorf("kafka: metadata of %s: the topic has no partition", topic)
	}
	ids := make([]int32, 0, len(tm.Partitions))
	for _, pm := range tm.Partitions {
		ids = append(ids, pm.ID)
	}
	p.mu.Lock()
	p.parts[topic] = cachedPartitions{ids: ids, at: p.now()}
	p.mu.Unlock()
	return ids, nil
}

// Close sends the messages in the queue until ctx is done, then closes the producer. It returns an
// error when messages were left. It is safe to call more than once.
func (p *Producer) Close(ctx context.Context) error {
	if p == nil || p.p == nil {
		return nil
	}
	var err error
	p.once.Do(func() {
		timeout := metadataTimeout
		if dl, ok := ctx.Deadline(); ok {
			timeout = time.Until(dl)
		}
		left := 0
		if timeout > 0 {
			left = p.p.Flush(int(timeout / time.Millisecond))
		} else {
			left = p.p.Len()
		}
		p.p.Close()
		<-p.done
		if left > 0 {
			err = fmt.Errorf("kafka: %d message(s) not sent at the close", left)
		}
	})
	return err
}
