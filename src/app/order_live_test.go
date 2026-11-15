package app

import (
	"context"
	stdcrypto "crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/config"
)

func (s *liveWaterStack) order(user string, req *pb.NewOrderPhoneRequest) (*pb.NewOrderResponse, error) {
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: user, Mobile: "989121234567"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.phone.NewOrderV2(ctx, req)
}

func irancellOrderReq(phone string, price int64) *pb.NewOrderPhoneRequest {
	return &pb.NewOrderPhoneRequest{Phone: phone, Operator: pb.NewOrderPhoneRequest_irancell, Type: pb.NewOrderPhoneRequest_fullTerm,
		PaymentType: pb.NewOrderPhoneRequest_direct, Price: price}
}

// tokenParts checks the token with the public key of the stack and returns its header and payload.
func (s *liveWaterStack) tokenParts(t *testing.T, token string) (map[string]any, map[string]any) {
	t.Helper()
	parts := strings.Split(token, ".")
	if !assert.Len(t, parts, 3) {
		t.FailNow()
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	assert.NoError(t, err)
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	assert.NoError(t, rsa.VerifyPKCS1v15(s.signer.PublicKey(), stdcrypto.SHA256, sum[:], sig), "the signature passes the check")
	var header, payload map[string]any
	for i, dest := range []*map[string]any{&header, &payload} {
		b, err := base64.RawURLEncoding.DecodeString(parts[i])
		if !assert.NoError(t, err) || !assert.NoError(t, json.Unmarshal(b, dest)) {
			t.FailNow()
		}
	}
	return header, payload
}

// TestLiveIrancellOrderDirectPrice is the feature-ON integration test of task 039 with the seed
// (order provider 26 and its fee row 900001): a price and no payId gives one orders row (initial,
// the fee of the row), a token that the public key accepts with the payload of bill, and the total.
func TestLiveIrancellOrderDirectPrice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, "")

	res, err := s.order(liveUserOne, irancellOrderReq("09350000391", 450000))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(459000), res.GetPrice())
	assert.Equal(t, uint32(105), res.GetServiceId())
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE user_id = $1 AND status = 'initial' AND bill_type = 'irancell'
		AND bill_id IS NULL AND pay_id = 'irancell' AND option = 'irancell' AND price = 459000 AND data = '989350000391'
		AND provider_id = 26 AND provider_name = 'irancell' AND fee = 9000 AND fee_type = 'fixed' AND fee_config = '{"amount": 9000}'
		AND fee_id = 900001 AND base_price = 450000 AND is_raw = false AND provider_confirmed = false AND provider_fee IS NULL AND bill_inquiry_id IS NULL`, liveUserOne))
	assert.Equal(t, 0, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no inquiry")

	header, payload := s.tokenParts(t, res.GetOrderId())
	assert.Equal(t, map[string]any{"alg": "RS256", "kid": "qabz", "typ": "JWT"}, header)
	var orderID string
	assert.NoError(t, s.db.Pool().QueryRow(ctx, `SELECT order_id::text FROM orders WHERE user_id = $1`, liveUserOne).Scan(&orderID))
	assert.Equal(t, map[string]any{
		"serviceId": 105.0, "orderId": orderID, "provider": "irancell", "price": 459000.0, "userId": liveUserOne,
		"data": map[string]any{
			"type": "irancell", "phone": "989350000391", "service": "irancell", "billId": "", "payId": "irancell",
			"payAmount": "459000", "rawAmount": "450000", "about": "قبض موبایل",
			"aboutIcon":     "https://public.780.ir/dl/Operator/logOirancell2.svg",
			"receiptMobile": "09350000391", "invoiceMobile": "09350000391", "feeAmount": "9000", "feeDescription": "کارمزد",
		},
	}, payload)

	assert.Equal(t, 1.0, s.metric(t, "qabz_order_create_total", map[string]string{"bill_type": "irancell", "provider": "irancell", "result": "created"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_fee_resolve_total", map[string]string{"source": "provider"}))
}

// TestLiveIrancellOrderDefaultFee: with no fees row the fee is the default of bill: fee 9000,
// fee_type, fee_config and fee_id NULL.
func TestLiveIrancellOrderDefaultFee(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, `DELETE FROM fees`)
	res, err := s.order(liveUserOne, irancellOrderReq("989350000391", 450000))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(459000), res.GetPrice())
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE fee = 9000 AND fee_type IS NULL AND fee_config IS NULL AND fee_id IS NULL`))
	assert.Equal(t, 1.0, s.metric(t, "qabz_fee_resolve_total", map[string]string{"source": "default"}))
}

// TestLiveIrancellOrderBadFee: a fees row whose config gives no fee: fee 0 (NULL), fee_type fixed,
// the fee_id of the row, one error line; the order is saved and the token has no fee keys.
func TestLiveIrancellOrderBadFee(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE fees SET config = '{"x": 1}' WHERE id = 900001`)
	res, err := s.order(liveUserOne, irancellOrderReq("989350000391", 450000))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(450000), res.GetPrice())
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE fee IS NULL AND fee_type = 'fixed' AND fee_config IS NULL AND fee_id = 900001 AND price = 450000`))
	assert.Equal(t, 1, s.log.count("error", "irancell: order fee failed"))
	_, payload := s.tokenParts(t, res.GetOrderId())
	data, ok := payload["data"].(map[string]any)
	if assert.True(t, ok) {
		assert.NotContains(t, data, "feeAmount")
		assert.NotContains(t, data, "feeDescription")
	}
	assert.Equal(t, 1.0, s.metric(t, "qabz_fee_resolve_total", map[string]string{"source": "calc_error"}))
}

// TestLiveIrancellOrderInquiryPrice: no price and no payId: the Irancell inquiry (the mock), also
// within the cache TTL (bill has no order cache for irancell); the final-term inquiry is the price
// and its row is bill_inquiry_id.
func TestLiveIrancellOrderInquiryPrice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, "") // caching.irancell is 1h here
	for i := range 2 {
		req := irancellOrderReq(livePhone, 0)
		req.Type = pb.NewOrderPhoneRequest_midTerm // the type does not choose the inquiry
		res, err := s.order(liveUserOne, req)
		if !assert.NoError(t, err) || !assert.NotNil(t, res) {
			return
		}
		assert.Equal(t, uint32(459000), res.GetPrice(), "the final-term amount + the fee")
		assert.Equal(t, i+1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "a provider call each time")
	}
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM orders o JOIN bill_inquiries b ON b.id::text = o.bill_inquiry_id
		WHERE b.inquiry_type = 'final_term' AND o.base_price = 450000 AND o.pay_id IS NULL AND o.bill_id IS NULL`))
	assert.Equal(t, 2, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`))
	assert.Equal(t, 0.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "irancell", "provider": "irancell", "result": "db_hit"}))
}

// TestLiveIrancellOrderFailures: no enabled order provider and a failed inquiry: the answer, no
// orders row.
func TestLiveIrancellOrderFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWater(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'disable' WHERE id = 26`)
	_, err := s.order(liveUserOne, irancellOrderReq("989350000391", 450000))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1, s.log.count("error", "irancell: no enabled provider for order"))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_create_total", map[string]string{"bill_type": "irancell", "provider": "none", "result": "no_provider"}))

	down := newLiveWater(t, ctx, config.MockReturnError, "")
	_, err = down.order(liveUserOne, irancellOrderReq("989350000391", 0))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 1.0, down.metric(t, "qabz_order_create_total", map[string]string{"bill_type": "irancell", "provider": "irancell", "result": "inquiry_failed"}))

	for _, st := range []*liveWaterStack{s, down} {
		assert.Equal(t, 0, st.count(t, `SELECT count(*) FROM orders`), "no orders row")
	}
}

// TestLiveIrancellOrderTokenKID (task 042): providers.internal.payment_server.kid sets the kid of
// the token header; the signature passes the check and the serviceId of the payload does not change.
func TestLiveIrancellOrderTokenKID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterCfg(t, ctx, config.MockReturnSuccess, "", func(c *config.Config) { c.PaymentServerKID = "bill" })
	res, err := s.order(liveUserOne, irancellOrderReq("09350000391", 450000))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	header, payload := s.tokenParts(t, res.GetOrderId())
	assert.Equal(t, map[string]any{"alg": "RS256", "kid": "bill", "typ": "JWT"}, header)
	assert.Equal(t, 105.0, payload["serviceId"])
}
