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
	"qabz/provider/jiring"
	"qabz/provider/parsianproxy"
)

const (
	liveMCIPhone  = "989121112233"
	liveMCIPhone2 = "989124445566"
	// enableJiringSQL enables the Jiring Switch row of (mobile, inquiry) (seed id 28: priority 10,
	// disabled) and gives it priority 1, so it comes before Parsian (id 39, priority 2; the order is
	// priority ascending) and is the pick (the operator constraint of task 036: Jiring Switch first).
	enableJiringSQL = `UPDATE providers SET status = 'enable', priority = 1 WHERE id = 28;`
)

func mciPhoneReq(phone string) *pb.InquiryPhoneRequest {
	return &pb.InquiryPhoneRequest{Phone: phone, Operator: pb.InquiryPhoneRequest_mci}
}

// newLiveMCI is newLiveWater with the Jiring Switch and the Parsian mock on (each with its own
// return type) and caching.mobile 1h.
func newLiveMCI(t *testing.T, ctx context.Context, jiringReturn, parsianReturn, setupSQL string) *liveWaterStack {
	t.Helper()
	return newLiveWaterCfg(t, ctx, config.MockReturnSuccess, setupSQL, func(c *config.Config) {
		c.JiringSwitchEnabled = true
		c.DebugMockJiringSwitchEnable = true
		c.DebugMockJiringSwitchReturnType = jiringReturn
		c.ParsianEnabled = true
		c.DebugMockParsianEnable = true
		c.DebugMockParsianReturnType = parsianReturn
		c.CachingMobile = time.Hour
	})
}

// TestLiveMCIInquiryFlow is the feature-ON integration test (task 036) with the Jiring Switch row
// enabled: the first call asks Jiring Switch (the mock) and saves the mid-term and the final-term
// inquiry; the second call within the TTL is answered from the database with no provider call;
// Parsian is never called. A request with no operator is the MCI inquiry too (the enum value 0).
func TestLiveMCIInquiryFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveMCI(t, ctx, config.MockReturnSuccess, config.MockReturnSuccess, enableJiringSQL)

	res, err := s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, []uint32{jiring.MockMidtermAmount, jiring.MockFinalTermAmount}, []uint32{res.GetMidTerm().GetPrice(), res.GetFullTerm().GetPrice()})
	assert.Equal(t, []string{jiring.MockMidtermPayID, jiring.MockFinalTermPayID}, []string{res.GetMidTerm().GetPayId(), res.GetFullTerm().GetPayId()})

	var st, prov, inquiryID, billType, tid string
	assert.NoError(t, s.db.Pool().QueryRow(ctx, `SELECT status, provider_name, inquiry_id, bill_type, tid FROM bill_inquiry_requests`).
		Scan(&st, &prov, &inquiryID, &billType, &tid))
	assert.Equal(t, []string{"success", "jiring-switch", liveMCIPhone, "mobile"}, []string{st, prov, inquiryID, billType})
	assert.Regexp(t, `^5[0-9]+$`, tid, "the mobile tid prefix")
	for typ, amount := range map[string]int{"midterm": jiring.MockMidtermAmount, "final_term": jiring.MockFinalTermAmount} {
		assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND bill_type = 'mobile' AND NOT is_cached
			AND provider_id = 28 AND provider_name = 'jiring-switch' AND user_id = $2 AND inquiry_type = $3 AND amount = $4 AND bill_id = $5`,
			liveMCIPhone, liveUserOne, typ, amount, jiring.MockBillID), typ)
	}
	assert.Equal(t, 1, s.log.count("info", mockInfoLine), "one provider (mock) call")

	// Second call, within the TTL, another user and no operator: from the database.
	res2, err := s.askPhone(liveUserTwo, &pb.InquiryPhoneRequest{Phone: liveMCIPhone})
	if assert.NoError(t, err) && assert.NotNil(t, res2) {
		assert.Equal(t, []uint32{jiring.MockMidtermAmount, jiring.MockFinalTermAmount}, []uint32{res2.GetMidTerm().GetPrice(), res2.GetFullTerm().GetPrice()}, "the same answer")
	}
	assert.Equal(t, 1, s.log.count("info", mockInfoLine), "no second provider call")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE is_cached AND user_id = $1 AND inquiry_id = $2`, liveUserTwo, liveMCIPhone))

	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "mobile", "provider": "jiring-switch", "result": "provider_ok"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "mobile", "provider": "jiring-switch", "result": "db_hit"}))
	assert.Equal(t, 0.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "parsian", "action": "inquiry", "result": "ok"}), "Parsian is not called")
}

// TestLiveMCIParsianWithSeed: with the seed (the Jiring Switch row is disabled, and its priority 10
// is after the priority 2 of Parsian), Parsian is the pick of (mobile, inquiry) and answers with its
// trace ID and its payment deadline.
func TestLiveMCIParsianWithSeed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveMCI(t, ctx, config.MockReturnSuccess, config.MockReturnSuccess, "")

	res, err := s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, []uint32{parsianproxy.MockMidtermAmount, parsianproxy.MockFinalTermAmount}, []uint32{res.GetMidTerm().GetPrice(), res.GetFullTerm().GetPrice()})
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE status = 'success' AND provider_name = 'parsian' AND provider_trace_id = $1`, parsianproxy.MockTrace))
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE provider_id = 39 AND provider_name = 'parsian' AND bill_type = 'mobile'
		AND payment_deadline IS NOT NULL AND inquiry_id = $1`, liveMCIPhone))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE provider_name = 'jiring-switch'`), "Jiring Switch is not called")
}

// TestLiveMCIErrorIsTheAnswer (operator rule of task 036): when the pick (Jiring Switch) fails, its
// failure is the answer: provider-down, one failed request row, no saved inquiry, and Parsian is NOT
// called in the same request (only the Provider Switcher moves a bill type to another provider). A
// phone that was saved earlier is still answered from the database.
func TestLiveMCIErrorIsTheAnswer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveMCI(t, ctx, config.MockReturnError, config.MockReturnSuccess, enableJiringSQL)

	res, err := s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone))
	assert.Nil(t, res)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "one request row: no second provider")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE status = 'failed' AND provider_name = 'jiring-switch' AND inquiry_id = $1`, liveMCIPhone))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiries`))
	assert.Equal(t, 1, s.log.count("error", "mobile: provider inquiry failed"))
	assert.Equal(t, 0.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "parsian", "action": "inquiry", "result": "ok"}), "Parsian is not called")

	_, err = s.db.Pool().Exec(ctx, `INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, tid, inquiry_type, amount, is_cached, is_paid)
		VALUES ('36363636-3636-4363-8363-363636363631', $1, $2, 28, 'jiring-switch', 'mobile', 't', 'midterm', 11000, false, false),
		       ('36363636-3636-4363-8363-363636363632', $1, $2, 28, 'jiring-switch', 'mobile', 't', 'final_term', 22000, false, false)`, liveUserTwo, liveMCIPhone2)
	if !assert.NoError(t, err) {
		return
	}
	res, err = s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone2))
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, []uint32{11000, 22000}, []uint32{res.GetMidTerm().GetPrice(), res.GetFullTerm().GetPrice()})
	}
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
}
