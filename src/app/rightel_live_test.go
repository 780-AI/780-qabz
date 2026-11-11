package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/config"
)

// Task 037: the Rightel mobile inquiry on the run database with the seed (Qabzino, id 34, is
// enabled for mobilemidtermRightel) and the Qabzino mock.

const (
	liveRightelPhone  = "989201234567"
	liveRightelPhone2 = "989209876543"
)

func rightelPhoneReq(phone string) *pb.InquiryPhoneRequest {
	return &pb.InquiryPhoneRequest{Phone: phone, Operator: pb.InquiryPhoneRequest_rightel}
}

// TestLiveRightelInquiryFlow: the first call asks Qabzino (the mock) and saves the mid-term and the
// final-term inquiry; the second call within the TTL (caching.mobile) is answered from the database
// with no provider call and no new request row.
func TestLiveRightelInquiryFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, "")

	res, err := s.askPhone(liveUserOne, rightelPhoneReq(liveRightelPhone))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, []string{"9200000034", "23000341", "9200000034", "63000342"},
		[]string{res.GetMidTerm().GetBillId(), res.GetMidTerm().GetPayId(), res.GetFullTerm().GetBillId(), res.GetFullTerm().GetPayId()})
	assert.Equal(t, []uint32{230000, 630000}, []uint32{res.GetMidTerm().GetPrice(), res.GetFullTerm().GetPrice()})

	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "one request row")
	var st, trace, prov, tid, inquiryID, billType string
	assert.NoError(t, s.db.Pool().QueryRow(ctx, `SELECT status, provider_trace_id, provider_name, tid, inquiry_id, bill_type FROM bill_inquiry_requests`).
		Scan(&st, &trace, &prov, &tid, &inquiryID, &billType))
	assert.Equal(t, []string{"success", "mock-qabzino-0001", "qabzino", liveRightelPhone, "mobilemidtermRightel"}, []string{st, trace, prov, inquiryID, billType})
	assert.Regexp(t, `^6[0-9]+$`, tid, "the Rightel tid prefix")
	for typ, c := range map[string]struct {
		payID  string
		amount int
	}{"midterm": {"23000341", 230000}, "final_term": {"63000342", 630000}} {
		assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND bill_type = 'mobilemidtermRightel' AND NOT is_cached AND NOT is_paid
			AND provider_id = 34 AND provider_name = 'qabzino' AND user_id = $2 AND inquiry_type = $3 AND amount = $4
			AND bill_id = '9200000034' AND pay_id = $5 AND payment_deadline IS NULL AND tid = 'mock-qabzino-0001'`,
			liveRightelPhone, liveUserOne, typ, c.amount, c.payID), typ)
	}
	assert.Equal(t, 1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "the provider (mock) was called")

	res2, err := s.askPhone(liveUserTwo, rightelPhoneReq(liveRightelPhone))
	if !assert.NoError(t, err) || !assert.NotNil(t, res2) {
		return
	}
	assert.Equal(t, []uint32{230000, 630000}, []uint32{res2.GetMidTerm().GetPrice(), res2.GetFullTerm().GetPrice()}, "the same answer")
	assert.Equal(t, "9200000034", res2.GetMidTerm().GetBillId())
	assert.Equal(t, 1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "no second provider call")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")

	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "mobilemidtermRightel", "provider": "qabzino", "result": "provider_ok"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "mobilemidtermRightel", "provider": "qabzino", "result": "db_hit"}))
}

// TestLiveRightelMockError: with return_type error a new phone gets the provider-down answer, the
// request row is failed and no inquiry is saved; saved rows of another phone still answer.
func TestLiveRightelMockError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnError, "")

	res, err := s.askPhone(liveUserOne, rightelPhoneReq(liveRightelPhone))
	assert.Nil(t, res)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE status = 'failed' AND provider_name = 'qabzino' AND inquiry_id = $1 AND bill_type = 'mobilemidtermRightel'`, liveRightelPhone))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiries`))
	assert.Equal(t, 1, s.log.count("error", "mobilemidtermRightel: provider inquiry failed"))

	_, err = s.db.Pool().Exec(ctx, `INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, tid, inquiry_type, bill_id, pay_id, amount, is_cached, is_paid)
		VALUES ('37373737-3737-4373-8373-373737373731', $1, $2, 34, 'qabzino', 'mobilemidtermRightel', 't', 'midterm', '9200000099', '1100099', 11000, false, false),
		       ('37373737-3737-4373-8373-373737373732', $1, $2, 34, 'qabzino', 'mobilemidtermRightel', 't', 'final_term', '9200000099', '2200099', 22000, false, false)`, liveUserTwo, liveRightelPhone2)
	if !assert.NoError(t, err) {
		return
	}
	res, err = s.askPhone(liveUserOne, rightelPhoneReq(liveRightelPhone2))
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, []uint32{11000, 22000}, []uint32{res.GetMidTerm().GetPrice(), res.GetFullTerm().GetPrice()})
	}
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
}

// TestLiveRightelOnlyQabzino (operator rule): with the Qabzino row disabled and another provider
// with an adapter enabled for mobilemidtermRightel, nobody is called: provider-down, no row. With no
// enabled provider: the same answer.
func TestLiveRightelOnlyQabzino(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE id = 34;
		INSERT INTO providers (id, name, subject, bill_type, action, type, priority, timeout, status, updated_at, use_nats)
		VALUES (9037, 'irancell', 'irancell-rightel-test', 'mobilemidtermRightel', 'inquiry', 'retryable', 1, 1000, 'enable', now(), false);`)
	_, err := s.askPhone(liveUserOne, rightelPhoneReq(liveRightelPhone))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 0, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "the other provider is not called")
	assert.Equal(t, 1, s.log.count("error", "mobilemidtermRightel: no enabled provider for inquiry"))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "mobilemidtermRightel", "provider": "none", "result": "no_provider"}))

	s = newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE id = 34;`)
	_, err = s.askPhone(liveUserOne, rightelPhoneReq(liveRightelPhone))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "mobilemidtermRightel: no enabled provider for inquiry"))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
}
