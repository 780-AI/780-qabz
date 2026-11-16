package reconcile

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/core/order"
	"qabz/provider"
)

// tracedAdapter is fakeAdapter with one span "provider status" under the span in ctx (the client
// span of the real adapter, task 047).
type tracedAdapter struct{ *fakeAdapter }

func (a tracedAdapter) Status(ctx context.Context, req provider.StatusRequest) (*provider.StatusResponse, error) {
	_, span := otel.Tracer("test").Start(ctx, "provider status")
	defer span.End()
	return a.fakeAdapter.Status(ctx, req)
}

// tracedReporter is fakeReporter with one span "kafka publish" under the span in ctx (the span of
// kafka.Publish, task 047).
type tracedReporter struct{ *fakeReporter }

func (r tracedReporter) Report(ctx context.Context, orderID, status string) error {
	_, span := otel.Tracer("test").Start(ctx, "kafka publish")
	defer span.End()
	return r.fakeReporter.Report(ctx, orderID, status)
}

// TestCheckSpanTree (conventions § 17; task 049 D2): one tick is the tree cron root > one span
// reconcile.StatusWorker.check for each order > its provider and Kafka spans; the span has the
// provider, the order ID and the result; a read, report or save failure gives Error, the normal
// outcomes (success, pending, skipped) do not. No span has the RRN or the error text.
func TestCheckSpanTree(t *testing.T) {
	sr := jaegertest.Install(t)
	wd := newWorld(t, 10*time.Hour)
	wd.w.o.Adapters = fakeAdapters{tracedAdapter{wd.adapter}}
	wd.w.o.Reporter = tracedReporter{wd.reporter}
	wd.repo.pages = [][]string{{"ok", "wait"}, {"done", "gone"}}
	wd.row("ok", order.StatusPending, time.Hour)
	wd.row("wait", order.StatusPending, time.Hour)
	wd.row("done", order.StatusSuccess, time.Hour)
	wd.adapter.answers["ok"] = &provider.StatusResponse{Status: "success"}
	wd.adapter.answers["wait"] = &provider.StatusResponse{Status: "in-progress"}
	// "gone" has no row: a failed read.

	ctx, root := otel.Tracer("test").Start(context.Background(), "cron parsian-getstatus")
	assert.NoError(t, wd.w.Run(ctx, &fakeCharger{}))
	root.End()

	checks := jaegertest.ByName(sr, checkSpan)
	if !assert.Len(t, checks, 4) {
		t.FailNow()
	}
	want := map[string]string{"ok": ResultSuccess, "wait": ResultPending, "done": ResultSkipped, "gone": ResultReadError}
	byOrder := map[string]sdktrace.ReadOnlySpan{}
	for i, s := range checks {
		id := []string{"ok", "wait", "done", "gone"}[i]
		byOrder[id] = s
		assert.Equal(t, root.SpanContext().SpanID(), s.Parent().SpanID(), id)
		assert.ElementsMatch(t, []attribute.KeyValue{attribute.String("provider", "parsian"), attribute.String("order.id", id),
			attribute.String("result", want[id])}, s.Attributes(), id)
		if want[id] == ResultReadError {
			assert.Equal(t, codes.Error, s.Status().Code, id)
			assert.Equal(t, ResultReadError, s.Status().Description, id)
		} else {
			assert.Equal(t, codes.Unset, s.Status().Code, id)
		}
	}
	statusSpans := jaegertest.ByName(sr, "provider status")
	if assert.Len(t, statusSpans, 2, "ok and wait ask the provider") {
		assert.Equal(t, byOrder["ok"].SpanContext().SpanID(), statusSpans[0].Parent().SpanID())
		assert.Equal(t, byOrder["wait"].SpanContext().SpanID(), statusSpans[1].Parent().SpanID())
	}
	publish := jaegertest.ByName(sr, "kafka publish")
	if assert.Len(t, publish, 1, "only ok is reported") {
		assert.Equal(t, byOrder["ok"].SpanContext().SpanID(), publish[0].Parent().SpanID())
	}
	jaegertest.AssertClean(t, sr.Ended(), "778899")
}

// TestCheckSpanReportAndSaveErrors: report_error and save_error give the status Error.
func TestCheckSpanReportAndSaveErrors(t *testing.T) {
	sr := jaegertest.Install(t)
	wd := newWorld(t, 10*time.Hour)
	wd.repo.pages = [][]string{{"a"}}
	wd.row("a", order.StatusPending, time.Hour)
	wd.adapter.answers["a"] = &provider.StatusResponse{Status: "success"}
	wd.reporter.err = errors.New("kafka down for 989121234567")
	assert.NoError(t, wd.w.Run(context.Background(), &fakeCharger{}))
	wd.reporter.err = nil
	wd.repo.finalErr = errors.New("already final")
	assert.NoError(t, wd.w.Run(context.Background(), &fakeCharger{}))

	checks := jaegertest.ByName(sr, checkSpan)
	if assert.Len(t, checks, 2) {
		for i, want := range []string{ResultReportError, ResultSaveError} {
			assert.Equal(t, codes.Error, checks[i].Status().Code, want)
			assert.Equal(t, want, checks[i].Status().Description, want)
		}
	}
	jaegertest.AssertClean(t, sr.Ended(), "989121234567", "kafka down", "already final")
}
