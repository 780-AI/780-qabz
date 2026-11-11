package provider

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubAdapter is a ProviderInterface that counts its Inquiry calls.
type stubAdapter struct {
	name  string
	mu    sync.Mutex
	calls int
}

func (s *stubAdapter) Name() string                { return s.name }
func (s *stubAdapter) SupportsInquiry(string) bool { return true }
func (s *stubAdapter) Inquiry(context.Context, InquiryRequest) (*InquiryResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return &InquiryResponse{Status: ResponseSuccess, BillID: "real"}, nil
}
func (s *stubAdapter) Pay(context.Context, PaymentRequest) (*PaymentResponse, error) {
	return nil, ErrNotSupported
}
func (s *stubAdapter) Status(context.Context, StatusRequest) (*StatusResponse, error) {
	return nil, ErrNotSupported
}
func (s *stubAdapter) GetDocument(context.Context, DocumentRequest) (*DocumentResponse, error) {
	return nil, ErrNotSupported
}
func (s *stubAdapter) Settle(context.Context, SettleRequest) (*SettleResponse, error) {
	return nil, ErrNotSupported
}

type infoLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *infoLog) Info(msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, msg)
}

var testFixtures = MockFixtures{Inquiry: func(req InquiryRequest) *InquiryResponse {
	return &InquiryResponse{Status: ResponseSuccess, BillID: req.InquiryID, Inquiries: []InquiryItem{{PayID: "p", Amount: 7}}}
}}

func TestNewMockOffReturnsTheAdapter(t *testing.T) {
	real := &stubAdapter{name: "x"}
	got, err := NewMock(real, MockSettings{Enable: false, ReturnType: MockReturnError}, testFixtures, nil)
	assert.NoError(t, err)
	assert.Same(t, real, got, "off: the same adapter, nothing wrapped")
	// With the mock off the return type and the fixture are not checked.
	got, err = NewMock(real, MockSettings{}, MockFixtures{}, nil)
	assert.NoError(t, err)
	assert.Same(t, real, got)
}

func TestNewMockRejects(t *testing.T) {
	real := &stubAdapter{name: "x"}
	_, err := NewMock(nil, MockSettings{Enable: true, ReturnType: MockReturnSuccess}, testFixtures, nil)
	assert.Error(t, err, "nil adapter")
	_, err = NewMock(real, MockSettings{Enable: true, ReturnType: "maybe"}, testFixtures, nil)
	assert.ErrorContains(t, err, "return type")
	_, err = NewMock(real, MockSettings{Enable: true, ReturnType: MockReturnSuccess}, MockFixtures{}, nil)
	assert.ErrorContains(t, err, "fixture")
}

func TestMockSuccessDoesNotCallTheAdapter(t *testing.T) {
	real := &stubAdapter{name: "x"}
	log := &infoLog{}
	m, err := NewMock(real, MockSettings{Enable: true, ReturnType: MockReturnSuccess}, testFixtures, log)
	if !assert.NoError(t, err) || !assert.NotNil(t, m) {
		return
	}
	assert.Equal(t, "x", m.Name())
	resp, err := m.Inquiry(context.Background(), InquiryRequest{InquiryID: "123"})
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.Equal(t, "123", resp.BillID)
	assert.Equal(t, 0, real.calls, "no call to the real adapter")
	assert.Len(t, log.lines, 1)
	// Every other method is the adapter's.
	_, err = m.Pay(context.Background(), PaymentRequest{})
	assert.ErrorIs(t, err, ErrNotSupported)
}

func TestMockErrorIsProviderDown(t *testing.T) {
	real := &stubAdapter{name: "x"}
	m, err := NewMock(real, MockSettings{Enable: true, ReturnType: MockReturnError}, testFixtures, nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, m) {
		return
	}
	resp, err := m.Inquiry(context.Background(), InquiryRequest{InquiryID: "123"})
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, ErrProviderDown))
	assert.Equal(t, 0, real.calls)
}

func TestMockCanceledContext(t *testing.T) {
	m, err := NewMock(&stubAdapter{name: "x"}, MockSettings{Enable: true, ReturnType: MockReturnSuccess}, testFixtures, nil)
	if !assert.NoError(t, err) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = m.Inquiry(ctx, InquiryRequest{})
	assert.ErrorIs(t, err, context.Canceled)
}
