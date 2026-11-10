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
	"qabz/provider/tavanir"
)

func (s *liveWaterStack) askElectricity(user, bill string) (*pb.InquiryElectricityResponse, error) {
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: user, Mobile: "989121234567"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.electricity.Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: bill})
}

// electricityAnswer is every field of the answer, in a fixed order, to compare two answers.
func electricityAnswer(r *pb.InquiryElectricityResponse) []any {
	return []any{r.GetBillId(), r.GetPayId(), r.GetPrice(), r.GetPaymentDeadline(), r.GetPhase(), r.GetCompanyName(), r.GetTariffType(),
		r.GetCustomerType(), r.GetSerialNumber(), r.GetServiceAddress(), r.GetServicePostCode(), r.GetCustomerName()}
}

// TestLiveElectricityInquiryFlow is the feature-ON integration test (task 022). The seed has Tavanir
// (id 2) enabled for electricity inquiry. The first call saves the rows with the details in
// extra_data; the second and third calls within the TTL are answered from the database with the
// same details and no provider call.
func TestLiveElectricityInquiryFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, "")

	res, err := s.askElectricity(liveUserOne, liveBillID)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, []any{liveBillID, tavanir.MockPayID, uint32(tavanir.MockAmount), tavanir.MockDeadline, tavanir.MockPhase, tavanir.MockCompanyName,
		tavanir.MockTariffType, "1", tavanir.MockSerialNumber, tavanir.MockServiceAddress, tavanir.MockServicePostCode, tavanir.MockCustomerName},
		electricityAnswer(res))

	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "one request row")
	var reqStatus, providerName, dialer, tid, billType string
	var trace *string
	assert.NoError(t, s.db.Pool().QueryRow(ctx, `SELECT status, provider_trace_id, provider_name, dialer, tid, bill_type FROM bill_inquiry_requests`).
		Scan(&reqStatus, &trace, &providerName, &dialer, &tid, &billType))
	assert.Equal(t, []string{"success", "tavanir", "989121234567", "electricity"}, []string{reqStatus, providerName, dialer, billType})
	assert.Nil(t, trace, "Tavanir has no trace ID")
	assert.Regexp(t, `^2[0-9]+$`, tid, "the electricity prefix")

	// The saved row, read by a direct SQL query: the columns and each key of extra_data.
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND NOT is_cached AND NOT is_paid AND provider_id = 2
		AND provider_name = 'tavanir' AND bill_type = 'electricity' AND user_id = $2 AND amount = 18000000 AND pay_id = '1800012345'
		AND payment_deadline = '2030-01-01' AND inquiry_type = 'regular' AND bill_id = $1 AND tid IS NULL`, liveBillID, liveUserOne))
	extra := map[string]string{}
	rows, err := s.db.Pool().Query(ctx, `SELECT e.key, e.value FROM bill_inquiries b, jsonb_each_text(b.extra_data) e WHERE b.inquiry_id = $1 AND NOT b.is_cached`, liveBillID)
	if !assert.NoError(t, err) || !assert.NotNil(t, rows) {
		return
	}
	for rows.Next() {
		var k, v string
		if assert.NoError(t, rows.Scan(&k, &v)) {
			extra[k] = v
		}
	}
	rows.Close()
	assert.NoError(t, rows.Err())
	assert.Equal(t, map[string]string{
		"city": tavanir.MockCompanyName, "region": tavanir.MockRegion, // what bill saves
		"phase": tavanir.MockPhase, "company_name": tavanir.MockCompanyName, "tariff_type": tavanir.MockTariffType, "customer_type": "1",
		"serial_number": tavanir.MockSerialNumber, "service_address": tavanir.MockServiceAddress,
		"service_post_code": tavanir.MockServicePostCode, "customer_name": tavanir.MockCustomerName,
	}, extra)
	assert.Equal(t, 1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "the provider (mock) was called")
	assert.Equal(t, 1, s.log.count("info", "inquiry: provider call"))

	// Second call (another user) and third call (the first user), within the TTL: from the database.
	for i, user := range []string{liveUserTwo, liveUserOne} {
		res2, err := s.askElectricity(user, liveBillID)
		if !assert.NoError(t, err) || !assert.NotNil(t, res2) {
			return
		}
		assert.Equal(t, electricityAnswer(res), electricityAnswer(res2), "the same answer and the same details as the provider call")
		assert.Equal(t, 1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "no second provider call")
		assert.Equal(t, i+1, s.log.count("info", "inquiry: answered from database, no provider call"))
		assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")
		assert.Equal(t, i+1, s.count(t, `SELECT count(*) FROM bill_inquiries c JOIN bill_inquiries o ON o.inquiry_id = c.inquiry_id AND NOT o.is_cached
			WHERE c.is_cached AND c.inquiry_id = $1 AND c.extra_data = o.extra_data`, liveBillID), "each copy has the extra_data of the original")
	}
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE is_cached AND user_id = $1`, liveUserTwo), "a copy for the second user")
	assert.Equal(t, 3, s.count(t, `SELECT count(*) FROM bill_inquiries`))

	// A water inquiry of the same bill ID is not answered by the electricity rows: the seed picks
	// parsian for water, which has no adapter.
	_, err = s.ask(liveUserOne, liveBillID)
	assert.Equal(t, codes.Unavailable, status.Code(err))

	// The metrics.
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "electricity", "provider": "tavanir", "result": "provider_ok"}))
	assert.Equal(t, 2.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "electricity", "provider": "tavanir", "result": "db_hit"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "tavanir", "action": "inquiry", "result": "ok"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_call_duration_ms", map[string]string{"provider": "tavanir", "action": "inquiry"}), "one sample: only the provider call")
	assert.Equal(t, 0, s.log.count("error", "electricity: inquiry failed"))
}

// TestLiveElectricityInquiryMockError: with return_type error a new bill gets the provider-down
// answer, the request row has the failed status and no inquiry is saved; a bill that was saved
// earlier is still answered from the database with its saved details. A row that bill wrote (only
// city and region in extra_data) answers with empty detail fields and customerType "0".
func TestLiveElectricityInquiryMockError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnError, "")

	res, err := s.askElectricity(liveUserOne, liveBillID)
	assert.Nil(t, res)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE status = 'failed' AND provider_trace_id IS NULL AND inquiry_id = $1 AND bill_type = 'electricity'`, liveBillID))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiries`), "no inquiry is saved")
	assert.Equal(t, 1, s.log.count("error", "electricity: provider inquiry failed"), "one error line for the failed call")
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "electricity", "provider": "tavanir", "result": "provider_down"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "tavanir", "action": "inquiry", "result": "down"}))

	// A valid saved answer of an earlier call, with details.
	const insert = `INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, inquiry_type, bill_id, pay_id, payment_deadline, amount, extra_data, is_cached, is_paid)
		VALUES ($1, $2, $3, 2, 'tavanir', 'electricity', 'regular', $3, '1000012345', '2031-02-03', 10000000, $4::jsonb, false, false)`
	_, err = s.db.Pool().Exec(ctx, insert, "11111111-1111-4111-8111-111111111111", liveUserTwo, liveBill2,
		`{"city": "c", "region": "r", "phase": "three", "customer_name": "saved name", "customer_type": 4}`)
	if !assert.NoError(t, err) {
		return
	}
	res, err = s.askElectricity(liveUserOne, liveBill2)
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, []any{liveBill2, "1000012345", uint32(10000000), "2031-02-03", "three", "", "", "4", "", "", "", "saved name"}, electricityAnswer(res))
	}
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no new request row")

	// A row that bill wrote: only city and region.
	const billRow = "8888888888888"
	_, err = s.db.Pool().Exec(ctx, insert, "22222222-2222-4222-8222-222222222222", liveUserTwo, billRow, `{"city": "c", "region": "r"}`)
	if !assert.NoError(t, err) {
		return
	}
	res, err = s.askElectricity(liveUserOne, billRow)
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, []any{billRow, "1000012345", uint32(10000000), "2031-02-03", "", "", "", "0", "", "", "", ""}, electricityAnswer(res))
	}
	assert.Equal(t, 2.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "electricity", "provider": "tavanir", "result": "db_hit"}))

	// A saved row whose extra_data cannot be read: one error line, then the provider is asked (the
	// mock error shows).
	const broken = "7777777777777"
	_, err = s.db.Pool().Exec(ctx, insert, "33333333-3333-4333-8333-333333333333", liveUserTwo, broken, `["not an object"]`)
	if !assert.NoError(t, err) {
		return
	}
	_, err = s.askElectricity(liveUserOne, broken)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "inquiry: read saved inquiries failed"))
}

// TestLiveElectricityInquiryProviderCases: with no enabled electricity inquiry provider the answer
// is provider-down with its own error line and result; an invalid bill ID is a client error.
func TestLiveElectricityInquiryProviderCases(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE bill_type = 'electricity' AND action = 'inquiry';`)
	_, err := s.askElectricity(liveUserOne, liveBillID)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "electricity: no enabled provider for inquiry"))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "electricity", "provider": "none", "result": "no_provider"}))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "nothing is written before the provider call")

	_, err = s.askElectricity(liveUserOne, "12345")
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "electricity: no enabled provider for inquiry")+s.log.count("error", "electricity: inquiry failed"), "still one error line in total")
}
