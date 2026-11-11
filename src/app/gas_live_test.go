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
	liveGasCode  = "0012345678" // a participate code: not a valid bill ID
	liveGasCode2 = "0098765432"
	liveGasBill  = "1234567890125" // qabzino.MockGasBillID
)

// qabzinoGas makes Qabzino the enabled gas inquiry provider (the seed has parsian, id 43, enabled
// and Qabzino, id 35, disabled).
const qabzinoGas = `UPDATE providers SET status = 'enable' WHERE id = 35; UPDATE providers SET status = 'disable' WHERE id = 43;`

func (s *liveWaterStack) askGas(user string, req *pb.InquiryBillPayRequest) (*pb.InquiryBillPayResponse, error) {
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: user, Mobile: "989121234567"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.gas.Inquiry(ctx, req)
}

// TestLiveGasInquiryFlow is the feature-ON integration test (task 023): the first call asks
// Qabzino (the mock) and saves the rows under the participate code, with the details; the second
// call within the TTL is answered from the database with no provider call; another participate
// code is another key.
func TestLiveGasInquiryFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, qabzinoGas)

	res, err := s.askGas(liveUserOne, &pb.InquiryBillPayRequest{ParticipateCode: liveGasCode})
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(32000000), res.GetPrice())
	assert.Equal(t, "3200012345", res.GetPayId())
	assert.Equal(t, "2030-01-01", res.GetPaymentDeadline())
	assert.Equal(t, liveGasBill, res.GetBillId())

	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "one request row")
	var st, trace, provider, dialer, tid, inquiryID, billType string
	assert.NoError(t, s.db.Pool().QueryRow(ctx, `SELECT status, provider_trace_id, provider_name, dialer, tid, inquiry_id, bill_type FROM bill_inquiry_requests`).
		Scan(&st, &trace, &provider, &dialer, &tid, &inquiryID, &billType))
	assert.Equal(t, []string{"success", "mock-qabzino-0001", "qabzino", "989121234567", liveGasCode, "gas"}, []string{st, trace, provider, dialer, inquiryID, billType})
	assert.Regexp(t, `^3[0-9]+$`, tid, "the gas tid prefix")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND bill_id = $2 AND bill_type = 'gas' AND NOT is_cached AND NOT is_paid
		AND provider_id = 35 AND provider_name = 'qabzino' AND user_id = $3 AND amount = 32000000 AND pay_id = '3200012345' AND payment_deadline = '2030-01-01' AND inquiry_type = 'regular'`,
		liveGasCode, liveGasBill, liveUserOne))
	var keys int
	var city, kind string
	assert.NoError(t, s.db.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM jsonb_object_keys(extra_data)), extra_data->>'city', extra_data->>'kind' FROM bill_inquiries WHERE NOT is_cached`).Scan(&keys, &city, &kind))
	assert.Equal(t, 2, keys, "extra_data has bill's two keys only")
	assert.NotEmpty(t, city)
	assert.NotEmpty(t, kind)
	assert.Equal(t, 1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "the provider (mock) was called")
	assert.Equal(t, 1, s.log.count("info", "inquiry: provider call"))

	// Second call, within the TTL, another user, with a bill ID next to the code: from the database.
	res2, err := s.askGas(liveUserTwo, &pb.InquiryBillPayRequest{ParticipateCode: liveGasCode, BillId: liveGasBill})
	if !assert.NoError(t, err) || !assert.NotNil(t, res2) {
		return
	}
	assert.Equal(t, []any{res.GetPrice(), res.GetPayId(), res.GetPaymentDeadline(), res.GetBillId()},
		[]any{res2.GetPrice(), res2.GetPayId(), res2.GetPaymentDeadline(), res2.GetBillId()}, "the same answer")
	assert.Equal(t, 1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "no second provider call")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE is_cached AND user_id = $1 AND inquiry_id = $2 AND extra_data->>'city' = $3`, liveUserTwo, liveGasCode, city), "a copy for the second user, with the details")
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM bill_inquiries`))

	// Another participate code is another cache key: a provider call.
	res3, err := s.askGas(liveUserOne, &pb.InquiryBillPayRequest{ParticipateCode: liveGasCode2})
	if assert.NoError(t, err) && assert.NotNil(t, res3) {
		assert.Equal(t, uint32(32000000), res3.GetPrice())
	}
	assert.Equal(t, 2, s.log.count("info", "provider mock: inquiry answered with no call to the provider"))
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND NOT is_cached`, liveGasCode2))

	assert.Equal(t, 2.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "gas", "provider": "qabzino", "result": "provider_ok"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "gas", "provider": "qabzino", "result": "db_hit"}))
	assert.Equal(t, 2.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "qabzino", "action": "inquiry", "result": "ok"}))
	assert.Equal(t, 0, s.log.count("error", "gas: inquiry failed"))

	// A water inquiry in the same App is as before: its own provider pick (parsian, no adapter).
	_, err = s.ask(liveUserOne, liveBillID)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "water", "provider": "parsian", "result": "error"}))
}

// TestLiveGasInquiryMockError: with return_type error a new participate code gets the
// provider-down answer, the request row has the failed status and no inquiry is saved; a code that
// was saved earlier is still answered from the database.
func TestLiveGasInquiryMockError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnError, qabzinoGas)

	res, err := s.askGas(liveUserOne, &pb.InquiryBillPayRequest{ParticipateCode: liveGasCode})
	assert.Nil(t, res)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, "استعلام از سوی تامین کننده موقتا غیر فعال می باشد", status.Convert(err).Message(), "bill's provider-down text")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE status = 'failed' AND provider_trace_id IS NULL AND inquiry_id = $1 AND bill_type = 'gas'`, liveGasCode))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiries`), "no inquiry is saved")
	assert.Equal(t, 1, s.log.count("error", "gas: provider inquiry failed"), "one error line for the failed call")
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "gas", "provider": "qabzino", "result": "provider_down"}))

	// A valid saved answer of an earlier call, as bill writes it (extra_data with city and kind).
	_, err = s.db.Pool().Exec(ctx, `INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, tid, inquiry_type, bill_id, pay_id, payment_deadline, amount, is_cached, is_paid, extra_data)
		VALUES ('22222222-2222-4222-8222-222222222222', $1, $2, 35, 'qabzino', 'gas', 't', 'regular', $3, '1000012345', '2031-02-03', 10000000, false, false, '{"city":"c","kind":"k"}')`, liveUserTwo, liveGasCode2, liveGasBill)
	if !assert.NoError(t, err) {
		return
	}
	res, err = s.askGas(liveUserOne, &pb.InquiryBillPayRequest{ParticipateCode: liveGasCode2})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(10000000), res.GetPrice())
		assert.Equal(t, "2031-02-03", res.GetPaymentDeadline())
		assert.Equal(t, "1000012345", res.GetPayId())
		assert.Equal(t, liveGasBill, res.GetBillId())
	}
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "gas", "provider": "qabzino", "result": "db_hit"}))
}

// TestLiveGasInquiryProviderCases: with the seed, parsian is picked for gas and has no adapter:
// provider-down with one error line. With no enabled gas inquiry provider: the same answer with its
// own line. Invalid requests write nothing.
func TestLiveGasInquiryProviderCases(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	s := newLiveWater(t, ctx, config.MockReturnSuccess, "")
	_, err := s.askGas(liveUserOne, &pb.InquiryBillPayRequest{ParticipateCode: liveGasCode})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "gas: provider inquiry failed"))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "gas", "provider": "parsian", "result": "error"}), "parsian was picked and has no adapter")
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "nothing is written before the provider call")

	s = newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE bill_type = 'gas' AND action = 'inquiry';`)
	_, err = s.askGas(liveUserOne, &pb.InquiryBillPayRequest{ParticipateCode: liveGasCode})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "gas: no enabled provider for inquiry"))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "gas", "provider": "none", "result": "no_provider"}))

	// Client errors: no log line, no row.
	errorsBefore := len(s.log.lines)
	for name, c := range map[string]struct {
		req  *pb.InquiryBillPayRequest
		want codes.Code
	}{
		"nothing":       {&pb.InquiryBillPayRequest{}, codes.InvalidArgument},
		"bill ID only":  {&pb.InquiryBillPayRequest{BillId: liveGasBill}, codes.InvalidArgument},
		"a bad bill ID": {&pb.InquiryBillPayRequest{BillId: "12345", ParticipateCode: liveGasCode}, codes.InvalidArgument},
		"a bad pay ID":  {&pb.InquiryBillPayRequest{BillId: liveGasBill, PayId: "2500012345"}, codes.InvalidArgument},
	} {
		_, err = s.askGas(liveUserOne, c.req)
		assert.Equal(t, c.want, status.Code(err), name)
	}
	assert.Len(t, s.log.lines, errorsBefore, "no log line for a client error")
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiries`))
}

// TestLiveGasLongParticipateCode: a participate code longer than the inquiry_id column
// (VARCHAR(15)) fails at the request row, as in bill: Internal, one error line, no provider call.
func TestLiveGasLongParticipateCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, qabzinoGas)
	_, err := s.askGas(liveUserOne, &pb.InquiryBillPayRequest{ParticipateCode: "1234567890123456"})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "gas: inquiry failed"))
	assert.Equal(t, 0, s.log.count("info", "provider mock: inquiry answered with no call to the provider"))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
	// 15 characters fit.
	res, err := s.askGas(liveUserOne, &pb.InquiryBillPayRequest{ParticipateCode: "123456789012345"})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(32000000), res.GetPrice())
	}
}

// TestLiveGasRawInquiry: a request with billId and payId is answered from the two IDs with every
// gas provider disabled: no provider, no row, no log line; the metric counts it as raw.
func TestLiveGasRawInquiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE bill_type = 'gas';`)
	lines := len(s.log.lines)
	for _, code := range []string{"", liveGasCode} {
		res, err := s.askGas(liveUserOne, &pb.InquiryBillPayRequest{BillId: liveGasBill, PayId: "3200012345", ParticipateCode: code})
		if assert.NoError(t, err) && assert.NotNil(t, res) {
			assert.Equal(t, []any{uint32(32000000), "3200012345", liveGasBill, ""}, []any{res.GetPrice(), res.GetPayId(), res.GetBillId(), res.GetPaymentDeadline()})
		}
	}
	assert.Len(t, s.log.lines, lines, "no log line")
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiries`))
	assert.Equal(t, 2.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "gas", "provider": "none", "result": "raw"}))
}
