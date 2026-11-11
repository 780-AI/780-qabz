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
	"qabz/config"
)

const (
	livePhone  = "989351234567"
	livePhone2 = "989359876543"
)

func (s *liveWaterStack) askPhone(user string, req *pb.InquiryPhoneRequest) (*pb.InquiryPhoneResponse, error) {
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: user, Mobile: "989121234567"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.phone.Inquiry(ctx, req)
}

func irancellPhoneReq(phone string) *pb.InquiryPhoneRequest {
	return &pb.InquiryPhoneRequest{Phone: phone, Operator: pb.InquiryPhoneRequest_irancell}
}

// TestLiveIrancellInquiryFlow is the feature-ON integration test (task 035) with the seed (the
// Irancell provider, id 38, is enabled): the first call asks Irancell (the mock) and saves the
// mid-term and the final-term inquiry; the second call within the TTL is answered from the database
// with no provider call; another phone is another key.
func TestLiveIrancellInquiryFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, "")

	res, err := s.askPhone(liveUserOne, irancellPhoneReq(livePhone))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(150000), res.GetMidTerm().GetPrice())
	assert.Equal(t, uint32(450000), res.GetFullTerm().GetPrice())

	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "one request row")
	var st, trace, prov, dialer, tid, inquiryID, billType string
	assert.NoError(t, s.db.Pool().QueryRow(ctx, `SELECT status, provider_trace_id, provider_name, dialer, tid, inquiry_id, bill_type FROM bill_inquiry_requests`).
		Scan(&st, &trace, &prov, &dialer, &tid, &inquiryID, &billType))
	assert.Equal(t, []string{"success", "mock-irancell-0001", "irancell", "989121234567", livePhone, "irancell"}, []string{st, trace, prov, dialer, inquiryID, billType})
	assert.Regexp(t, `^8[0-9]+$`, tid, "the Irancell tid prefix")
	for typ, amount := range map[string]int{"midterm": 150000, "final_term": 450000} {
		assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND bill_type = 'irancell' AND NOT is_cached AND NOT is_paid
			AND provider_id = 38 AND provider_name = 'irancell' AND user_id = $2 AND inquiry_type = $3 AND amount = $4
			AND bill_id IS NULL AND pay_id IS NULL AND payment_deadline IS NULL AND extra_data IS NULL AND tid = 'mock-irancell-0001'`,
			livePhone, liveUserOne, typ, amount), typ)
	}
	assert.Equal(t, 1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "the provider (mock) was called")

	// Second call, within the TTL, another user: from the database.
	res2, err := s.askPhone(liveUserTwo, irancellPhoneReq(livePhone))
	if !assert.NoError(t, err) || !assert.NotNil(t, res2) {
		return
	}
	assert.Equal(t, []uint32{150000, 450000}, []uint32{res2.GetMidTerm().GetPrice(), res2.GetFullTerm().GetPrice()}, "the same answer")
	assert.Equal(t, 1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "no second provider call")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE is_cached AND user_id = $1 AND inquiry_id = $2`, liveUserTwo, livePhone), "two copies for the second user")

	// Another phone is another key.
	_, err = s.askPhone(liveUserOne, irancellPhoneReq(livePhone2))
	assert.NoError(t, err)
	assert.Equal(t, 2, s.log.count("info", "provider mock: inquiry answered with no call to the provider"))

	assert.Equal(t, 2.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "irancell", "provider": "irancell", "result": "provider_ok"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "irancell", "provider": "irancell", "result": "db_hit"}))
	assert.Equal(t, 2.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "irancell", "action": "inquiry", "result": "ok"}))
}

// TestLiveIrancellCacheOff: with the default caching.irancell (0, as bill) every call asks the
// provider and the saved inquiries are not used.
func TestLiveIrancellCacheOff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterCfg(t, ctx, config.MockReturnSuccess, "", func(c *config.Config) { c.CachingIrancell = 0 })
	for range 2 {
		_, err := s.askPhone(liveUserOne, irancellPhoneReq(livePhone))
		assert.NoError(t, err)
	}
	assert.Equal(t, 2, s.log.count("info", "provider mock: inquiry answered with no call to the provider"))
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
	assert.Equal(t, 4, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE NOT is_cached`))
	assert.Equal(t, 0.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "irancell", "provider": "irancell", "result": "db_hit"}))
}

// TestLiveIrancellMockError: with return_type error a new phone gets the provider-down answer, the
// request row is failed and no inquiry is saved; a phone that was saved earlier is still answered
// from the database.
func TestLiveIrancellMockError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnError, "")

	res, err := s.askPhone(liveUserOne, irancellPhoneReq(livePhone))
	assert.Nil(t, res)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, "استعلام از سوی تامین کننده موقتا غیر فعال می باشد", status.Convert(err).Message())
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE status = 'failed' AND provider_trace_id IS NULL AND inquiry_id = $1 AND bill_type = 'irancell'`, livePhone))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiries`))
	assert.Equal(t, 1, s.log.count("error", "irancell: provider inquiry failed"))

	_, err = s.db.Pool().Exec(ctx, `INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, tid, inquiry_type, amount, is_cached, is_paid)
		VALUES ('33333333-3333-4333-8333-333333333331', $1, $2, 38, 'irancell', 'irancell', 't', 'midterm', 11000, false, false),
		       ('33333333-3333-4333-8333-333333333332', $1, $2, 38, 'irancell', 'irancell', 't', 'final_term', 22000, false, false)`, liveUserTwo, livePhone2)
	if !assert.NoError(t, err) {
		return
	}
	res, err = s.askPhone(liveUserOne, irancellPhoneReq(livePhone2))
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, []uint32{11000, 22000}, []uint32{res.GetMidTerm().GetPrice(), res.GetFullTerm().GetPrice()})
	}
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
}

// TestLiveIrancellOnlyIrancell (operator rule): when the Irancell row is disabled and another
// provider is enabled for the bill type irancell, nobody is called: provider-down. With no enabled
// provider: the same answer. Rightel is not available yet.
func TestLiveIrancellOnlyIrancell(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// Another provider row for the bill type irancell, with an adapter (Qabzino), and the Irancell row disabled.
	s := newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE id = 38;
		INSERT INTO providers (id, name, subject, bill_type, action, type, priority, timeout, status, updated_at, use_nats)
		VALUES (9035, 'qabzino', 'qabzino-irancell-test', 'irancell', 'inquiry', 'retryable', 1, 1000, 'enable', now(), false);`)
	_, err := s.askPhone(liveUserOne, irancellPhoneReq(livePhone))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 0, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "Qabzino is not called")
	assert.Equal(t, 1, s.log.count("error", "irancell: no enabled provider for inquiry"))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "irancell", "provider": "none", "result": "no_provider"}))

	s = newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE id = 38;`)
	_, err = s.askPhone(liveUserOne, irancellPhoneReq(livePhone))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "irancell: no enabled provider for inquiry"))

	// rightel: not available yet; invalid requests: InvalidArgument. No line, no row. (mci and no
	// operator run the MCI inquiry since task 036: TestLiveMCI*.)
	lines := len(s.log.lines)
	for name, c := range map[string]struct {
		req  *pb.InquiryPhoneRequest
		want codes.Code
	}{
		"rightel":     {&pb.InquiryPhoneRequest{Phone: livePhone, Operator: pb.InquiryPhoneRequest_rightel}, codes.Unimplemented},
		"no phone":    {&pb.InquiryPhoneRequest{Operator: pb.InquiryPhoneRequest_irancell}, codes.InvalidArgument},
		"a bad phone": {&pb.InquiryPhoneRequest{Phone: "09351234567", Operator: pb.InquiryPhoneRequest_irancell}, codes.InvalidArgument},
		"unknown":     {&pb.InquiryPhoneRequest{Phone: livePhone, Operator: pb.InquiryPhoneRequest_unknown}, codes.InvalidArgument},
	} {
		_, err = s.askPhone(liveUserOne, c.req)
		assert.Equal(t, c.want, status.Code(err), name)
	}
	assert.Len(t, s.log.lines, lines, "no log line")
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
}
