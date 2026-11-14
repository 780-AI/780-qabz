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
	"qabz/provider/karashop"
)

// Task 025: the landline inquiry on the run database with the seed (Karashop, id 124325, is enabled
// for tel; Parsian 40 and Qabzino 52 are disabled) and the Karashop mock.

const (
	liveTel  = "982188776655"
	liveTel2 = "982133445566"
)

func (s *liveWaterStack) askTel(user string, req *pb.InquiryTelRequest) (*pb.InquiryPhoneResponse, error) {
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: user, Mobile: "989121234567"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.tel.Inquiry(ctx, req)
}

// TestLiveTelInquiryFlow: the first call asks Karashop (the mock) and saves the mid-term and the
// final-term inquiry with one request row; the second call within the TTL (caching.tel) is answered
// from the database with no provider call and no new request row. The phone is accepted in the
// 0 form and saved in the 98 form.
func TestLiveTelInquiryFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, "")

	res, err := s.askTel(liveUserOne, &pb.InquiryTelRequest{Phone: "02188776655"})
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, []string{karashop.MockBillID, karashop.MockMidTermPayID, karashop.MockBillID, karashop.MockFinalTermPayID},
		[]string{res.GetMidTerm().GetBillId(), res.GetMidTerm().GetPayId(), res.GetFullTerm().GetBillId(), res.GetFullTerm().GetPayId()})
	assert.Equal(t, []uint32{karashop.MockMidTermAmount, karashop.MockFinalTermAmount}, []uint32{res.GetMidTerm().GetPrice(), res.GetFullTerm().GetPrice()})

	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "one request row")
	var st, trace, prov, tid, inquiryID, billType string
	assert.NoError(t, s.db.Pool().QueryRow(ctx, `SELECT status, provider_trace_id, provider_name, tid, inquiry_id, bill_type FROM bill_inquiry_requests`).
		Scan(&st, &trace, &prov, &tid, &inquiryID, &billType))
	assert.Equal(t, []string{"success", "mock-karashop-0001", "karashop", liveTel, "tel"}, []string{st, trace, prov, inquiryID, billType})
	assert.Regexp(t, `^4[0-9]+$`, tid, "the telephone tid prefix")
	for typ, c := range map[string]struct {
		payID  string
		amount int
	}{"midterm": {karashop.MockMidTermPayID, karashop.MockMidTermAmount}, "final_term": {karashop.MockFinalTermPayID, karashop.MockFinalTermAmount}} {
		assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND bill_type = 'tel' AND NOT is_cached AND NOT is_paid
			AND provider_id = 124325 AND provider_name = 'karashop' AND user_id = $2 AND inquiry_type = $3 AND amount = $4
			AND bill_id = $5 AND pay_id = $6 AND payment_deadline IS NULL AND tid = 'mock-karashop-0001'`,
			liveTel, liveUserOne, typ, c.amount, karashop.MockBillID, c.payID), typ)
	}
	assert.Equal(t, 1, s.log.count("info", mockInfoLine), "the provider (mock) was called")

	res2, err := s.askTel(liveUserTwo, &pb.InquiryTelRequest{Phone: liveTel})
	if !assert.NoError(t, err) || !assert.NotNil(t, res2) {
		return
	}
	assert.Equal(t, []uint32{karashop.MockMidTermAmount, karashop.MockFinalTermAmount}, []uint32{res2.GetMidTerm().GetPrice(), res2.GetFullTerm().GetPrice()}, "the same answer")
	assert.Equal(t, karashop.MockBillID, res2.GetFullTerm().GetBillId())
	assert.Equal(t, 1, s.log.count("info", mockInfoLine), "no second provider call")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE is_cached AND user_id = $1`, liveUserTwo), "the copies of the second user")

	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "tel", "provider": "karashop", "result": "provider_ok"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "tel", "provider": "karashop", "result": "db_hit"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "karashop", "action": "inquiry", "result": "ok"}))
}

// TestLiveTelMockError: with return_type error a new number gets the provider-down answer, the
// request row is failed and no inquiry is saved; saved rows of another number still answer; two
// saved rows of the same inquiry type are a miss (bill: shouldSkipCache).
func TestLiveTelMockError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnError, "")

	res, err := s.askTel(liveUserOne, &pb.InquiryTelRequest{Phone: liveTel})
	assert.Nil(t, res)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE status = 'failed' AND provider_name = 'karashop' AND inquiry_id = $1 AND bill_type = 'tel'`, liveTel))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiries`))
	assert.Equal(t, 1, s.log.count("error", "tel: provider inquiry failed"))

	_, err = s.db.Pool().Exec(ctx, `INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, tid, inquiry_type, bill_id, pay_id, amount, is_cached, is_paid)
		VALUES ('25252525-2525-4525-8525-252525252521', $1, $2, 124325, 'karashop', 'tel', 't', 'midterm', '2101234560040', '1100099', 11000, false, false),
		       ('25252525-2525-4525-8525-252525252522', $1, $2, 124325, 'karashop', 'tel', 't', 'final_term', '2101234560040', '2200099', 22000, false, false)`, liveUserTwo, liveTel2)
	if !assert.NoError(t, err) {
		return
	}
	res, err = s.askTel(liveUserOne, &pb.InquiryTelRequest{Phone: liveTel2})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, []uint32{11000, 22000}, []uint32{res.GetMidTerm().GetPrice(), res.GetFullTerm().GetPrice()})
	}
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")

	// Two mid-term rows of a third number: not one answer, so Karashop (the mock error) is asked.
	_, err = s.db.Pool().Exec(ctx, `INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, tid, inquiry_type, bill_id, pay_id, amount, is_cached, is_paid)
		VALUES ('25252525-2525-4525-8525-252525252523', $1, '982177001122', 124325, 'karashop', 'tel', 't', 'midterm', '2101234560040', '1100099', 11000, false, false),
		       ('25252525-2525-4525-8525-252525252524', $1, '982177001122', 124325, 'karashop', 'tel', 't', 'midterm', '2101234560040', '1100099', 11000, false, false)`, liveUserTwo)
	if !assert.NoError(t, err) {
		return
	}
	_, err = s.askTel(liveUserOne, &pb.InquiryTelRequest{Phone: "982177001122"})
	assert.Equal(t, codes.Unavailable, status.Code(err), "the pair is a miss")
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
}

// TestLiveTelOnlyKarashop (operator decision of 2026-11-14): with the Karashop tel row disabled and
// the Qabzino tel row (id 52; Qabzino has an adapter) enabled, nobody is called: provider-down, no
// row. With no enabled provider: the same answer. The raw inquiry needs no provider.
func TestLiveTelOnlyKarashop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE id = 124325; UPDATE providers SET status = 'enable' WHERE id = 52;`)
	_, err := s.askTel(liveUserOne, &pb.InquiryTelRequest{Phone: liveTel})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 0, s.log.count("info", mockInfoLine), "the other provider is not called")
	assert.Equal(t, 1, s.log.count("error", "tel: no enabled provider for inquiry"))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "tel", "provider": "none", "result": "no_provider"}))

	s = newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE id = 124325;`)
	_, err = s.askTel(liveUserOne, &pb.InquiryTelRequest{Phone: liveTel})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "tel: no enabled provider for inquiry"))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))

	res, err := s.askTel(liveUserOne, &pb.InquiryTelRequest{BillId: karashop.MockBillID, PayId: karashop.MockFinalTermPayID})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, []string{karashop.MockBillID, karashop.MockFinalTermPayID}, []string{res.GetFullTerm().GetBillId(), res.GetFullTerm().GetPayId()})
		assert.Equal(t, uint32(karashop.MockFinalTermAmount), res.GetFullTerm().GetPrice())
		if assert.NotNil(t, res.GetMidTerm(), "an empty mid-term part, as bill") {
			assert.Equal(t, []string{"", "", ""}, []string{res.GetMidTerm().GetBillId(), res.GetMidTerm().GetPayId(), res.GetMidTerm().GetPaymentDeadline()})
			assert.Zero(t, res.GetMidTerm().GetPrice())
		}
	}
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "the raw inquiry writes no row")
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "tel", "provider": "none", "result": "raw"}))
}
