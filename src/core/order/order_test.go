package order

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"qabz/core/fee"
	"qabz/core/inquiry"
	"qabz/provider"
)

const (
	testUser  = "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"
	testPhone = "989350000391"
)

type stubSelector struct {
	pro   provider.Provider
	err   error
	calls []string
}

func (s *stubSelector) PickProvider(billType, action string) (provider.Provider, error) {
	s.calls = append(s.calls, billType+"/"+action)
	return s.pro, s.err
}

type stubInquirer struct {
	res  inquiry.Result
	err  error
	reqs []inquiry.Request
}

func (s *stubInquirer) Inquire(_ context.Context, req inquiry.Request) (inquiry.Result, error) {
	s.reqs = append(s.reqs, req)
	return s.res, s.err
}

type stubRaw struct {
	res   inquiry.RawResult
	err   error
	calls []string
}

func (s *stubRaw) RawInquiry(endpointBillType, billID, payID string) (inquiry.RawResult, error) {
	s.calls = append(s.calls, endpointBillType+"/"+billID+"/"+payID)
	return s.res, s.err
}

type stubFees struct {
	res     fee.Result
	err     error
	amounts []int64
	users   []string
}

func (s *stubFees) Resolve(_ context.Context, userID string, _ int64, amount int64) (fee.Result, error) {
	s.users = append(s.users, userID)
	s.amounts = append(s.amounts, amount)
	return s.res, s.err
}

type stubRepo struct {
	inserted   []Order
	insertErr  error
	readIDs    []string
	readErr    error
	insertHook func()
}

func (s *stubRepo) Insert(_ context.Context, o Order) error {
	if s.insertHook != nil {
		s.insertHook()
	}
	if s.insertErr != nil {
		return s.insertErr
	}
	s.inserted = append(s.inserted, o)
	return nil
}

func (s *stubRepo) InquiryProvider(_ context.Context, id string) (string, string, error) {
	s.readIDs = append(s.readIDs, id)
	return "irancell", "irancell", s.readErr
}

type recLogger struct{ lines []string }

func (l *recLogger) Error(msg string, _ ...any) { l.lines = append(l.lines, msg) }

// fixture is a flow with stubs; each test changes the stub it needs.
type fixture struct {
	sel     *stubSelector
	inq     *stubInquirer
	raw     *stubRaw
	fees    *stubFees
	repo    *stubRepo
	log     *recLogger
	results []string
	svc     *Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	feeID := int64(900001)
	f := &fixture{
		sel:  &stubSelector{pro: provider.Provider{ID: 26, Name: "irancell", BillType: "irancell", Action: ActionOrder, Status: provider.StatusEnable}},
		inq:  &stubInquirer{},
		raw:  &stubRaw{err: inquiry.ErrInvalidBillID},
		fees: &stubFees{res: fee.Result{Fee: 9000, Type: fee.TypeFixed, Config: []byte(`{"amount":9000}`), ID: &feeID, Source: fee.SourceProvider}},
		repo: &stubRepo{},
		log:  &recLogger{},
	}
	svc, err := NewService(Options{Providers: f.sel, Inquiry: f.inq, Raw: f.raw, Fees: f.fees, Repository: f.repo, Logger: f.log,
		OnOrder: func(billType, providerName, result string) {
			f.results = append(f.results, billType+"/"+providerName+"/"+result)
		}})
	if !assert.NoError(t, err) || !assert.NotNil(t, svc) {
		t.FailNow()
	}
	n := 0
	svc.newID = func() string { n++; return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }
	f.svc = svc
	return f
}

func irancellReq() Request {
	return Request{BillType: provider.BillTypeIrancell, InquiryID: testPhone, UserID: testUser, Dialer: "989121234567", Option: "irancell", Price: 450000}
}

// TestCreateDirectPrice: rule a of bill (irancell, no pay ID, a price): the order row has the
// price of the request, the pay ID "irancell", no bill ID and no inquiry; the total adds the fee.
func TestCreateDirectPrice(t *testing.T) {
	f := newFixture(t)
	o, err := f.svc.Create(context.Background(), irancellReq())
	if !assert.NoError(t, err) {
		return
	}
	feeID := int64(900001)
	want := Order{
		ID: "00000000-0000-0000-0000-000000000001", OrderID: "00000000-0000-0000-0000-000000000002", UserID: testUser,
		BillType: "irancell", PayID: "irancell", Option: "irancell", Price: 459000, Data: testPhone, ProviderID: 26,
		ProviderName: "irancell", Status: StatusInitial, Fee: 9000, FeeType: fee.TypeFixed, FeeConfig: []byte(`{"amount":9000}`),
		FeeID: &feeID, BasePrice: 450000, FeeSource: fee.SourceProvider,
	}
	assert.Equal(t, want, o)
	assert.Equal(t, []Order{want}, f.repo.inserted)
	assert.Equal(t, []string{"irancell/order"}, f.sel.calls, "the order provider")
	assert.Empty(t, f.inq.reqs, "no inquiry")
	assert.Empty(t, f.raw.calls, "no raw inquiry")
	assert.Empty(t, f.repo.readIDs, "no inquiry row read")
	assert.Equal(t, []int64{450000}, f.fees.amounts)
	assert.Equal(t, []string{testUser}, f.fees.users)
	assert.Equal(t, []string{"irancell/irancell/created"}, f.results)
	assert.Empty(t, f.log.lines)
}

// TestCreateRawInquiry: rule b of bill (numeric bill ID and pay ID): the price, the bill ID and the
// pay ID of the raw inquiry. With a pay ID, rule a does not apply even with a price.
func TestCreateRawInquiry(t *testing.T) {
	f := newFixture(t)
	f.raw.err = nil
	f.raw.res = inquiry.RawResult{BillID: "1234567890125", PayID: "45000123", Amount: 450000, BillType: "water"}
	req := irancellReq()
	req.BillID, req.PayID = "1234567890125", "45000123"
	o, err := f.svc.Create(context.Background(), req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []string{"irancell/1234567890125/45000123"}, f.raw.calls)
	assert.Equal(t, []any{"1234567890125", "45000123", int64(450000), uint32(459000), ""}, []any{o.BillID, o.PayID, o.BasePrice, o.Price, o.BillInquiryID})
	assert.Empty(t, f.inq.reqs)
}

// TestCreateInquiry: rule c of bill: the inquiry of the bill type with no saved inquiry, the
// final-term inquiry (bill: BillTypeInquiryType[irancell]), and the read of its row.
func TestCreateInquiry(t *testing.T) {
	for _, tc := range []struct {
		name          string
		billID, payID string
		rawErr        error
		price         uint64
	}{
		{name: "no price"},
		{name: "a pay ID with a price", payID: "x", price: 450000},
		{name: "a non-numeric bill ID", billID: "abc", payID: "123"},
		{name: "a failed raw inquiry", billID: "1", payID: "2", rawErr: inquiry.ErrInvalidPayID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			if tc.rawErr != nil {
				f.raw.err = tc.rawErr
			}
			f.inq.res = inquiry.Result{Inquiries: []inquiry.Inquiry{
				{ID: "mid-id", BillID: "b1", PayID: "p1", Amount: 150000, InquiryType: provider.InquiryTypeMidterm},
				{ID: "final-id", BillID: "b2", PayID: "p2", Amount: 450000, InquiryType: provider.InquiryTypeFinalTerm},
				{ID: "final-2", BillID: "b3", PayID: "p3", Amount: 1, InquiryType: provider.InquiryTypeFinalTerm},
			}}
			req := irancellReq()
			req.BillID, req.PayID, req.Price = tc.billID, tc.payID, tc.price
			o, err := f.svc.Create(context.Background(), req)
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, []inquiry.Request{{BillType: "irancell", InquiryID: testPhone, UserID: testUser, Dialer: "989121234567", NoCache: true}}, f.inq.reqs)
			assert.Equal(t, []any{"final-id", "b2", "p2", int64(450000), uint32(459000)}, []any{o.BillInquiryID, o.BillID, o.PayID, o.BasePrice, o.Price})
			assert.Equal(t, []string{"final-id"}, f.repo.readIDs, "the row is read for the provider fee")
			assert.Equal(t, uint64(0), o.ProviderFee)
			if tc.billID == "1" {
				assert.Len(t, f.raw.calls, 1, "the raw inquiry was tried")
			}
		})
	}
}

// TestCreateInquiryNotSaved: with the inquiry store off (debug.cache_inquiries false) the answer has
// no rows: the order has no bill_inquiry_id and reads no row.
func TestCreateInquiryNotSaved(t *testing.T) {
	f := newFixture(t)
	f.inq.res = inquiry.Result{Unsaved: true, Inquiries: []inquiry.Inquiry{{ID: "final-id", Amount: 450000, InquiryType: provider.InquiryTypeFinalTerm}}}
	req := irancellReq()
	req.Price = 0
	o, err := f.svc.Create(context.Background(), req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "", o.BillInquiryID)
	assert.Empty(t, f.repo.readIDs)
	assert.Equal(t, int64(450000), o.BasePrice)
}

func TestCreateFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name       string
		change     func(f *fixture, r *Request)
		wantIs     error
		wantInqErr bool
		wantResult string
	}{
		{"no provider", func(f *fixture, _ *Request) { f.sel.err = fmt.Errorf("x: %w", provider.ErrNoProvider) }, provider.ErrNoProvider, false, "irancell/none/no_provider"},
		{"pick failed", func(f *fixture, _ *Request) { f.sel.err = boom }, boom, false, "irancell/none/error"},
		{"inquiry failed", func(f *fixture, r *Request) { r.Price = 0; f.inq.err = provider.ErrProviderDown }, provider.ErrProviderDown, true, "irancell/irancell/inquiry_failed"},
		{"no final-term inquiry", func(f *fixture, r *Request) {
			r.Price = 0
			f.inq.res = inquiry.Result{Inquiries: []inquiry.Inquiry{{ID: "m", Amount: 5, InquiryType: provider.InquiryTypeMidterm}}}
		}, ErrNoInquiry, false, "irancell/irancell/inquiry_failed"},
		{"zero price from the inquiry", func(f *fixture, r *Request) {
			r.Price = 0
			f.inq.res = inquiry.Result{Inquiries: []inquiry.Inquiry{{ID: "f", InquiryType: provider.InquiryTypeFinalTerm}}}
		}, ErrZeroPrice, false, "irancell/irancell/zero_price"},
		{"zero price from the raw inquiry", func(f *fixture, r *Request) {
			r.BillID, r.PayID = "1", "2"
			f.raw.err = nil
		}, ErrZeroPrice, false, "irancell/irancell/zero_price"},
		{"failed inquiry row read", func(f *fixture, r *Request) {
			r.Price = 0
			f.inq.res = inquiry.Result{Inquiries: []inquiry.Inquiry{{ID: "f", Amount: 10, InquiryType: provider.InquiryTypeFinalTerm}}}
			f.repo.readErr = boom
		}, boom, false, "irancell/irancell/error"},
		{"total above uint32", func(f *fixture, r *Request) { r.Price = math.MaxUint32 }, ErrTotalTooLarge, false, "irancell/irancell/invalid"},
		{"negative request price is a very large one", func(f *fixture, r *Request) {
			r.Price = uint64(math.MaxUint64) // bill: uint64(-1); int64 of it is -1
			f.fees.res = fee.Result{Fee: 0}
		}, ErrTotalTooLarge, false, "irancell/irancell/invalid"},
		{"failed insert", func(f *fixture, _ *Request) { f.repo.insertErr = boom }, boom, false, "irancell/irancell/error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			req := irancellReq()
			tc.change(f, &req)
			o, err := f.svc.Create(context.Background(), req)
			assert.Equal(t, Order{}, o)
			assert.ErrorIs(t, err, tc.wantIs)
			var inqErr *InquiryError
			assert.Equal(t, tc.wantInqErr, errors.As(err, &inqErr))
			assert.Equal(t, []string{tc.wantResult}, f.results)
			if tc.name != "failed insert" {
				assert.Empty(t, f.repo.inserted, "no row")
			}
		})
	}
}

// TestCreateTotalLimits: the total equal to math.MaxUint32 is saved; one more is invalid.
func TestCreateTotalLimits(t *testing.T) {
	f := newFixture(t)
	req := irancellReq()
	req.Price = math.MaxUint32 - 9000
	o, err := f.svc.Create(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, uint32(math.MaxUint32), o.Price)
	req.Price++
	_, err = f.svc.Create(context.Background(), req)
	assert.ErrorIs(t, err, ErrTotalTooLarge)
}

// TestCreateFeeFailuresGoOn: a failed fee does not stop the order (bill): the result of the
// resolver is saved and one error line is written.
func TestCreateFeeFailuresGoOn(t *testing.T) {
	id := int64(7)
	for _, tc := range []struct {
		name string
		res  fee.Result
	}{
		{"lookup error: the default fee", fee.Result{Fee: fee.DefaultFee, Source: fee.SourceLookupError}},
		{"calculation error: 0 with the row", fee.Result{Fee: 0, Type: fee.TypeFixed, ID: &id, Source: fee.SourceCalcError}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.fees.res, f.fees.err = tc.res, errors.New("fee failed")
			o, err := f.svc.Create(context.Background(), irancellReq())
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, []any{tc.res.Fee, tc.res.Type, tc.res.ID, uint32(450000 + tc.res.Fee)}, []any{o.Fee, o.FeeType, o.FeeID, o.Price})
			assert.Nil(t, o.FeeConfig)
			assert.Equal(t, []string{"irancell: order fee failed"}, f.log.lines)
			assert.Len(t, f.repo.inserted, 1)
		})
	}
}

// TestCreateDefaultFee: no fees row: the default fee with no type, no config and no ID.
func TestCreateDefaultFee(t *testing.T) {
	f := newFixture(t)
	f.fees.res = fee.Result{Fee: fee.DefaultFee, Source: fee.SourceDefault}
	o, err := f.svc.Create(context.Background(), irancellReq())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []any{int64(9000), "", []byte(nil), (*int64)(nil), uint32(459000)}, []any{o.Fee, o.FeeType, o.FeeConfig, o.FeeID, o.Price})
}

func TestCreateUnsupportedAndNotBuilt(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Create(context.Background(), Request{BillType: "water"})
	assert.ErrorIs(t, err, ErrUnsupported)
	assert.Empty(t, f.results, "an unknown bill type is not counted")
	assert.Empty(t, f.sel.calls)

	var none *Service
	_, err = none.Create(context.Background(), irancellReq())
	assert.Error(t, err)

	//nolint:staticcheck // a nil context is the case under test
	_, err = f.svc.Create(nil, irancellReq())
	assert.NoError(t, err)
}

func TestNewServiceRequiresEach(t *testing.T) {
	full := Options{Providers: &stubSelector{}, Inquiry: &stubInquirer{}, Raw: &stubRaw{}, Fees: &stubFees{}, Repository: &stubRepo{}}
	for name, change := range map[string]func(o *Options){
		"providers":  func(o *Options) { o.Providers = nil },
		"inquiry":    func(o *Options) { o.Inquiry = nil },
		"raw":        func(o *Options) { o.Raw = nil },
		"fees":       func(o *Options) { o.Fees = nil },
		"repository": func(o *Options) { o.Repository = nil },
	} {
		o := full
		change(&o)
		svc, err := NewService(o)
		assert.Error(t, err, name)
		assert.Nil(t, svc, name)
	}
	svc, err := NewService(full)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestNumericIsAtoi(t *testing.T) {
	for in, want := range map[string]bool{"123": true, "+12": true, "-12": true, "": false, "1a": false, " 1": false,
		"9223372036854775807": true, "9223372036854775808": false, "۱۲": false} {
		assert.Equal(t, want, numeric(in), in)
	}
}

func TestInquiryErrorText(t *testing.T) {
	var none *InquiryError
	assert.Equal(t, "order: inquiry failed", none.Error())
	assert.Nil(t, none.Unwrap())
	assert.Equal(t, "order: inquiry failed: boom", (&InquiryError{Err: errors.New("boom")}).Error())
}
