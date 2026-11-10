package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/auth/authtest"
	"qabz/core/inquiry"
	"qabz/provider"
)

func powerDetails() *provider.Details {
	return &provider.Details{Electricity: &provider.ElectricityDetails{
		Phase: "ph", CompanyName: "co", TariffType: "ta", CustomerType: 3, SerialNumber: "se",
		ServiceAddress: "ad", ServicePostCode: "po", CustomerName: "na", City: "ci", Region: "re",
	}}
}

func TestElectricityInquiryMapsRequestAndAnswer(t *testing.T) {
	f := &fakeInquirer{res: inquiry.Result{BillID: "1234567890125", Details: powerDetails(), Inquiries: []inquiry.Inquiry{
		{PayID: "first", Amount: 1, PaymentDeadline: "2029-01-01"},
		{PayID: "1800012345", Amount: 18000000, PaymentDeadline: "2030-01-01"},
	}}}
	svc := NewElectricity(ElectricityDeps{Inquiry: f})
	res, err := svc.Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1234567890125", PayId: "ignored", ParticipateCode: "ignored"})
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.Len(t, f.reqs, 1) {
		return
	}
	assert.Equal(t, inquiry.Request{BillType: "electricity", InquiryID: "1234567890125", UserID: billsUser, Dialer: "989121234567"}, f.reqs[0],
		"payId and participateCode are ignored, as in bill")
	assert.Equal(t, "1234567890125", res.GetBillId())
	assert.Equal(t, "1800012345", res.GetPayId(), "the last inquiry is sent, as in bill")
	assert.Equal(t, uint32(18000000), res.GetPrice())
	assert.Equal(t, "2030-01-01", res.GetPaymentDeadline())
	assert.Equal(t, []string{"ph", "co", "ta", "3", "se", "ad", "po", "na"},
		[]string{res.GetPhase(), res.GetCompanyName(), res.GetTariffType(), res.GetCustomerType(), res.GetSerialNumber(), res.GetServiceAddress(), res.GetServicePostCode(), res.GetCustomerName()},
		"customerType is the number as text; city and region are not in the answer")
}

// TestElectricityInquiryDetailShapes: no details gives empty fields (also customerType); details
// with the zero values give customerType "0", as bill (fmt.Sprint of the number).
func TestElectricityInquiryDetailShapes(t *testing.T) {
	cases := map[string]struct {
		details      *provider.Details
		wantCustomer string
	}{
		"nil":             {nil, ""},
		"no bill type":    {&provider.Details{}, ""},
		"zero values":     {&provider.Details{Electricity: &provider.ElectricityDetails{}}, "0"},
		"negative number": {&provider.Details{Electricity: &provider.ElectricityDetails{CustomerType: -7}}, "-7"},
	}
	for name, c := range cases {
		f := &fakeInquirer{res: inquiry.Result{BillID: "b", Details: c.details, Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: 5}}}}
		res, err := NewElectricity(ElectricityDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "b"})
		if assert.NoError(t, err, name) && assert.NotNil(t, res, name) {
			assert.Equal(t, c.wantCustomer, res.GetCustomerType(), name)
			assert.Equal(t, "", res.GetPhase()+res.GetCompanyName()+res.GetCustomerName(), name)
			assert.Equal(t, uint32(5), res.GetPrice(), name)
		}
	}
}

func TestElectricityInquiryPaidBill(t *testing.T) {
	f := &fakeInquirer{res: inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: 0}}}}
	res, err := NewElectricity(ElectricityDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "x"})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(0), res.GetPrice())
		assert.Equal(t, "", res.GetPayId())
	}
}

func TestElectricityInquiryNotAvailableAndNoUser(t *testing.T) {
	res, err := NewElectricity(ElectricityDeps{}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{})
	assert.Nil(t, res)
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	f := &fakeInquirer{}
	svc := NewElectricity(ElectricityDeps{Inquiry: f})
	var nilCtx context.Context
	for name, ctx := range map[string]context.Context{
		"no claims":   context.Background(),
		"empty user":  auth.WithClaims(context.Background(), auth.Claims{}),
		"nil context": nilCtx,
	} {
		res, err := svc.Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1"})
		assert.Nil(t, res, name)
		assert.Equal(t, codes.PermissionDenied, status.Code(err), name)
	}
	assert.Equal(t, 0, f.calls())
}

func TestElectricityInquiryInvalidIsClientError(t *testing.T) {
	log := &errLog{}
	f := &fakeInquirer{err: fmt.Errorf("%w: bill id", inquiry.ErrInvalid)}
	svc := NewElectricity(ElectricityDeps{Inquiry: f, Logger: log})
	_, err := svc.Inquiry(waterCtx(), nil) // a nil request: an empty bill ID reaches the flow
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, "invalid parameter", status.Convert(err).Message())
	assert.Empty(t, log.lines, "a client error writes no log line (conventions § 10)")
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, "", f.reqs[0].InquiryID)
	}
}

func TestElectricityInquiryServerFaultsAreLogged(t *testing.T) {
	cause := errors.New("connection refused to 10.0.0.1:9000")
	cases := []struct {
		name     string
		err      error
		wantCode codes.Code
		wantText string
		wantLine string
	}{
		{"no provider", fmt.Errorf("pick provider: %w", provider.ErrNoProvider), codes.Unavailable, providerDownText, "electricity: no enabled provider for inquiry"},
		{"provider down", fmt.Errorf("tavanir: %w: %w", provider.ErrProviderDown, cause), codes.Unavailable, providerDownText, "electricity: provider inquiry failed"},
		{"no adapter", fmt.Errorf("%w %q", inquiry.ErrNoAdapter, "parsian"), codes.Unavailable, providerDownText, "electricity: provider inquiry failed"},
		{"database", fmt.Errorf("save request row: %w", cause), codes.Internal, "internal error", "electricity: inquiry failed"},
	}
	for _, c := range cases {
		log := &errLog{}
		svc := NewElectricity(ElectricityDeps{Inquiry: &fakeInquirer{err: c.err}, Logger: log})
		res, err := svc.Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1234567890125"})
		assert.Nil(t, res, c.name)
		assert.Equal(t, c.wantCode, status.Code(err), c.name)
		assert.Equal(t, c.wantText, status.Convert(err).Message(), c.name)
		if assert.Error(t, err, c.name) {
			assert.NotContains(t, err.Error(), "10.0.0.1", c.name+": the cause is not sent to the client")
		}
		if assert.Len(t, log.lines, 1, c.name+": one error line") && assert.Len(t, log.kvs, 1, c.name) {
			assert.Equal(t, c.wantLine, log.lines[0], c.name)
			assert.Equal(t, []any{"error", c.err}, log.kvs[0], c.name+": the log has the cause")
		}
	}
	// No logger: no panic.
	_, err := NewElectricity(ElectricityDeps{Inquiry: &fakeInquirer{err: cause}}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestElectricityInquiryProviderRefusedAndClientLeaves(t *testing.T) {
	log := &errLog{}
	f := &fakeInquirer{err: fmt.Errorf("x: %w", &inquiry.RefusedError{Message: "bill not found"})}
	_, err := NewElectricity(ElectricityDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, "bill not found", status.Convert(err).Message())
	f.err = &inquiry.RefusedError{}
	_, err = NewElectricity(ElectricityDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1"})
	assert.Equal(t, "inquiry failed", status.Convert(err).Message(), "a default text when the provider sent none")

	ctx, cancel := context.WithCancel(waterCtx())
	f2 := &fakeInquirer{err: errors.Join(provider.ErrProviderDown, context.Canceled), hook: cancel}
	_, err = NewElectricity(ElectricityDeps{Inquiry: f2, Logger: log}).Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1"})
	assert.Equal(t, codes.Canceled, status.Code(err))
	// A context that is canceled before the call: the flow is not called.
	f3 := &fakeInquirer{}
	_, err = NewElectricity(ElectricityDeps{Inquiry: f3}).Inquiry(ctx, &pb.InquiryBillPayRequest{})
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Equal(t, 0, f3.calls())
	assert.Empty(t, log.lines, "a refused inquiry and a client that left are not server faults")
}

func TestElectricityInquiryAmountAboveUint32(t *testing.T) {
	log := &errLog{}
	f := &fakeInquirer{res: inquiry.Result{Details: powerDetails(), Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: math.MaxUint32 + 1}}}}
	res, err := NewElectricity(ElectricityDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1"})
	assert.Nil(t, res)
	assert.Equal(t, codes.Internal, status.Code(err))
	if assert.Len(t, log.lines, 1) {
		assert.Equal(t, "electricity: inquiry amount does not fit the price field", log.lines[0])
	}
	f.res = inquiry.Result{Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: math.MaxUint32}}}
	res, err = NewElectricity(ElectricityDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1"})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(math.MaxUint32), res.GetPrice(), "the largest value that fits")
	}
	// No logger: no panic.
	f.res = inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: math.MaxUint32 + 1}}}
	_, err = NewElectricity(ElectricityDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1"})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestRegisterElectricityRejectsNil(t *testing.T) {
	assert.Nil(t, RegisterElectricity(nil))
	var typedNil *Electricity
	assert.Nil(t, RegisterElectricity(typedNil))
	assert.NotNil(t, RegisterElectricity(NewElectricity(ElectricityDeps{})))
}

// TestAnswerInquiryErrorNilContext: the shared error mapping does not need a context.
func TestAnswerInquiryErrorNilContext(t *testing.T) {
	var nilCtx context.Context
	log := &errLog{}
	err := answerInquiryError(nilCtx, log, "gas", provider.ErrProviderDown)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	if assert.Len(t, log.lines, 1) {
		assert.Equal(t, "gas: provider inquiry failed", log.lines[0], "the bill type starts the line")
	}
}

// TestElectricityInquiryOverGRPCWithAuth: through a real gRPC server with the JWT check on, the user
// of the token reaches the flow and the details reach the client; a call with no token is rejected
// before the flow; InquiryDocument is not written yet.
func TestElectricityInquiryOverGRPCWithAuth(t *testing.T) {
	a, err := NewAuth(AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	f := &fakeInquirer{res: inquiry.Result{BillID: "b", Details: powerDetails(), Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: 5, PaymentDeadline: "2030-01-01"}}}}
	s, err := NewWithOptions("127.0.0.1:0", a.ServerOptions(), RegisterElectricity(NewElectricity(ElectricityDeps{Inquiry: f})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) || !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	conn, err := grpc.NewClient(s.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = conn.Close() })
	c := pb.NewElectricityClient(conn)

	_, err = c.Inquiry(context.Background(), &pb.InquiryBillPayRequest{BillId: "1234567890125"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, 0, f.calls())

	token := authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims())
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	res, err := c.Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1234567890125"})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(5), res.GetPrice())
		assert.Equal(t, "na", res.GetCustomerName())
		assert.Equal(t, "3", res.GetCustomerType())
	}
	_, err = c.InquiryDocument(ctx, &pb.InquiryBillPayRequest{BillId: "1234567890125"})
	assert.Equal(t, codes.Unimplemented, status.Code(err), "the bill document is not in task 022")
	f.mu.Lock()
	defer f.mu.Unlock()
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, authtest.UserID, f.reqs[0].UserID)
		assert.Equal(t, "electricity", f.reqs[0].BillType)
	}
}
