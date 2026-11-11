package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"qabz/config"
)

// noCacheInquiries sets debug.cache_inquiries: false (task 031).
func noCacheInquiries(c *config.Config) { c.DebugCacheInquiries = false }

// TestLiveNoCacheInquiries is the feature-ON integration test of task 031: with
// debug.cache_inquiries false two calls of the same bill give two provider calls, and
// bill_inquiries and bill_inquiry_requests have no row after both, also for a provider error; a
// valid saved row does not answer.
func TestLiveNoCacheInquiries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterCfg(t, ctx, config.MockReturnSuccess, qabzinoWater, noCacheInquiries)
	assert.Equal(t, 1, s.log.count("warn", "inquiry store disabled (debug.cache_inquiries: false): development only, every inquiry calls the provider and nothing is saved"))

	for i := 1; i <= 2; i++ {
		res, err := s.ask(liveUserOne, liveBillID)
		if assert.NoError(t, err, i) && assert.NotNil(t, res, i) {
			assert.Equal(t, uint32(25000000), res.GetPrice(), i)
			assert.Equal(t, "2500012345", res.GetPayId(), i)
		}
	}
	assert.Equal(t, 2, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "two provider calls")
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiries`), "no inquiry row")
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no request row")
	assert.Equal(t, 2.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "water", "provider": "qabzino", "result": "provider_ok"}))
	assert.Equal(t, 0.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "water", "provider": "qabzino", "result": "db_hit"}))

	// A valid saved row of another bill does not answer: the provider is called and nothing is added.
	_, err := s.db.Pool().Exec(ctx, `INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, tid, inquiry_type, bill_id, pay_id, payment_deadline, amount, is_cached, is_paid)
		VALUES ('11111111-1111-4111-8111-111111111111', $1, $2, 36, 'qabzino', 'water', 't', 'regular', $2, '1000012345', '2031-02-03', 10000000, false, false)`, liveUserTwo, liveBill2)
	if !assert.NoError(t, err) {
		return
	}
	res, err := s.ask(liveUserOne, liveBill2)
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(25000000), res.GetPrice(), "the mock answer, not the saved row")
	}
	assert.Equal(t, 3, s.log.count("info", "provider mock: inquiry answered with no call to the provider"))
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries`), "only the inserted row")
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))

	// A provider error: the error answer, still no row.
	e := newLiveWaterCfg(t, ctx, config.MockReturnError, qabzinoWater, noCacheInquiries)
	_, err = e.ask(liveUserOne, liveBillID)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, e.log.count("error", "water: provider inquiry failed"))
	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM bill_inquiries`))
	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
}
