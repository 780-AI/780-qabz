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

// payFixtures have Pay and Settle (task 044).
var payFixtures = MockFixtures{
	Inquiry: testFixtures.Inquiry,
	Pay: func(req PaymentRequest) *PaymentResponse {
		return &PaymentResponse{Status: StatusSuccess, Code: 200, ProviderTraceID: "mock-" + req.OrderID}
	},
	Settle: func(SettleRequest) *SettleResponse { return &SettleResponse{Status: StatusSuccess, Code: 200} },
}

// TestMockPay (task 044): success gives the fixture, failed a failed answer, pending an error that
// wraps ErrProviderDown; empty is success; no call to the adapter; a done context is its error.
func TestMockPay(t *testing.T) {
	for rt, want := range map[string]string{MockReturnSuccess: StatusSuccess, MockReturnFailed: StatusFailed, "": StatusSuccess} {
		real := &stubAdapter{name: "parsian"}
		log := &infoLog{}
		m, err := NewMock(real, MockSettings{Enable: true, ReturnType: MockReturnSuccess, PayReturnType: rt}, payFixtures, log)
		if !assert.NoError(t, err, rt) {
			continue
		}
		resp, err := m.Pay(context.Background(), PaymentRequest{OrderID: "o1"})
		if assert.NoError(t, err, rt) && assert.NotNil(t, resp, rt) {
			assert.Equal(t, want, resp.Status, rt)
		}
		assert.Len(t, log.lines, 1, rt)
	}
	m, _ := NewMock(&stubAdapter{name: "parsian"}, MockSettings{Enable: true, ReturnType: MockReturnSuccess, PayReturnType: MockReturnPending}, payFixtures, nil)
	_, err := m.Pay(context.Background(), PaymentRequest{})
	assert.ErrorIs(t, err, ErrProviderDown)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = m.Pay(ctx, PaymentRequest{})
	assert.ErrorIs(t, err, context.Canceled)

	// No Pay fixture: the adapter answers.
	m, _ = NewMock(&stubAdapter{name: "x"}, MockSettings{Enable: true, ReturnType: MockReturnSuccess, PayReturnType: "nonsense"}, testFixtures, nil)
	_, err = m.Pay(context.Background(), PaymentRequest{})
	assert.ErrorIs(t, err, ErrNotSupported, "the pay return type is not checked with no Pay fixture")
	_, err = NewMock(&stubAdapter{name: "x"}, MockSettings{Enable: true, ReturnType: MockReturnSuccess, PayReturnType: MockReturnError}, payFixtures, nil)
	assert.Error(t, err, "error is not a pay return type")
}

// TestMockSettle (task 044): success gives the fixture, error an error that wraps ErrProviderDown;
// with no Settle fixture the adapter answers; an unknown settle return type is rejected.
func TestMockSettle(t *testing.T) {
	m, err := NewMock(&stubAdapter{name: "karashop"}, MockSettings{Enable: true, ReturnType: MockReturnSuccess}, payFixtures, nil)
	if assert.NoError(t, err) {
		resp, err := m.Settle(context.Background(), SettleRequest{})
		assert.NoError(t, err)
		assert.Equal(t, StatusSuccess, resp.Status)
	}
	m, _ = NewMock(&stubAdapter{name: "karashop"}, MockSettings{Enable: true, ReturnType: MockReturnSuccess, SettleReturnType: MockReturnError}, payFixtures, nil)
	_, err = m.Settle(context.Background(), SettleRequest{})
	assert.ErrorIs(t, err, ErrProviderDown)
	m, _ = NewMock(&stubAdapter{name: "x"}, MockSettings{Enable: true, ReturnType: MockReturnSuccess}, testFixtures, nil)
	_, err = m.Settle(context.Background(), SettleRequest{})
	assert.ErrorIs(t, err, ErrNotSupported)
	_, err = NewMock(&stubAdapter{name: "x"}, MockSettings{Enable: true, ReturnType: MockReturnSuccess, SettleReturnType: MockReturnPending}, payFixtures, nil)
	assert.Error(t, err)
	// The inquiry return type error does not change Pay and Settle.
	m, _ = NewMock(&stubAdapter{name: "x"}, MockSettings{Enable: true, ReturnType: MockReturnError}, payFixtures, nil)
	resp, err := m.Settle(context.Background(), SettleRequest{})
	assert.NoError(t, err)
	assert.Equal(t, StatusSuccess, resp.Status)
}

// TestTelServiceStatus: bill's map of the three Karashop texts; any other text gives "".
func TestTelServiceStatus(t *testing.T) {
	assert.Equal(t, TelServiceActive, TelServiceStatus("وصل"))
	assert.Equal(t, TelServicePartiallyDisconnected, TelServiceStatus("قطع یکطرفه"))
	assert.Equal(t, TelServiceFullyDisconnected, TelServiceStatus("قطع دوطرفه"))
	assert.Empty(t, TelServiceStatus("other"))
	assert.Empty(t, TelServiceStatus(""))
}
