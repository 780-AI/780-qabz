package qabzinoproxy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	pb "qabz/api/pb/qabzino"
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

func newAdapter(t *testing.T, c Client) *Adapter {
	t.Helper()
	a, err := New(Options{Client: c, Timeout: time.Second})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	a.newTrace = func() string { return "trace-1" }
	return a
}

var waterReq = provider.InquiryRequest{BillType: "water", UserID: "u1", Dialer: "989121234567", InquiryID: "1234567890125"}

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
	assert.Equal(t, "qabzinoproxy", a.Name())
	_, err := a.Pay(context.Background(), provider.PaymentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Status(context.Background(), provider.StatusRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.GetDocument(context.Background(), provider.DocumentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Settle(context.Background(), provider.SettleRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
}

func TestInquiryRequestFields(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: &pb.BillBaseResponse{Status: "success", Tid: "t1", BillId: "b1"},
		Inquiries: []*pb.BillInquiryResponse_Inquiry{{PayId: "2500012345", Amount: 1, PaymentDeadline: "2030-01-01"}}}}
	a := newAdapter(t, f)
	_, err := a.Inquiry(context.Background(), waterReq)
	assert.NoError(t, err)
	if !assert.Len(t, f.reqs, 1) {
		return
	}
	r := f.reqs[0]
	assert.Equal(t, "u1", r.GetUserId())
	assert.Equal(t, "989121234567", r.GetDialer())
	assert.Equal(t, "1234567890125", r.GetInquiryId())
	assert.Equal(t, "trace-1", r.GetTraceNumber())
	assert.Equal(t, "water", r.GetBillType())
	assert.True(t, f.dl, "each call has a deadline (providers.qabzino.timeout)")
}

func TestInquiryAnswerMapping(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{
		BaseResponse: &pb.BillBaseResponse{Code: 0, Status: "ignored", Tid: "t1", BillId: "b1"},
		Inquiries: []*pb.BillInquiryResponse_Inquiry{
			{PayId: "2500012345", Amount: 999, PaymentDeadline: "2030-01-01", InquiryType: "x"},
		}}}
	resp, err := newAdapter(t, f).Inquiry(context.Background(), waterReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) || !assert.Len(t, resp.Inquiries, 1) {
		return
	}
	assert.Equal(t, provider.ResponseSuccess, resp.Status, "an answer with inquiries is a success")
	assert.Equal(t, "t1", resp.ProviderTraceID)
	assert.Equal(t, "b1", resp.BillID)
	assert.Equal(t, provider.InquiryItem{PayID: "2500012345", Amount: 25000000, PaymentDeadline: "2030-01-01", InquiryType: "regular"}, resp.Inquiries[0],
		"the amount comes from the pay ID, not from the amount field")
}

func TestInquiryEmptyPayIDIsPaid(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: &pb.BillBaseResponse{Tid: "t"},
		Inquiries: []*pb.BillInquiryResponse_Inquiry{{PayId: "", Amount: 5000}}}}
	resp, err := newAdapter(t, f).Inquiry(context.Background(), waterReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) || !assert.Len(t, resp.Inquiries, 1) {
		return
	}
	assert.Equal(t, uint64(0), resp.Inquiries[0].Amount, "a paid bill has an empty pay ID: amount 0 although the field is above 0")
	assert.Equal(t, "", resp.Inquiries[0].PayID)
}

func TestInquiryBadPayIDFallsBackToAmount(t *testing.T) {
	for _, payID := range []string{"123", "abcdefgh", "99999999999999999999999"} {
		f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: &pb.BillBaseResponse{},
			Inquiries: []*pb.BillInquiryResponse_Inquiry{{PayId: payID, Amount: 4242}}}}
		resp, err := newAdapter(t, f).Inquiry(context.Background(), waterReq)
		if assert.NoError(t, err, payID) && assert.NotNil(t, resp, payID) && assert.Len(t, resp.Inquiries, 1, payID) {
			assert.Equal(t, uint64(4242), resp.Inquiries[0].Amount, payID)
		}
	}
}

func TestInquiryWithoutInquiries(t *testing.T) {
	cases := []struct {
		name       string
		base       *pb.BillBaseResponse
		wantStatus string
		wantItems  int
	}{
		{"no debt", &pb.BillBaseResponse{Status: "success", Code: 200, Message: "no debt", Tid: "t9"}, "success", 1},
		{"failed", &pb.BillBaseResponse{Status: "failed", Code: 4, Message: "bill not found", Tid: "t9"}, "failed", 0},
		{"empty status", &pb.BillBaseResponse{Code: 4, Message: "?"}, "failed", 0},
		{"nil base", nil, "failed", 0},
	}
	for _, c := range cases {
		f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: c.base}}
		resp, err := newAdapter(t, f).Inquiry(context.Background(), waterReq)
		if !assert.NoError(t, err, c.name) || !assert.NotNil(t, resp, c.name) {
			continue
		}
		assert.Equal(t, c.wantStatus, resp.Status, c.name)
		assert.Len(t, resp.Inquiries, c.wantItems, c.name)
		assert.Equal(t, c.base.GetMessage(), resp.Message, c.name)
		assert.Equal(t, c.base.GetTid(), resp.ProviderTraceID, c.name)
		assert.Equal(t, "", resp.BillID, c.name, "bill drops the bill ID of an answer with no inquiry")
	}
}

func TestInquiryNilInquiryEntry(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: &pb.BillBaseResponse{Status: "failed", Message: "m"},
		Inquiries: []*pb.BillInquiryResponse_Inquiry{nil}}}
	resp, err := newAdapter(t, f).Inquiry(context.Background(), waterReq)
	if assert.NoError(t, err) && assert.NotNil(t, resp) {
		assert.Empty(t, resp.Inquiries, "a nil entry is skipped; with none left the answer is the base answer")
		assert.Equal(t, "failed", resp.Status)
	}
}

func TestInquiryErrors(t *testing.T) {
	a := newAdapter(t, &fakeClient{err: errors.New("connection refused")})
	resp, err := a.Inquiry(context.Background(), waterReq)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, provider.ErrProviderDown)

	a = newAdapter(t, &fakeClient{}) // nil, nil
	resp, err = a.Inquiry(context.Background(), waterReq)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, provider.ErrProviderDown, "a nil answer is a failed call")

	_, err = a.Inquiry(context.Background(), provider.InquiryRequest{BillType: "gas", InquiryID: "x"})
	assert.ErrorIs(t, err, provider.ErrNotSupported, "only water is implemented")

	var nilAdapter *Adapter
	_, err = nilAdapter.Inquiry(context.Background(), waterReq)
	assert.Error(t, err)
}

func TestInquiryTimeout(t *testing.T) {
	a, err := New(Options{Client: &fakeClient{block: true}, Timeout: 20 * time.Millisecond})
	if !assert.NoError(t, err) {
		return
	}
	start := time.Now()
	_, err = a.Inquiry(context.Background(), waterReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestInquiryNilContext(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: &pb.BillBaseResponse{Status: "failed"}}}
	var ctx context.Context // a nil context is the case under test
	resp, err := newAdapter(t, f).Inquiry(ctx, waterReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestDialEmptyAddressFailsEveryCall(t *testing.T) {
	c, err := Dial("")
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	a := newAdapter(t, c)
	_, err = a.Inquiry(context.Background(), waterReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorContains(t, err, "grpc_address")
}

func TestDialLazyAndUnreachable(t *testing.T) {
	c, err := Dial("127.0.0.1:1") // nothing listens: Dial itself must not fail
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	a, err := New(Options{Client: c, Timeout: 500 * time.Millisecond})
	if !assert.NoError(t, err) {
		return
	}
	_, err = a.Inquiry(context.Background(), waterReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
}

func TestAmountFromPayID(t *testing.T) {
	for payID, want := range map[string]uint64{"2500012345": 25000000, "100000": 1000, "000001": 0, "1234567": 12000} {
		got, ok := amountFromPayID(payID)
		assert.True(t, ok, payID)
		assert.Equal(t, want, got, payID)
	}
	for _, bad := range []string{"", "12345", "x234567", "-100000", "99999999999999999999999"} {
		_, ok := amountFromPayID(bad)
		assert.False(t, ok, bad)
	}
}

func TestMockFixtures(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Inquiry) {
		return
	}
	a, b := f.Inquiry(waterReq), f.Inquiry(waterReq)
	if !assert.NotNil(t, a) || !assert.Len(t, a.Inquiries, 1) {
		return
	}
	assert.NotSame(t, a, b, "a new value on each call")
	assert.Equal(t, provider.ResponseSuccess, a.Status)
	assert.Equal(t, waterReq.InquiryID, a.BillID)
	it := a.Inquiries[0]
	assert.Equal(t, "2500012345", it.PayID)
	assert.Equal(t, uint64(25000000), it.Amount)
	assert.Equal(t, "2030-01-01", it.PaymentDeadline)
	got, ok := amountFromPayID(it.PayID)
	assert.True(t, ok)
	assert.Equal(t, it.Amount, got, "the mock amount follows the pay ID rule")
}

// TestMockThroughProviderMock checks the mock with the real adapter: the backend is not called,
// with either return type.
func TestMockThroughProviderMock(t *testing.T) {
	f := &fakeClient{err: errors.New("must not be called")}
	m, err := provider.NewMock(newAdapter(t, f), provider.MockSettings{Enable: true, ReturnType: provider.MockReturnSuccess}, MockFixtures(), nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, m) {
		return
	}
	resp, err := m.Inquiry(context.Background(), waterReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Empty(t, f.reqs, "no call to the backend")
	m, err = provider.NewMock(newAdapter(t, f), provider.MockSettings{Enable: true, ReturnType: provider.MockReturnError}, MockFixtures(), nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, m) {
		return
	}
	_, err = m.Inquiry(context.Background(), waterReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.Empty(t, f.reqs)
}

// SupportsInquiry gives true only for the bill types that Inquiry answers (task 036); Inquiry of
// any other bill type is ErrNotSupported. A nil adapter answers the same.
func TestSupportsInquiry(t *testing.T) {
	want := map[string]bool{
		provider.BillTypeWater:       true,
		provider.BillTypeElectricity: false,
		provider.BillTypeGas:         false,
		provider.BillTypeIrancell:    false,
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
