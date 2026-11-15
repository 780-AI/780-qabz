package reconcile

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
)

// Payment is the message of a final status to the payment service. It is a copy of bill's
// entity.Payment (domain/entity/payment.go): the same fields, order and JSON names, so the bytes
// are the bytes of bill (supertask 043 decision C).
type Payment struct {
	ID          string            `json:"id"`
	CurrentStep string            `json:"current_step"`
	Payload     Payload           `json:"payload"`
	Status      string            `json:"status"`
	StepStatus  StepStatus        `json:"step_status"`
	Type        string            `json:"type"`
	Version     int               `json:"version"`
	Data        map[string]string `json:"data"`
}

// Payload is bill's entity.Payload.
type Payload struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	PaymentID string `json:"payment_id"`
	ServiceID uint32 `json:"service_id"`
	Price     uint32 `json:"price"`
	RRN       string `json:"rrn"`
}

// StepStatus is bill's entity.StepStatus.
type StepStatus struct {
	Payment string `json:"payment"`
	Service string `json:"service"`
}

// StatusMessage returns the value of the status message of an order (bill: verifyPaymentOrder):
// only payload.order_id, status and step_status.service are set; data is an empty map. The bytes are
// the bytes of bill's json.Marshal (encoding/json v1): the struct fields in order and the escapes of
// HTML characters, U+2028 and U+2029. encoding/json/v2 is used because v1 is forbidden (conventions
// § 1). An invalid UTF-8 byte is an error: v1 writes it as the escape \ufffd and v2 cannot, so the
// message is not sent rather than sent with other bytes (the order ID is a UUID text and the status is
// success or failed, so it does not happen).
func StatusMessage(orderID, status string) ([]byte, error) {
	return json.Marshal(Payment{
		Payload:    Payload{OrderID: orderID},
		Status:     status,
		StepStatus: StepStatus{Service: status},
		Data:       map[string]string{},
	},
		json.Deterministic(true),
		jsontext.EscapeForHTML(true),
		jsontext.EscapeForJS(true),
		json.FormatNilMapAsNull(true),
	)
}

// Publisher writes one message (system/kafka.Producer).
type Publisher interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
}

// PaymentReporter reports a final status on Kafka (bill: broker.Client.Publish on
// kafka.topic.payment.write.subject, key "bill").
type PaymentReporter struct {
	publisher Publisher
	topic     string
	key       []byte
}

// NewPaymentReporter returns the reporter. key is service.ecosystem_name (task 045 D17: bill).
func NewPaymentReporter(p Publisher, topic, key string) (*PaymentReporter, error) {
	if p == nil || topic == "" || key == "" {
		return nil, errors.New("reconcile: the publisher, the topic and the key are required")
	}
	return &PaymentReporter{publisher: p, topic: topic, key: []byte(key)}, nil
}

// Report publishes the status message of the order.
func (r *PaymentReporter) Report(ctx context.Context, orderID, status string) error {
	value, err := StatusMessage(orderID, status)
	if err != nil {
		return fmt.Errorf("reconcile: status message: %w", err)
	}
	return r.publisher.Publish(ctx, r.topic, r.key, value)
}
