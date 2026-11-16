package inquiry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/provider"
)

// anySelector picks the provider p for every bill type (the tracing tests cover each bill type).
type anySelector struct {
	p   provider.Provider
	err error
}

func (a anySelector) PickProvider(string, string) (provider.Provider, error) { return a.p, a.err }

// tracedProvider is stubProvider with one client span "provider call", a child of the span in ctx
// (the client span that the shared instrumentation of task 047 makes for a real adapter).
func tracedProvider(name string) *stubProvider {
	return &stubProvider{name: name, resp: successResp(), run: func(ctx context.Context) {
		_, span := otel.Tracer("test").Start(ctx, "provider call", trace.WithSpanKind(trace.SpanKindClient))
		span.End()
	}}
}

// tracedService is a flow with the provider name for every bill type, a cache of one hour and
// repo.
func tracedService(t *testing.T, name string, repo *memRepo, adapter *stubProvider, selErr error) *Service {
	t.Helper()
	svc, err := NewService(Options{
		Repository: repo, Providers: anySelector{p: provider.Provider{ID: 7, Name: name, Status: provider.StatusEnable}, err: selErr},
		Adapters: fakeAdapters{name: adapter}, CacheTTL: func(string) time.Duration { return time.Hour },
	})
	if !assert.NoError(t, err) || !assert.NotNil(t, svc) {
		t.FailNow()
	}
	svc.newTID = func(string) (string, error) { return "tid-1", nil }
	return svc
}

// one returns the only ended span with the name.
func one(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	var out []sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	if !assert.Len(t, out, 1, name) {
		t.FailNow()
	}
	return out[0]
}

// TestInquireSpanTree (conventions § 17; task 049 D1): for each bill type, a provider answer is the
// tree parent > inquiry.Service.Inquire > (fromDatabase, fromProvider > provider call), and a second
// call is answered from the saved inquiries (fromDatabase hit, no fromProvider). No span has the
// inquiry ID, the user or the bill ID.
func TestInquireSpanTree(t *testing.T) {
	for _, tc := range []struct{ billType, provider, inquiryID string }{
		{provider.BillTypeWater, "qabzino", validBill},
		{provider.BillTypeElectricity, "tavanir", validBill},
		{provider.BillTypeGas, "qabzino", gasCode},
		{provider.BillTypeTel, karashopProvider, telNumber},
		{provider.BillTypeIrancell, irancellProvider, irPhone},
		{provider.BillTypeMobile, "jiring", irPhone},
		{provider.BillTypeMobileMidTermRightel, qabzinoProvider, irPhone},
	} {
		t.Run(tc.billType, func(t *testing.T) {
			sr := jaegertest.Install(t)
			svc := tracedService(t, tc.provider, newMemRepo(), tracedProvider(tc.provider), nil)
			req := Request{BillType: tc.billType, InquiryID: tc.inquiryID, UserID: userA}

			ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
			_, err := svc.Inquire(ctx, req)
			parent.End()
			assert.NoError(t, err)
			spans := sr.Ended()
			inq := one(t, spans, inquireSpan)
			db := one(t, spans, fromDatabaseSpan)
			pro := one(t, spans, fromProviderSpan)
			call := one(t, spans, "provider call")
			assert.Equal(t, parent.SpanContext().SpanID(), inq.Parent().SpanID())
			assert.Equal(t, inq.SpanContext().SpanID(), db.Parent().SpanID())
			assert.Equal(t, inq.SpanContext().SpanID(), pro.Parent().SpanID())
			assert.Equal(t, pro.SpanContext().SpanID(), call.Parent().SpanID(), "the provider call is under fromProvider")
			assert.ElementsMatch(t, []attribute.KeyValue{attribute.String("bill_type", tc.billType),
				attribute.String("provider", tc.provider), attribute.String("result", ResultProviderOK)}, inq.Attributes())
			assert.Contains(t, db.Attributes(), attribute.String("result", dbMiss))
			assert.Contains(t, pro.Attributes(), attribute.String("result", ResultProviderOK))
			for _, s := range []sdktrace.ReadOnlySpan{inq, db, pro} {
				assert.Equal(t, codes.Unset, s.Status().Code, s.Name())
			}

			_, err = svc.Inquire(context.Background(), req)
			assert.NoError(t, err)
			assert.Len(t, jaegertest.ByName(sr, fromProviderSpan), 1, "a saved answer makes no fromProvider span")
			dbs := jaegertest.ByName(sr, fromDatabaseSpan)
			if assert.Len(t, dbs, 2) {
				assert.Contains(t, dbs[1].Attributes(), attribute.String("result", dbHit))
			}
			inqs := jaegertest.ByName(sr, inquireSpan)
			if assert.Len(t, inqs, 2) {
				assert.Contains(t, inqs[1].Attributes(), attribute.String("result", ResultDBHit))
				assert.False(t, inqs[1].Parent().IsValid(), "no span in ctx: a root (the server span in the App)")
			}
			jaegertest.AssertClean(t, sr.Ended(), tc.inquiryID, userA, validBill, "2500012345")
		})
	}
}

// TestInquireSpanFailures (task 049 D1, D5): a provider that is down gives Error on fromProvider and
// on Inquire; a failed read of the saved inquiries gives Error on fromDatabase only (the flow goes on
// to the provider); no provider gives Error on Inquire with no child span; a request that breaks a
// rule makes no span. No error text is in a span.
func TestInquireSpanFailures(t *testing.T) {
	sr := jaegertest.Install(t)
	down := tracedProvider("qabzino")
	down.err = fmt.Errorf("dial 989121234567: %w", provider.ErrProviderDown)
	svc := tracedService(t, "qabzino", newMemRepo(), down, nil)
	req := Request{BillType: provider.BillTypeWater, InquiryID: validBill, UserID: userA}
	_, err := svc.Inquire(context.Background(), req)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	inq, pro := one(t, sr.Ended(), inquireSpan), one(t, sr.Ended(), fromProviderSpan)
	for _, s := range []sdktrace.ReadOnlySpan{inq, pro} {
		assert.Equal(t, codes.Error, s.Status().Code, s.Name())
		assert.Equal(t, ResultProviderDown, s.Status().Description, s.Name())
	}
	assert.Equal(t, codes.Unset, one(t, sr.Ended(), "provider call").Status().Code, "the stub span; a real client span gets its own status")
	jaegertest.AssertClean(t, sr.Ended(), validBill, userA, "989121234567")

	sr = jaegertest.Install(t)
	repo := newMemRepo()
	repo.findErr = errors.New("read " + validBill + " failed")
	svc = tracedService(t, "qabzino", repo, tracedProvider("qabzino"), nil)
	_, err = svc.Inquire(context.Background(), req)
	assert.NoError(t, err)
	db := one(t, sr.Ended(), fromDatabaseSpan)
	assert.Equal(t, codes.Error, db.Status().Code)
	assert.Equal(t, dbError, db.Status().Description)
	assert.Equal(t, codes.Unset, one(t, sr.Ended(), inquireSpan).Status().Code, "the provider answered")
	jaegertest.AssertClean(t, sr.Ended(), validBill, userA, "failed")

	sr = jaegertest.Install(t)
	svc = tracedService(t, "qabzino", newMemRepo(), tracedProvider("qabzino"), provider.ErrNoProvider)
	_, err = svc.Inquire(context.Background(), req)
	assert.ErrorIs(t, err, provider.ErrNoProvider)
	if assert.Len(t, sr.Ended(), 1) {
		assert.Equal(t, inquireSpan, sr.Ended()[0].Name())
		assert.Equal(t, ResultNoProvider, sr.Ended()[0].Status().Description)
		assert.Contains(t, sr.Ended()[0].Attributes(), attribute.String("provider", NoProviderLabel))
	}

	sr = jaegertest.Install(t)
	_, err = svc.Inquire(context.Background(), Request{BillType: provider.BillTypeWater, InquiryID: "123", UserID: userA})
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = svc.Inquire(context.Background(), Request{BillType: "gas 1234567890117", InquiryID: validBill, UserID: userA})
	assert.Error(t, err)
	assert.Empty(t, sr.Ended(), "a request that breaks a rule makes no span")

	jaegertest.AssertClean(t, sr.Ended(), validBill, userA, "989121234567")
}
