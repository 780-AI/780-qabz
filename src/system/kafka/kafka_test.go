package kafka

import (
	"context"
	"errors"
	"math/rand/v2"
	"sort"
	"sync"
	"testing"
	"time"

	ckafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
)

// billLeastBytes is the rule of kafka-go v0.4.31 LeastBytes.Balance as bill's writer runs it,
// written out from the module source (balancer.go) for the differential test. It is not used by
// the producer.
type billLeastBytes struct {
	counters []struct {
		partition int
		bytes     uint64
	}
}

func (lb *billLeastBytes) Balance(keyLen, valueLen int, partitions ...int) int {
	if len(partitions) != len(lb.counters) {
		lb.counters = make([]struct {
			partition int
			bytes     uint64
		}, len(partitions))
		for i, p := range partitions {
			lb.counters[i].partition = p
		}
		sort.Slice(lb.counters, func(i, j int) bool { return lb.counters[i].partition < lb.counters[j].partition })
	}
	minBytes := lb.counters[0].bytes
	minIndex := 0
	for i, c := range lb.counters[1:] {
		if c.bytes < minBytes {
			minIndex = i + 1
			minBytes = c.bytes
		}
	}
	c := &lb.counters[minIndex]
	c.bytes += uint64(keyLen) + uint64(valueLen)
	return c.partition
}

// TestLeastBytesFixed: a fixed sequence with ties, the lowest partition first, and a change of the
// partition count (all counts start again at 0).
func TestLeastBytesFixed(t *testing.T) {
	var lb leastBytes
	parts := []int32{2, 0, 1} // unsorted, as metadata can give them
	var got []int32
	for _, size := range []int{10, 10, 10, 5, 30, 1} {
		got = append(got, lb.balance(size, parts))
	}
	// 0:10 | 1:10 | 2:10 | tie -> 0 (15) | 1 (40) | 2 (11)
	assert.Equal(t, []int32{0, 1, 2, 0, 1, 2}, got)

	got = got[:0]
	for _, size := range []int{100, 1, 1} {
		got = append(got, lb.balance(size, []int32{0, 1, 2, 3}))
	}
	assert.Equal(t, []int32{0, 1, 2}, got, "a new partition count starts at 0 again")
}

// TestLeastBytesDifferential: on random sizes and partition lists, the producer rule gives the same
// partition as bill's rule for every message.
func TestLeastBytesDifferential(t *testing.T) {
	r := rand.New(rand.NewPCG(45, 2026))
	var ours leastBytes
	var bill billLeastBytes
	lists := [][]int32{{0, 1, 2}, {5, 3, 4, 0}, {0}, {1, 0}, {0, 1, 2, 3, 4, 5, 6, 7}}
	list := lists[0]
	for i := range 5000 {
		if i%700 == 0 {
			list = lists[r.IntN(len(lists))]
		}
		keyLen, valueLen := r.IntN(8), r.IntN(400)
		ints := make([]int, len(list))
		for j, p := range list {
			ints[j] = int(p)
		}
		want := bill.Balance(keyLen, valueLen, ints...)
		if !assert.Equal(t, int32(want), ours.balance(keyLen+valueLen, list), "message %d", i) {
			return
		}
	}
}

func TestNewErrors(t *testing.T) {
	_, err := New(Options{ClientID: "bill"})
	assert.ErrorContains(t, err, "at least one broker")
	_, err = New(Options{Brokers: []string{"127.0.0.1:1"}})
	assert.ErrorContains(t, err, "client ID")

	var nilProducer *Producer
	assert.Error(t, nilProducer.Publish(context.Background(), "t", nil, nil))
	assert.NoError(t, nilProducer.Close(context.Background()))
}

// mockCluster starts librdkafka's in-process mock cluster with the topic and its partitions.
func mockCluster(t *testing.T, topic string, partitions int) *ckafka.MockCluster {
	t.Helper()
	mc, err := ckafka.NewMockCluster(1)
	if !assert.NoError(t, err) || !assert.NotNil(t, mc) {
		t.FailNow()
	}
	t.Cleanup(mc.Close)
	if !assert.NoError(t, mc.CreateTopic(topic, partitions, 1)) {
		t.FailNow()
	}
	return mc
}

// readAll reads every message of the topic from the start of each partition.
func readAll(t *testing.T, bootstrap, topic string, partitions, want int) []*ckafka.Message {
	t.Helper()
	c, err := ckafka.NewConsumer(&ckafka.ConfigMap{"bootstrap.servers": bootstrap, "group.id": "qabz-test", "enable.auto.commit": false})
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	defer func() { _ = c.Close() }()
	var tps []ckafka.TopicPartition
	for p := range partitions {
		tps = append(tps, ckafka.TopicPartition{Topic: &topic, Partition: int32(p), Offset: ckafka.OffsetBeginning})
	}
	if !assert.NoError(t, c.Assign(tps)) {
		t.FailNow()
	}
	var got []*ckafka.Message
	deadline := time.Now().Add(15 * time.Second)
	for len(got) < want && time.Now().Before(deadline) {
		m, err := c.ReadMessage(500 * time.Millisecond)
		if err != nil {
			continue
		}
		got = append(got, m)
	}
	return got
}

// TestPublishMockCluster: each message reaches the topic with its key and value, no headers, on the
// partition of bill's rule; OnPublish counts each publish.
func TestPublishMockCluster(t *testing.T) {
	const topic = "paymentwriter"
	mc := mockCluster(t, topic, 3)
	var mu sync.Mutex
	results := map[string]int{}
	p, err := New(Options{Brokers: []string{mc.BootstrapServers()}, ClientID: "bill", OnPublish: func(result string, d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		results[result]++
		assert.GreaterOrEqual(t, d, time.Duration(0))
	}})
	if !assert.NoError(t, err) || !assert.NotNil(t, p) {
		return
	}
	defer func() { assert.NoError(t, p.Close(context.Background())) }()

	values := []string{`{"a":1}`, `{"b":22}`, `{"c":333}`, `{"d":4}`}
	var ref leastBytes
	wantPart := map[string]int32{}
	for _, v := range values {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := p.Publish(ctx, topic, []byte("bill"), []byte(v))
		cancel()
		if !assert.NoError(t, err, v) {
			return
		}
		wantPart[v] = ref.balance(len("bill")+len(v), []int32{0, 1, 2})
	}
	got := readAll(t, mc.BootstrapServers(), topic, 3, len(values))
	if !assert.Len(t, got, len(values)) {
		return
	}
	for _, m := range got {
		if !assert.NotNil(t, m) {
			continue
		}
		assert.Equal(t, "bill", string(m.Key))
		assert.Empty(t, m.Headers)
		assert.Equal(t, wantPart[string(m.Value)], m.TopicPartition.Partition, string(m.Value))
	}
	assert.Equal(t, map[string]int{ResultOK: len(values)}, results)
}

// TestPublishErrors: a done ctx is an error, counted as error.
func TestPublishErrors(t *testing.T) {
	mc := mockCluster(t, "known", 1)
	var mu sync.Mutex
	results := map[string]int{}
	p, err := New(Options{Brokers: []string{mc.BootstrapServers()}, ClientID: "bill", OnPublish: func(result string, _ time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		results[result]++
	}})
	if !assert.NoError(t, err) || !assert.NotNil(t, p) {
		return
	}
	defer func() { _ = p.Close(context.Background()) }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = p.Publish(ctx, "known", []byte("bill"), []byte("x"))
	assert.True(t, errors.Is(err, context.Canceled), "%v", err)
	assert.Equal(t, map[string]int{ResultError: 1}, results)
}

// TestPublishNoBroker: with no broker that answers, Publish fails when ctx ends.
func TestPublishNoBroker(t *testing.T) {
	p, err := New(Options{Brokers: []string{"127.0.0.1:1"}, ClientID: "bill"})
	if !assert.NoError(t, err) || !assert.NotNil(t, p) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	assert.Error(t, p.Publish(ctx, "t", []byte("bill"), []byte("x")))
	cctx, ccancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer ccancel()
	assert.NoError(t, p.Close(cctx), "nothing in the queue")
	assert.NoError(t, p.Close(cctx), "a second close does nothing")
}
