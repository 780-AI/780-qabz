package tavanir

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	pb "qabz/api/pb/tavanir"
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
	return a
}

var powerReq = provider.InquiryRequest{BillType: "electricity", UserID: "u1", Dialer: "989121234567", InquiryID: "1234567890125"}

func pbDetails() *pb.Details {
	return &pb.Details{Phase: "ph", CompanyName: "co", TariffType: "ta", CustomerType: 3, SerialNumber: "se",
		ServiceAddress: "ad", ServicePostCode: "po", CustomerName: "na", Region: "re"}
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
	assert.Equal(t, "tavanir", a.Name())
	_, err := a.Pay(context.Background(), provider.PaymentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Status(context.Background(), provider.StatusRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.GetDocument(context.Background(), provider.DocumentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported, "the bill document is a later task")
	_, err = a.Settle(context.Background(), provider.SettleRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
}

func TestInquiryRequestFields(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: &pb.BillBaseResponse{Status: "success", BillId: "b1"},
		Inquiries: []*pb.BillInquiryResponse_Inquiry{{PayId: "1800012345", Amount: 1, PaymentDeadline: "2030-01-01"}}}}
	_, err := newAdapter(t, f).Inquiry(context.Background(), powerReq)
	assert.NoError(t, err)
	if !assert.Len(t, f.reqs, 1) || !assert.NotNil(t, f.reqs[0]) {
		return
	}
	r := f.reqs[0]
	assert.Equal(t, "u1", r.GetUserId())
	assert.Equal(t, "989121234567", r.GetDialer())
	assert.Equal(t, "1234567890125", r.GetInquiryId())
	assert.Equal(t, "electricity", r.GetBillType())
	assert.True(t, f.dl, "each call has a deadline (providers.tavanir.timeout)")
}

func TestInquiryAnswerAndDetailsMapping(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{
		BaseResponse: &pb.BillBaseResponse{Code: 0, Status: "ignored", Message: "ignored", BillId: "b1"},
		Inquiries: []*pb.BillInquiryResponse_Inquiry{
			{PayId: "1800012345", Amount: 999, PaymentDeadline: "1405/01/02", InquiryType: "x"},
			nil,
			{PayId: "2", Amount: 18446744073709551615, PaymentDeadline: ""},
		},
		Details: pbDetails(),
	}}
	resp, err := newAdapter(t, f).Inquiry(context.Background(), powerReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) || !assert.Len(t, resp.Inquiries, 2, "a nil entry is skipped") {
		return
	}
	assert.Equal(t, provider.ResponseSuccess, resp.Status, "an answer with inquiries is a success")
	assert.Equal(t, int32(200), resp.Code)
	assert.Equal(t, successMessage, resp.Message)
	assert.Equal(t, "", resp.ProviderTraceID, "Tavanir has no trace ID")
	assert.Equal(t, "b1", resp.BillID)
	assert.Equal(t, provider.InquiryItem{PayID: "1800012345", Amount: 999, PaymentDeadline: "1405/01/02", InquiryType: "regular"}, resp.Inquiries[0],
		"the amount is the amount field (not read from the pay ID, as for Qabzino); the type is always regular")
	assert.Equal(t, uint64(18446744073709551615), resp.Inquiries[1].Amount)
	if assert.NotNil(t, resp.Details) {
		assert.Equal(t, &provider.ElectricityDetails{Phase: "ph", CompanyName: "co", TariffType: "ta", CustomerType: 3, SerialNumber: "se",
			ServiceAddress: "ad", ServicePostCode: "po", CustomerName: "na", City: "co", Region: "re"}, resp.Details.Electricity,
			"City is the company name, as in bill")
	}
}

func TestInquiryNoDetails(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: &pb.BillBaseResponse{BillId: "b1"},
		Inquiries: []*pb.BillInquiryResponse_Inquiry{{PayId: "p", Amount: 5}}}}
	resp, err := newAdapter(t, f).Inquiry(context.Background(), powerReq)
	if assert.NoError(t, err) && assert.NotNil(t, resp) {
		assert.Nil(t, resp.Details, "the backend sent no details")
	}
	// Details with no value are details (the zero values), not nil.
	f.resp.Details = &pb.Details{}
	resp, err = newAdapter(t, f).Inquiry(context.Background(), powerReq)
	if assert.NoError(t, err) && assert.NotNil(t, resp) && assert.NotNil(t, resp.Details) {
		assert.Equal(t, &provider.ElectricityDetails{}, resp.Details.Electricity)
	}
}

func TestInquiryEmptyPayIDKeepsAmount(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: &pb.BillBaseResponse{},
		Inquiries: []*pb.BillInquiryResponse_Inquiry{{PayId: "", Amount: 5000}}}}
	resp, err := newAdapter(t, f).Inquiry(context.Background(), powerReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) || !assert.Len(t, resp.Inquiries, 1) {
		return
	}
	assert.Equal(t, "", resp.Inquiries[0].PayID)
	assert.Equal(t, uint64(5000), resp.Inquiries[0].Amount, "as bill: the answer of Tavanir is passed on unchanged")
}

func TestInquiryWithoutInquiries(t *testing.T) {
	cases := []struct {
		name       string
		base       *pb.BillBaseResponse
		wantStatus string
		wantItems  int
	}{
		{"no debt", &pb.BillBaseResponse{Status: "success", Code: 200, Message: "no debt", BillId: "b"}, "success", 1},
		{"failed", &pb.BillBaseResponse{Status: "failed", Code: 4, Message: "bill not found", BillId: "b"}, "failed", 0},
		{"empty status", &pb.BillBaseResponse{Code: 4, Message: "?"}, "failed", 0},
		{"nil base", nil, "failed", 0},
	}
	for _, c := range cases {
		for _, inquiries := range [][]*pb.BillInquiryResponse_Inquiry{nil, {nil}} { // none, and only a nil entry
			f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: c.base, Inquiries: inquiries, Details: pbDetails()}}
			resp, err := newAdapter(t, f).Inquiry(context.Background(), powerReq)
			if !assert.NoError(t, err, c.name) || !assert.NotNil(t, resp, c.name) {
				continue
			}
			assert.Equal(t, c.wantStatus, resp.Status, c.name)
			assert.Len(t, resp.Inquiries, c.wantItems, c.name)
			assert.Equal(t, c.base.GetMessage(), resp.Message, c.name)
			assert.Equal(t, c.base.GetCode(), resp.Code, c.name)
			assert.Equal(t, "", resp.BillID, c.name+": bill drops the bill ID of an answer with no inquiry")
			assert.Nil(t, resp.Details, c.name+": bill drops the details of an answer with no inquiry")
			if c.wantItems == 1 && len(resp.Inquiries) == 1 { // the length is asserted above
				assert.Equal(t, provider.InquiryItem{InquiryType: "regular"}, resp.Inquiries[0], c.name+": amount 0, no pay ID")
			}
		}
	}
}

func TestInquiryErrors(t *testing.T) {
	a := newAdapter(t, &fakeClient{err: errors.New("connection refused")})
	resp, err := a.Inquiry(context.Background(), powerReq)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, provider.ErrProviderDown)

	a = newAdapter(t, &fakeClient{}) // nil, nil
	resp, err = a.Inquiry(context.Background(), powerReq)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, provider.ErrProviderDown, "a nil answer is a failed call")

	f := &fakeClient{}
	_, err = newAdapter(t, f).Inquiry(context.Background(), provider.InquiryRequest{BillType: "water", InquiryID: "x"})
	assert.ErrorIs(t, err, provider.ErrNotSupported, "only electricity is implemented")
	assert.Empty(t, f.reqs, "no call for another bill type")

	var nilAdapter *Adapter
	_, err = nilAdapter.Inquiry(context.Background(), powerReq)
	assert.Error(t, err)
	_, err = (&Adapter{}).Inquiry(context.Background(), powerReq)
	assert.Error(t, err)
}

func TestInquiryTimeout(t *testing.T) {
	a, err := New(Options{Client: &fakeClient{block: true}, Timeout: 20 * time.Millisecond})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		return
	}
	start := time.Now()
	_, err = a.Inquiry(context.Background(), powerReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestInquiryNilContext(t *testing.T) {
	f := &fakeClient{resp: &pb.BillInquiryResponse{BaseResponse: &pb.BillBaseResponse{Status: "failed"}}}
	var ctx context.Context // a nil context is the case under test
	resp, err := newAdapter(t, f).Inquiry(ctx, powerReq)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestDialEmptyAddressFailsEveryCall(t *testing.T) {
	c, err := Dial("")
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	_, err = newAdapter(t, c).Inquiry(context.Background(), powerReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorContains(t, err, "providers.tavanir.grpc_address")
}

func TestDialLazyAndUnreachable(t *testing.T) {
	c, err := Dial("127.0.0.1:1") // nothing listens: Dial itself must not fail
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	a, err := New(Options{Client: c, Timeout: 500 * time.Millisecond})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		return
	}
	_, err = a.Inquiry(context.Background(), powerReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
}

func TestMockFixtures(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Inquiry) {
		return
	}
	a, b := f.Inquiry(powerReq), f.Inquiry(powerReq)
	if !assert.NotNil(t, a) || !assert.NotNil(t, b) || !assert.Len(t, a.Inquiries, 1) || !assert.NotNil(t, a.Details) || !assert.NotNil(t, b.Details) {
		return
	}
	assert.NotSame(t, a, b, "a new value on each call")
	assert.NotSame(t, a.Details, b.Details)
	assert.NotSame(t, a.Details.Electricity, b.Details.Electricity)
	assert.Equal(t, provider.ResponseSuccess, a.Status)
	assert.Equal(t, powerReq.InquiryID, a.BillID)
	assert.Equal(t, "", a.ProviderTraceID, "as Tavanir")
	assert.Equal(t, provider.InquiryItem{PayID: "1800012345", Amount: 18000000, PaymentDeadline: "2030-01-01", InquiryType: "regular"}, a.Inquiries[0])
	d := a.Details.Electricity
	if !assert.NotNil(t, d) {
		return
	}
	for name, v := range map[string]string{"phase": d.Phase, "company": d.CompanyName, "tariff": d.TariffType, "serial": d.SerialNumber,
		"address": d.ServiceAddress, "post code": d.ServicePostCode, "customer": d.CustomerName, "city": d.City, "region": d.Region} {
		assert.NotEmpty(t, v, name+": the mock answer has all details")
	}
	assert.Equal(t, int32(1), d.CustomerType)
	assert.Equal(t, d.CompanyName, d.City, "as the real adapter")
	assert.Len(t, d.ServicePostCode, 10)
}

// TestMockThroughProviderMock checks the mock with the real adapter: the backend is not called,
// with either return type.
func TestMockThroughProviderMock(t *testing.T) {
	f := &fakeClient{err: errors.New("must not be called")}
	m, err := provider.NewMock(newAdapter(t, f), provider.MockSettings{Enable: true, ReturnType: provider.MockReturnSuccess}, MockFixtures(), nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, m) {
		return
	}
	resp, err := m.Inquiry(context.Background(), powerReq)
	assert.NoError(t, err)
	if assert.NotNil(t, resp) {
		assert.NotNil(t, resp.Details)
	}
	assert.Empty(t, f.reqs, "no call to the backend")
	m, err = provider.NewMock(newAdapter(t, f), provider.MockSettings{Enable: true, ReturnType: provider.MockReturnError}, MockFixtures(), nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, m) {
		return
	}
	_, err = m.Inquiry(context.Background(), powerReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.Empty(t, f.reqs)
	// Off: the adapter itself, so the backend is called.
	m, err = provider.NewMock(newAdapter(t, f), provider.MockSettings{}, MockFixtures(), nil)
	if assert.NoError(t, err) && assert.NotNil(t, m) {
		_, err = m.Inquiry(context.Background(), powerReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown)
		assert.Len(t, f.reqs, 1, "mock off: the real call")
	}
}
