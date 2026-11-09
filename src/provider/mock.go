package provider

import (
	"context"
	"errors"
	"fmt"
)

// The return types of a provider mock (config debug.mocks.<provider>.return_type). They equal
// config.MockReturnSuccess and config.MockReturnError (a test in app checks it).
const (
	MockReturnSuccess = "success"
	MockReturnError   = "error"
)

// MockSettings are the settings of one provider mock (config debug.mocks.<provider>).
type MockSettings struct {
	Enable     bool
	ReturnType string // MockReturnSuccess or MockReturnError
}

// MockFixtures are the fixed answers that a provider supplies for its mock.
type MockFixtures struct {
	// Inquiry returns the success answer of Inquiry. It must return a new value on each call.
	Inquiry func(req InquiryRequest) *InquiryResponse
}

// MockLogger is the logger of a mock.
type MockLogger interface {
	Info(msg string, kv ...any)
}

// NewMock returns the adapter that the app registers for p (task 021). The mechanism is the same
// for every provider: debug.mocks.<provider> in the config, and the fixtures of the provider.
//   - Settings off (Enable false): it returns p itself. Nothing changes.
//   - Settings on: Inquiry does not call p. With MockReturnSuccess it returns fixtures.Inquiry; with
//     MockReturnError it returns an error that wraps ErrProviderDown. Every other method goes to p.
//
// The app calls it in development mode only (Config.Mode is dev). A nil p, a nil fixture or an
// unknown return type (with the mock on) is an error. log may be nil (no log line).
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
	return &mock{ProviderInterface: p, returnType: s.ReturnType, fixtures: f, log: log}, nil
}

// mock answers Inquiry with a fixed answer. The embedded adapter answers every other method.
type mock struct {
	ProviderInterface
	returnType string
	fixtures   MockFixtures
	log        MockLogger
}

// Inquiry answers with the fixture or with ErrProviderDown, with no call to the provider.
func (m *mock) Inquiry(ctx context.Context, req InquiryRequest) (*InquiryResponse, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if m.log != nil {
		m.log.Info("provider mock: inquiry answered with no call to the provider", "provider", m.Name(), "return_type", m.returnType, "inquiry_id", req.InquiryID)
	}
	if m.returnType == MockReturnError {
		return nil, fmt.Errorf("mock %s: %w", m.Name(), ErrProviderDown)
	}
	return m.fixtures.Inquiry(req), nil
}
