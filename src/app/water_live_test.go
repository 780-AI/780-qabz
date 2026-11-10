package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/monitoring/prometheus"
	"qabz/config"
	grpcserver "qabz/entry/grpc"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
)

const (
	liveBillID  = "1234567890125"
	liveBill2   = "9999999999999"
	liveUserOne = "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"
	liveUserTwo = "0b9d2c4e-8a3f-4e61-b7d2-5c1a9e0f3d22"
)

// liveWaterStack is the Water and the Electricity service of the composition root (one inquiry
// flow) on a throw-away schema with the devdb tables and the seed providers, after setupSQL ran.
// The providers are loaded (a real ProvidersService), the Qabzino and Tavanir mocks are on with
// returnType (dev mode), metrics are on.
type liveWaterStack struct {
	db          *postgres.Client
	water       *grpcserver.Water
	electricity *grpcserver.Electricity
	metrics     *prometheus.Server
	log         *liveLogger
}

func newLiveWater(t *testing.T, ctx context.Context, returnType, setupSQL string) *liveWaterStack {
	t.Helper()
	schemaDSN := postgrestest.SeededSchema(t, ctx, postgrestest.RunDSN(t))
	db, err := postgres.New(ctx, schemaDSN)
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	t.Cleanup(db.Close)
	if setupSQL != "" {
		if _, err := db.Pool().Exec(ctx, setupSQL); !assert.NoError(t, err) {
			t.FailNow()
		}
	}
	cfg := testConfig(t)
	cfg.DebugMockQabzinoEnable = true
	cfg.DebugMockQabzinoReturnType = returnType
	cfg.DebugMockTavanirEnable = true
	cfg.DebugMockTavanirReturnType = returnType
	cfg.CachingWater = time.Hour
	cfg.CachingElectricity = time.Hour
	log := &liveLogger{}
	providers, err := newProviders(config.NewHolder(cfg), postgres.NewProviderRepository(db), prometheus.New("127.0.0.1:0", false), log)
	if !assert.NoError(t, err) || !assert.NotNil(t, providers) || !assert.NoError(t, providers.Start(ctx)) {
		t.FailNow()
	}
	t.Cleanup(func() { assert.NoError(t, providers.Stop(context.Background())) })
	registry, err := newProviderRegistry(cfg, log)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	metrics := prometheus.New("127.0.0.1:0", true)
	flow, err := newInquiryFlow(cfg, db, providers, registry, metrics, log)
	if !assert.NoError(t, err) || !assert.NotNil(t, flow) {
		t.FailNow()
	}
	return &liveWaterStack{db: db, water: newWater(flow, log), electricity: newElectricity(flow, log), metrics: metrics, log: log}
}

func (s *liveWaterStack) ask(user, bill string) (*pb.InquiryBillPayResponse, error) {
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: user, Mobile: "989121234567"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.water.Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: bill})
}

func (s *liveWaterStack) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if !assert.NoError(t, s.db.Pool().QueryRow(context.Background(), query, args...).Scan(&n)) {
		t.FailNow()
	}
	return n
}

func (s *liveWaterStack) metric(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	v, _ := metricValue(t, s.metrics.Registry(), name, labels)
	return v
}

// qabzinoWater makes Qabzino the enabled water inquiry provider (the seed has parsian enabled and
// Qabzino disabled).
const qabzinoWater = `UPDATE providers SET status = 'enable' WHERE id = 36; UPDATE providers SET status = 'disable' WHERE id = 44;`

// TestLiveWaterInquiryFlow is the feature-ON integration test (task 021): the first call saves the
// rows, the second call within the TTL is answered from the database with no provider call, and a
// provider error does not hide a valid saved answer.
func TestLiveWaterInquiryFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, qabzinoWater)

	res, err := s.ask(liveUserOne, liveBillID)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(25000000), res.GetPrice())
	assert.Equal(t, "2500012345", res.GetPayId())
	assert.Equal(t, "2030-01-01", res.GetPaymentDeadline())
	assert.Equal(t, liveBillID, res.GetBillId())

	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "one request row")
	var status, trace, provider, dialer, tid string
	assert.NoError(t, s.db.Pool().QueryRow(ctx, `SELECT status, provider_trace_id, provider_name, dialer, tid FROM bill_inquiry_requests`).Scan(&status, &trace, &provider, &dialer, &tid))
	assert.Equal(t, []string{"success", "mock-qabzino-0001", "qabzino", "989121234567"}, []string{status, trace, provider, dialer})
	assert.Regexp(t, `^1[0-9]+$`, tid)
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND NOT is_cached AND provider_id = 36 AND user_id = $2 AND amount = 25000000 AND pay_id = '2500012345'`, liveBillID, liveUserOne))
	assert.Equal(t, 1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "the provider (mock) was called")
	assert.Equal(t, 1, s.log.count("info", "inquiry: provider call"))

	// Second call, within the TTL, another user: from the database.
	res2, err := s.ask(liveUserTwo, liveBillID)
	if !assert.NoError(t, err) || !assert.NotNil(t, res2) {
		return
	}
	assert.Equal(t, res.GetPrice(), res2.GetPrice())
	assert.Equal(t, res.GetPayId(), res2.GetPayId())
	assert.Equal(t, res.GetPaymentDeadline(), res2.GetPaymentDeadline())
	assert.Equal(t, res.GetBillId(), res2.GetBillId())
	assert.Equal(t, 1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "no second provider call")
	assert.Equal(t, 1, s.log.count("info", "inquiry: answered from database, no provider call"))
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE is_cached AND user_id = $1 AND inquiry_id = $2`, liveUserTwo, liveBillID), "a copy for the second user")
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM bill_inquiries`))

	// The metrics of the two calls.
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "water", "provider": "qabzino", "result": "provider_ok"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "water", "provider": "qabzino", "result": "db_hit"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "qabzino", "action": "inquiry", "result": "ok"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_call_duration_ms", map[string]string{"provider": "qabzino", "action": "inquiry"}), "one sample: only the provider call")
	assert.Equal(t, 0, s.log.count("error", "water: inquiry failed"))
}

// TestLiveWaterInquiryMockError: with return_type error a new bill gets the provider-down answer,
// the request row has the failed status and no inquiry is saved; a bill that was saved earlier is
// still answered from the database (the mock error does not show).
func TestLiveWaterInquiryMockError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnError, qabzinoWater)

	res, err := s.ask(liveUserOne, liveBillID)
	assert.Nil(t, res)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE status = 'failed' AND provider_trace_id IS NULL AND inquiry_id = $1`, liveBillID))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiries`), "no inquiry is saved")
	assert.Equal(t, 1, s.log.count("error", "water: provider inquiry failed"), "one error line for the failed call")
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "water", "provider": "qabzino", "result": "provider_down"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "qabzino", "action": "inquiry", "result": "down"}))

	// A valid saved answer of an earlier call.
	_, err = s.db.Pool().Exec(ctx, `INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, tid, inquiry_type, bill_id, pay_id, payment_deadline, amount, is_cached, is_paid)
		VALUES ('11111111-1111-4111-8111-111111111111', $1, $2, 36, 'qabzino', 'water', 't', 'regular', $2, '1000012345', '2031-02-03', 10000000, false, false)`, liveUserTwo, liveBill2)
	if !assert.NoError(t, err) {
		return
	}
	res, err = s.ask(liveUserOne, liveBill2)
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(10000000), res.GetPrice())
		assert.Equal(t, "2031-02-03", res.GetPaymentDeadline())
	}
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "water", "provider": "qabzino", "result": "db_hit"}))
}

// TestLiveWaterInquiryProviderCases: the seed has parsian enabled (no adapter yet): the answer is
// provider-down with one error line; with no enabled water inquiry provider the answer is the same
// with its own error line and result.
func TestLiveWaterInquiryProviderCases(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	s := newLiveWater(t, ctx, config.MockReturnSuccess, "")
	_, err := s.ask(liveUserOne, liveBillID)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "water: provider inquiry failed"))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "water", "provider": "parsian", "result": "error"}), "parsian was picked and has no adapter")
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "nothing is written before the provider call")

	s = newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE bill_type = 'water' AND action = 'inquiry';`)
	_, err = s.ask(liveUserOne, liveBillID)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "water: no enabled provider for inquiry"))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "water", "provider": "none", "result": "no_provider"}))

	// An invalid bill ID is a client error: no log line, no row, nothing counted as an inquiry.
	_, err = s.ask(liveUserOne, "12345")
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "water: no enabled provider for inquiry")+s.log.count("error", "water: inquiry failed"), "still one error line in total")
}
