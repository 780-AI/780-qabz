package reconcile

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestStatusMessageBytes (task 045): the value is byte for byte the value of bill's json.Marshal
// (encoding/json v1, written out here: v1 is forbidden in this repository, conventions § 1), the
// text of the task file; also for an order ID with the characters that v1 escapes (HTML, U+2028,
// U+2029). An invalid UTF-8 byte is an error (v1 writes the escape \ufffd, which v2 cannot).
func TestStatusMessageBytes(t *testing.T) {
	const orderID = "0b6c1d6e-6f1a-4c3e-9a51-6f8f4a0c0045"
	for _, status := range []string{"success", "failed"} {
		got, err := StatusMessage(orderID, status)
		if !assert.NoError(t, err) {
			continue
		}
		assert.Equal(t, `{"id":"","current_step":"","payload":{"order_id":"`+orderID+`","user_id":"","payment_id":"","service_id":0,"price":0,"rrn":""},"status":"`+status+`","step_status":{"payment":"","service":"`+status+`"},"type":"","version":0,"data":{}}`, string(got))
	}
	got, err := StatusMessage("a<b>&c\u2028d\u2029", "success")
	if assert.NoError(t, err) {
		assert.Equal(t, `{"id":"","current_step":"","payload":{"order_id":"a\u003cb\u003e\u0026c\u2028d\u2029","user_id":"","payment_id":"","service_id":0,"price":0,"rrn":""},"status":"success","step_status":{"payment":"","service":"success"},"type":"","version":0,"data":{}}`, string(got))
	}
	_, err = StatusMessage("a\xff", "success")
	assert.Error(t, err, "an invalid UTF-8 byte is not sent with other bytes than bill's")
}

// fakePublisher records the messages.
type fakePublisher struct {
	topic      string
	key, value []byte
	err        error
	deadline   time.Time // the deadline of the ctx of the last publish
	hasLimit   bool
}

func (f *fakePublisher) Publish(ctx context.Context, topic string, key, value []byte) error {
	f.topic, f.key, f.value = topic, key, value
	f.deadline, f.hasLimit = ctx.Deadline()
	return f.err
}

// TestPaymentReporter (task 045): the topic of the config, the key of the ecosystem name, the value
// of StatusMessage; a failed publish is the error of Report.
func TestPaymentReporter(t *testing.T) {
	p := &fakePublisher{}
	r, err := NewPaymentReporter(p, "paymentwriter", "bill", time.Second)
	if !assert.NoError(t, err) || !assert.NotNil(t, r) {
		return
	}
	assert.NoError(t, r.Report(context.Background(), "o-1", "failed"))
	want, _ := StatusMessage("o-1", "failed")
	assert.Equal(t, "paymentwriter", p.topic)
	assert.Equal(t, "bill", string(p.key))
	assert.Equal(t, string(want), string(p.value))

	p.err = errors.New("kafka down")
	assert.ErrorContains(t, r.Report(context.Background(), "o-1", "success"), "kafka down")

	for _, args := range [][2]string{{"", "bill"}, {"t", ""}} {
		_, err := NewPaymentReporter(p, args[0], args[1], time.Second)
		assert.Error(t, err)
	}
	_, err = NewPaymentReporter(nil, "t", "bill", time.Second)
	assert.Error(t, err)
	_, err = NewPaymentReporter(p, "t", "bill", 0)
	assert.Error(t, err, "the publish timeout must be positive")
}

// TestPaymentReporterTimeout (task 050 D6): each publish waits at most the timeout of the reporter
// (kafka.timeout), also when the ctx of the run has no deadline.
func TestPaymentReporterTimeout(t *testing.T) {
	p := &fakePublisher{}
	r, err := NewPaymentReporter(p, "paymentwriter", "bill", 5*time.Second)
	if !assert.NoError(t, err) {
		return
	}
	before := time.Now()
	assert.NoError(t, r.Report(context.Background(), "o-1", "success"))
	assert.True(t, p.hasLimit, "the publish has a deadline")
	assert.WithinDuration(t, before.Add(5*time.Second), p.deadline, time.Second)
}
