package irancellproxy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	pb "qabz/api/pb/irancell"
	"qabz/provider"
)

// fakeClient is a Client that records the request and gives a fixed answer.
type fakeClient struct {
	mu    sync.Mutex
	reqs  []*pb.BillInquiryRequest
	resp  *pb.BillInquiryResponse
	err   error
	block bool // wait for the context
	dl    bool // the context had a deadline
}

func (f *fakeClient) BillInquiry(ctx context.Context, in *pb.BillInquiryRequest, _ ...grpc.CallOption) (*pb.BillInquiryResponse, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, in)
	_, f.dl = ctx.Deadline()
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.resp, f.err
}

func (f *fakeClient) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func newAdapter(t *testing.T, c Client) *Adapter {
	t.Helper()
	a, err := New(Options{Enabled: true, Client: c, Timeout: time.Second})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	return a
}

var mobileReq = provider.InquiryRequest{BillType: provider.BillTypeIrancell, UserID: "u1", Dialer: "989121234567", InquiryID: "989351234567"}

func inq(amount int64, typ string) *pb.BillInquiryResponse_Inquiry {
	return &pb.BillInquiryResponse_Inquiry{Amount: amount, InquiryType: typ}
}

func TestNewRejects(t *testing.T) {
	_, err := New(Options{Timeout: time.Second})
	assert.Error(t, err)
	_, err = New(Options{Client: &fakeClient{}})
	assert.Error(t, err)
	_, err = New(Options{Client: &fakeClient{}, Timeout: -1})
	assert.Error(t, err)
}

func TestNameAndNotSupported(t *testing.T) {
	a := newAdapter(t, &fakeClient{})
	assert.Equal(t, "irancell", a.Name(), "the providers.name of the seed")
	assert.True(t, a.Enabled())
	var nilAdapter *Adapter
	assert.False(t, nilAdapter.Enabled())
	_, err := a.Pay(context.Background(), provider.PaymentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Status(context.Background(), provider.StatusRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.GetDocument(context.Background(), provider.DocumentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Settle(context.Background(), provider.SettleRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
}

func TestInquiryRequestFieldsAndAnswer(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{
		BaseResponse: &pb.BillBaseResponse{Code: 200, Status: "success", Message: "ok", ProviderTraceId: "tr-9"},
		Inquiries:    []*pb.BillInquiryResponse_Inquiry{inq(120000, "midterm"), nil, inq(340000, "final_term")},
	}}
	res, err := newAdapter(t, f).Inquiry(context.Background(), mobileReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.Len(t, f.reqs, 1) {
		return
	}
	assert.Equal(t, "u1", f.reqs[0].GetUserId())
	assert.Equal(t, "989121234567", f.reqs[0].GetDialer())
	assert.Equal(t, "989351234567", f.reqs[0].GetInquiryId(), "the phone is sent as it is")
	assert.True(t, f.dl, "the call has the timeout")

	assert.Equal(t, provider.ResponseSuccess, res.Status)
	assert.EqualValues(t, 200, res.Code)
	assert.Equal(t, successMessage, res.Message)
	assert.Equal(t, "tr-9", res.ProviderTraceID)
	assert.Equal(t, "", res.BillID, "Irancell gives no bill ID")
	assert.Nil(t, res.Details)
	assert.Equal(t, []provider.InquiryItem{
		{Amount: 120000, InquiryType: provider.InquiryTypeMidterm},
		{Amount: 340000, InquiryType: provider.InquiryTypeFinalTerm},
	}, res.Inquiries, "a nil inquiry is skipped; no pay ID, no deadline")
}

func TestInquiryNegativeAmountIsZero(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{
		BaseResponse: &pb.BillBaseResponse{Status: "success"},
		Inquiries:    []*pb.BillInquiryResponse_Inquiry{inq(-5000, "midterm"), inq(0, "final_term")},
	}}
	res, err := newAdapter(t, f).Inquiry(context.Background(), mobileReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.Len(t, res.Inquiries, 2) {
		return
	}
	assert.EqualValues(t, 0, res.Inquiries[0].Amount, "bill: a negative amount is 0")
	assert.EqualValues(t, 0, res.Inquiries[1].Amount)
	assert.Equal(t, provider.ResponseSuccess, res.Status)
}

func TestInquiryNoDebt(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: &pb.BillBaseResponse{Code: 200, Status: "success", Message: "m", ProviderTraceId: "tr"}}}
	res, err := newAdapter(t, f).Inquiry(context.Background(), mobileReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, provider.ResponseSuccess, res.Status)
	assert.Equal(t, "tr", res.ProviderTraceID)
	assert.Equal(t, []provider.InquiryItem{{InquiryType: provider.InquiryTypeMidterm}, {InquiryType: provider.InquiryTypeFinalTerm}},
		res.Inquiries, "bill: a success answer with no inquiry is two inquiries of amount 0")
}

func TestInquiryRefused(t *testing.T) {
	cases := []struct {
		name       string
		base       *pb.BillBaseResponse
		inquiries  []*pb.BillInquiryResponse_Inquiry
		wantStatus string
	}{
		{"failed status", &pb.BillBaseResponse{Code: 400, Status: "failed", Message: "not an Irancell number", ProviderTraceId: "t"}, nil, "failed"},
		{"pending status", &pb.BillBaseResponse{Code: 500, Status: "pending", Message: "later"}, nil, "pending"},
		{"empty status", &pb.BillBaseResponse{Code: 400, Message: "x"}, nil, provider.ResponseFailed},
		{"no base response", nil, nil, provider.ResponseFailed},
		{"only nil inquiries", &pb.BillBaseResponse{Status: "failed", Message: "x"}, []*pb.BillInquiryResponse_Inquiry{nil}, "failed"},
	}
	for _, c := range cases {
		f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: c.base, Inquiries: c.inquiries}}
		res, err := newAdapter(t, f).Inquiry(context.Background(), mobileReq)
		if !assert.NoError(t, err, c.name) || !assert.NotNil(t, res, c.name) {
			continue
		}
		assert.Equal(t, c.wantStatus, res.Status, c.name)
		assert.Empty(t, res.Inquiries, c.name)
		assert.Equal(t, c.base.GetMessage(), res.Message, c.name)
		assert.Equal(t, c.base.GetCode(), res.Code, c.name)
		assert.Equal(t, c.base.GetProviderTraceId(), res.ProviderTraceID, c.name)
	}
}

func TestInquiryFailuresAreProviderDown(t *testing.T) {
	cause := errors.New("connection refused")
	res, err := newAdapter(t, &fakeClient{err: cause}).Inquiry(context.Background(), mobileReq)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, cause, "the cause is kept for the log line")

	res, err = newAdapter(t, &fakeClient{}).Inquiry(context.Background(), mobileReq) // a nil answer and a nil error
	assert.Nil(t, res)
	assert.ErrorIs(t, err, provider.ErrProviderDown)

	// A nil context is valid.
	var nilCtx context.Context
	_, err = newAdapter(t, &fakeClient{err: cause}).Inquiry(nilCtx, mobileReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
}

func TestInquiryTimeout(t *testing.T) {
	f := &fakeClient{block: true}
	a, err := New(Options{Enabled: true, Client: f, Timeout: 20 * time.Millisecond})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		return
	}
	start := time.Now()
	res, err := a.Inquiry(context.Background(), mobileReq)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestInquiryDisabledSendsNothing(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{}}
	a, err := New(Options{Enabled: false, Client: f, Timeout: time.Second})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		return
	}
	assert.False(t, a.Enabled())
	res, err := a.Inquiry(context.Background(), mobileReq)
	assert.Nil(t, res)
	if assert.ErrorIs(t, err, provider.ErrProviderDown) {
		assert.Contains(t, err.Error(), "providers.internal.irancell.enabled is false")
	}
	assert.Equal(t, 0, f.calls(), "no call to the irancell service")
}

func TestInquiryOtherBillTypesAndBadAdapter(t *testing.T) {
	f := &fakeClient{}
	for _, bt := range []string{"water", "electricity", "gas", "mobile", ""} {
		_, err := newAdapter(t, f).Inquiry(context.Background(), provider.InquiryRequest{BillType: bt, InquiryID: "989351234567"})
		assert.ErrorIs(t, err, provider.ErrNotSupported, bt)
		assert.NotErrorIs(t, err, provider.ErrProviderDown, bt)
	}
	assert.Equal(t, 0, f.calls())

	var nilAdapter *Adapter
	_, err := nilAdapter.Inquiry(context.Background(), mobileReq)
	assert.Error(t, err)
	_, err = (&Adapter{}).Inquiry(context.Background(), mobileReq)
	assert.Error(t, err)
}

func TestDial(t *testing.T) {
	c, err := Dial("")
	if assert.NoError(t, err) && assert.NotNil(t, c) {
		a := newAdapter(t, c)
		_, err = a.Inquiry(context.Background(), mobileReq)
		if assert.ErrorIs(t, err, provider.ErrProviderDown) {
			assert.Contains(t, err.Error(), "providers.internal.irancell.grpc_address is not set")
		}
	}
	// A lazy connection: no running service is needed. Port 1 refuses, so a call fails as provider down.
	c, err = Dial("127.0.0.1:1")
	if assert.NoError(t, err) && assert.NotNil(t, c) {
		a, err := New(Options{Enabled: true, Client: c, Timeout: 2 * time.Second})
		if assert.NoError(t, err) && assert.NotNil(t, a) {
			_, err = a.Inquiry(context.Background(), mobileReq)
			assert.ErrorIs(t, err, provider.ErrProviderDown)
		}
	}
}

func TestMockFixtures(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Inquiry) {
		return
	}
	a, b := f.Inquiry(mobileReq), f.Inquiry(mobileReq)
	if !assert.NotNil(t, a) || !assert.NotNil(t, b) {
		return
	}
	assert.NotSame(t, a, b, "a new value on each call")
	assert.Equal(t, provider.ResponseSuccess, a.Status)
	assert.Equal(t, MockTrace, a.ProviderTraceID)
	assert.Equal(t, "", a.BillID)
	assert.Equal(t, []provider.InquiryItem{
		{Amount: 150000, InquiryType: provider.InquiryTypeMidterm},
		{Amount: 450000, InquiryType: provider.InquiryTypeFinalTerm},
	}, a.Inquiries)

	// Through provider.NewMock: success, error, off.
	real := newAdapter(t, &fakeClient{err: errors.New("must not be called")})
	on, err := provider.NewMock(real, provider.MockSettings{Enable: true, ReturnType: provider.MockReturnSuccess}, f, nil)
	if assert.NoError(t, err) && assert.NotNil(t, on) {
		res, err := on.Inquiry(context.Background(), mobileReq)
		if assert.NoError(t, err) && assert.NotNil(t, res) {
			assert.Len(t, res.Inquiries, 2)
		}
	}
	down, err := provider.NewMock(real, provider.MockSettings{Enable: true, ReturnType: provider.MockReturnError}, f, nil)
	if assert.NoError(t, err) && assert.NotNil(t, down) {
		_, err = down.Inquiry(context.Background(), mobileReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown)
	}
	off, err := provider.NewMock(real, provider.MockSettings{}, f, nil)
	if assert.NoError(t, err) {
		assert.Same(t, provider.ProviderInterface(real), off, "off is the adapter itself")
	}
}

// SupportsInquiry gives true only for the bill types that Inquiry answers (task 036); Inquiry of
// any other bill type is ErrNotSupported. A nil adapter answers the same.
func TestSupportsInquiry(t *testing.T) {
	want := map[string]bool{
		provider.BillTypeWater:       false,
		provider.BillTypeElectricity: false,
		provider.BillTypeGas:         false,
		provider.BillTypeIrancell:    true,
		provider.BillTypeMobile:      false,
		"tel":                        false,
		"mobilemidterm":              false,
		"":                           false,
	}
	var nilAdapter *Adapter
	for billType, ok := range want {
		assert.Equal(t, ok, (&Adapter{}).SupportsInquiry(billType), billType)
		assert.Equal(t, ok, nilAdapter.SupportsInquiry(billType), billType)
	}
}
