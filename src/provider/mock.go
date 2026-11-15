package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// The return types of a provider mock (config debug.mocks.<provider>.return_type and the
// <method>_return_type keys). They equal config.MockReturnSuccess, config.MockReturnError,
// config.MockReturnFailed and config.MockReturnPending (a test in app checks it).
const (
	MockReturnSuccess = "success"
	MockReturnError   = "error"
	MockReturnFailed  = "failed"  // pay_return_type only: the provider answers the payment as failed
	MockReturnPending = "pending" // pay_return_type only: no answer (a timeout), the status is not known
)

// MockPayFailedMessage is the message of a payment that the mock answers as failed.
const MockPayFailedMessage = "mock: the payment failed"

// MockSettings are the settings of one provider mock (config debug.mocks.<provider>).
type MockSettings struct {
	Enable     bool
	ReturnType string // the inquiry: MockReturnSuccess or MockReturnError
	// PayReturnType is the payment answer (pay_return_type, task 044): MockReturnSuccess,
	// MockReturnFailed or MockReturnPending. Empty is MockReturnSuccess. Used only when the
	// fixtures have Pay.
	PayReturnType string
	// SettleReturnType is the settlement answer (settle_return_type, task 044): MockReturnSuccess or
	// MockReturnError. Empty is MockReturnSuccess. Used only when the fixtures have Settle.
	SettleReturnType string
}

// MockFixtures are the fixed answers that a provider supplies for its mock. Each function must
// return a new value on each call.
type MockFixtures struct {
	// Inquiry returns the success answer of Inquiry. Required.
	Inquiry func(req InquiryRequest) *InquiryResponse
	// Pay returns the success answer of Pay (task 044). Nil: the mock does not answer Pay; the
	// adapter does.
	Pay func(req PaymentRequest) *PaymentResponse
	// Settle returns the success answer of Settle (task 044). Nil: the mock does not answer Settle;
	// the adapter does.
	Settle func(req SettleRequest) *SettleResponse
}

// MockLogger is the logger of a mock.
type MockLogger interface {
	Info(msg string, kv ...any)
}

// NewMock returns the adapter that the app registers for p (task 021). The mechanism is the same
// for every provider: debug.mocks.<provider> in the config, and the fixtures of the provider.
//   - Settings off (Enable false): it returns p itself. Nothing changes.
//   - Settings on: Inquiry does not call p. With MockReturnSuccess it returns fixtures.Inquiry; with
//     MockReturnError it returns an error that wraps ErrProviderDown. When the fixtures have Pay
//     (Settle), Pay (Settle) does not call p either and answers by PayReturnType
//     (SettleReturnType). Every other method goes to p.
//
// The app calls it in development mode only (Config.Mode is dev). A nil p, a nil inquiry fixture or
// an unknown return type (with the mock on) is an error. log may be nil (no log line).
func NewMock(p ProviderInterface, s MockSettings, f MockFixtures, log MockLogger) (ProviderInterface, error) {
	if p == nil {
		return nil, errors.New("provider mock: adapter is nil")
	}
	if !s.Enable {
		return p, nil
	}
	if s.ReturnType != MockReturnSuccess && s.ReturnType != MockReturnError {
		return nil, fmt.Errorf("provider mock %s: return type must be %q or %q", p.Name(), MockReturnSuccess, MockReturnError)
	}
	if f.Inquiry == nil {
		return nil, fmt.Errorf("provider mock %s: the inquiry fixture is required", p.Name())
	}
	if s.PayReturnType == "" {
		s.PayReturnType = MockReturnSuccess
	}
	if s.SettleReturnType == "" {
		s.SettleReturnType = MockReturnSuccess
	}
	if f.Pay != nil && s.PayReturnType != MockReturnSuccess && s.PayReturnType != MockReturnFailed && s.PayReturnType != MockReturnPending {
		return nil, fmt.Errorf("provider mock %s: pay return type must be %q, %q or %q", p.Name(), MockReturnSuccess, MockReturnFailed, MockReturnPending)
	}
	if f.Settle != nil && s.SettleReturnType != MockReturnSuccess && s.SettleReturnType != MockReturnError {
		return nil, fmt.Errorf("provider mock %s: settle return type must be %q or %q", p.Name(), MockReturnSuccess, MockReturnError)
	}
	return &mock{ProviderInterface: p, settings: s, fixtures: f, log: log}, nil
}

// mock answers Inquiry (and Pay, Settle when the fixtures have them) with a fixed answer. The
// embedded adapter answers every other method.
type mock struct {
	ProviderInterface
	settings MockSettings
	fixtures MockFixtures
	log      MockLogger
}

// canceled returns the error of a done ctx (nil ctx: not done).
func canceled(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// Inquiry answers with the fixture or with ErrProviderDown, with no call to the provider.
func (m *mock) Inquiry(ctx context.Context, req InquiryRequest) (*InquiryResponse, error) {
	if err := canceled(ctx); err != nil {
		return nil, err
	}
	if m.log != nil {
		m.log.Info("provider mock: inquiry answered with no call to the provider", "provider", m.Name(), "return_type", m.settings.ReturnType, "inquiry_id", req.InquiryID)
	}
	if m.settings.ReturnType == MockReturnError {
		return nil, fmt.Errorf("mock %s: %w", m.Name(), ErrProviderDown)
	}
	return m.fixtures.Inquiry(req), nil
}

// Pay answers with the fixture (success), a failed answer, or ErrProviderDown (pending), with no
// call to the provider. With no Pay fixture the adapter answers.
func (m *mock) Pay(ctx context.Context, req PaymentRequest) (*PaymentResponse, error) {
	if m.fixtures.Pay == nil {
		return m.ProviderInterface.Pay(ctx, req)
	}
	if err := canceled(ctx); err != nil {
		return nil, err
	}
	if m.log != nil {
		m.log.Info("provider mock: payment answered with no call to the provider", "provider", m.Name(), "pay_return_type", m.settings.PayReturnType, "order_id", req.OrderID)
	}
	switch m.settings.PayReturnType {
	case MockReturnFailed:
		return &PaymentResponse{Status: StatusFailed, Code: http.StatusBadRequest, Message: MockPayFailedMessage}, nil
	case MockReturnPending:
		return nil, fmt.Errorf("mock %s: payment: %w", m.Name(), ErrProviderDown)
	}
	return m.fixtures.Pay(req), nil
}

// Settle answers with the fixture (success) or an error (error), with no call to the provider. With
// no Settle fixture the adapter answers.
func (m *mock) Settle(ctx context.Context, req SettleRequest) (*SettleResponse, error) {
	if m.fixtures.Settle == nil {
		return m.ProviderInterface.Settle(ctx, req)
	}
	if err := canceled(ctx); err != nil {
		return nil, err
	}
	if m.log != nil {
		m.log.Info("provider mock: settlement answered with no call to the provider", "provider", m.Name(), "settle_return_type", m.settings.SettleReturnType, "bill_id", req.BillID)
	}
	if m.settings.SettleReturnType == MockReturnError {
		return nil, fmt.Errorf("mock %s: settlement: %w", m.Name(), ErrProviderDown)
	}
	return m.fixtures.Settle(req), nil
}
