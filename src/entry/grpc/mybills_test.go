package grpcserver

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/auth/authtest"
	"qabz/core/savedbill"
)

const billsUser = "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"

// fakeSaver records each call and answers with a copy of bill (ID set), created and err.
type fakeSaver struct {
	mu      sync.Mutex
	users   []string
	reqs    []savedbill.InsertRequest
	bill    savedbill.Bill
	created bool
	err     error
	hook    func() // runs inside Insert (for example to cancel the call)
}

func (f *fakeSaver) Insert(_ context.Context, userID string, req savedbill.InsertRequest) (savedbill.Bill, bool, error) {
	f.mu.Lock()
	f.users = append(f.users, userID)
	f.reqs = append(f.reqs, req)
	hook := f.hook
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	if f.err != nil {
		return savedbill.Bill{}, false, f.err
	}
	return f.bill, f.created, nil
}

func (f *fakeSaver) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func userCtx() context.Context {
	return auth.WithClaims(context.Background(), auth.Claims{UserID: billsUser})
}

// results records the OnInsert calls.
type results struct {
	mu  sync.Mutex
	got []string
}

func (r *results) add(s string) { r.mu.Lock(); r.got = append(r.got, s); r.mu.Unlock() }

func TestInsertMapsRequestAndAnswer(t *testing.T) {
	f := &fakeSaver{bill: savedbill.Bill{ID: "new-id", Name: "قبض همراه اول", Type: "mobile", Operator: "mci"}, created: true}
	r := &results{}
	svc := NewMyBills(MyBillsDeps{Bills: f, OnInsert: r.add})
	req := &pb.InsertBillRequest{BillId: "b", Name: "", Type: pb.InsertBillRequest_mobile, Operator: "",
		IsPined: true, Phone: "09121234567", ParticipateCode: "pc"}
	res, err := svc.Insert(userCtx(), req)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.Len(t, f.reqs, 1) {
		return
	}
	assert.Equal(t, []string{billsUser}, f.users, "the user comes from the JWT claims")
	assert.Equal(t, savedbill.InsertRequest{Type: "mobile", BillID: "b", Name: "", Operator: "",
		Phone: "09121234567", ParticipateCode: "pc"}, f.reqs[0])

	assert.Equal(t, "new-id", res.GetId())
	assert.Equal(t, "قبض همراه اول", res.GetName(), "the default name is in the answer")
	assert.Equal(t, pb.BillResponse_mobile, res.GetType())
	assert.Equal(t, "b", res.GetBillId())
	assert.Equal(t, "", res.GetOperator(), "the operator as sent, as in bill")
	assert.Equal(t, "09121234567", res.GetPhone(), "the phone as sent, as in bill")
	assert.False(t, res.GetIsPined(), "bill does not save or echo isPined")
	assert.Empty(t, res.GetLastOrderDate())
	assert.Empty(t, res.GetParticipateCode())
	assert.Equal(t, []string{InsertCreated}, r.got)
}

// TestInsertEveryType: each proto type reaches the store by its name and comes back as the same
// answer type.
func TestInsertEveryType(t *testing.T) {
	types := map[pb.InsertBillRequest_TypeEnum]pb.BillResponse_TypeEnum{
		pb.InsertBillRequest_gas:         pb.BillResponse_gas,
		pb.InsertBillRequest_water:       pb.BillResponse_water,
		pb.InsertBillRequest_tel:         pb.BillResponse_tel,
		pb.InsertBillRequest_mobile:      pb.BillResponse_mobile,
		pb.InsertBillRequest_electricity: pb.BillResponse_electricity,
	}
	for in, out := range types {
		f := &fakeSaver{bill: savedbill.Bill{ID: "id", Type: in.String()}, created: true}
		res, err := NewMyBills(MyBillsDeps{Bills: f}).Insert(userCtx(), &pb.InsertBillRequest{Type: in})
		if !assert.NoError(t, err, in.String()) || !assert.Len(t, f.reqs, 1) {
			continue
		}
		assert.Equal(t, in.String(), f.reqs[0].Type)
		assert.Equal(t, out, res.GetType())
	}
}

func TestInsertExistingResult(t *testing.T) {
	f := &fakeSaver{bill: savedbill.Bill{ID: "old-id", Type: "water"}, created: false}
	r := &results{}
	res, err := NewMyBills(MyBillsDeps{Bills: f, OnInsert: r.add}).Insert(userCtx(), &pb.InsertBillRequest{Type: pb.InsertBillRequest_water})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, "old-id", res.GetId())
	}
	assert.Equal(t, []string{InsertExisting}, r.got)
}

// TestInsertInvalid: a request that breaks a rule gives InvalidArgument with no detail, no log line
// and no result.
func TestInsertInvalid(t *testing.T) {
	cases := []struct {
		name  string
		req   *pb.InsertBillRequest
		err   error
		calls int
	}{
		{"store rule", &pb.InsertBillRequest{Type: pb.InsertBillRequest_water, BillId: "123456"}, errors.Join(savedbill.ErrInvalid, errors.New("bad bill id")), 1},
		{"type out of range", &pb.InsertBillRequest{Type: pb.InsertBillRequest_TypeEnum(9)}, nil, 0},
		{"negative type", &pb.InsertBillRequest{Type: pb.InsertBillRequest_TypeEnum(-1)}, nil, 0},
		{"nil request", nil, nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeSaver{err: c.err}
			log, r := &errLog{}, &results{}
			res, err := NewMyBills(MyBillsDeps{Bills: f, Logger: log, OnInsert: r.add}).Insert(userCtx(), c.req)
			assert.Nil(t, res)
			if assert.Error(t, err) {
				assert.Equal(t, codes.InvalidArgument, status.Code(err))
				assert.Equal(t, "invalid parameter", status.Convert(err).Message())
			}
			assert.Equal(t, c.calls, f.calls())
			assert.Empty(t, log.lines, "a client error writes no log line")
			assert.Empty(t, r.got)
		})
	}
}

func TestInsertWithoutUser(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"no claims":  context.Background(),
		"empty user": auth.WithClaims(context.Background(), auth.Claims{}),
	} {
		f := &fakeSaver{}
		res, err := NewMyBills(MyBillsDeps{Bills: f}).Insert(ctx, &pb.InsertBillRequest{Type: pb.InsertBillRequest_water})
		assert.Nil(t, res, name)
		if assert.Error(t, err, name) {
			assert.Equal(t, codes.PermissionDenied, status.Code(err), name)
		}
		assert.Equal(t, 0, f.calls(), name)
	}
}

func TestInsertErrorIsInternal(t *testing.T) {
	cause := errors.New(`relation "bills" does not exist`)
	log, r := &errLog{}, &results{}
	res, err := NewMyBills(MyBillsDeps{Bills: &fakeSaver{err: cause}, Logger: log, OnInsert: r.add}).
		Insert(userCtx(), &pb.InsertBillRequest{Type: pb.InsertBillRequest_water})
	assert.Nil(t, res)
	if assert.Error(t, err) {
		assert.Equal(t, codes.Internal, status.Code(err))
		assert.NotContains(t, err.Error(), "bills", "the cause is not sent to the client")
	}
	if assert.Len(t, log.lines, 1) && assert.Len(t, log.kvs, 1) {
		assert.Equal(t, "mybill: insert failed", log.lines[0])
		assert.Equal(t, []any{"error", cause}, log.kvs[0])
	}
	assert.Empty(t, r.got)

	// Without a logger the answer is the same.
	_, err = NewMyBills(MyBillsDeps{Bills: &fakeSaver{err: cause}}).Insert(userCtx(), &pb.InsertBillRequest{})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestInsertContext(t *testing.T) {
	f := &fakeSaver{bill: savedbill.Bill{ID: "id", Type: "water"}, created: true}
	ctx, cancel := context.WithCancel(userCtx())
	cancel()
	_, err := NewMyBills(MyBillsDeps{Bills: f}).Insert(ctx, &pb.InsertBillRequest{})
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Equal(t, 0, f.calls(), "a canceled call writes nothing")

	// A nil context has no claims.
	_, err = NewMyBills(MyBillsDeps{Bills: f}).Insert(nil, &pb.InsertBillRequest{}) //nolint:staticcheck // a nil context is the case under test
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// The write fails because the call was canceled meanwhile: Canceled, not Internal, and no log line.
	ctx, cancel = context.WithCancel(userCtx())
	log := &errLog{}
	f2 := &fakeSaver{err: context.Canceled, hook: cancel}
	_, err = NewMyBills(MyBillsDeps{Bills: f2, Logger: log}).Insert(ctx, &pb.InsertBillRequest{})
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Empty(t, log.lines)
}

func TestInsertWithoutStoreIsUnimplemented(t *testing.T) {
	res, err := NewMyBills(MyBillsDeps{}).Insert(userCtx(), &pb.InsertBillRequest{})
	assert.Nil(t, res)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestRegisterMyBillsRejectsNil(t *testing.T) {
	assert.Nil(t, RegisterMyBills(nil))
	var typedNil *MyBills
	assert.Nil(t, RegisterMyBills(typedNil))
	assert.NotNil(t, RegisterMyBills(NewMyBills(MyBillsDeps{})))
}

// TestInsertOverGRPCWithAuth: through a real gRPC server with the JWT check on, the user of the
// token reaches the store; a call with no token is rejected before the store.
func TestInsertOverGRPCWithAuth(t *testing.T) {
	a, err := NewAuth(AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	f := &fakeSaver{bill: savedbill.Bill{ID: "id", Type: "water"}, created: true}
	s, err := NewWithOptions("127.0.0.1:0", a.ServerOptions(), RegisterMyBills(NewMyBills(MyBillsDeps{Bills: f})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) || !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	conn, err := grpc.NewClient(s.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = conn.Close() })
	c := pb.NewMyBillsClient(conn)

	_, err = c.Insert(context.Background(), &pb.InsertBillRequest{Type: pb.InsertBillRequest_water})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, 0, f.calls())

	token := authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims())
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	res, err := c.Insert(ctx, &pb.InsertBillRequest{Type: pb.InsertBillRequest_water})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, "id", res.GetId())
	}
	f.mu.Lock()
	assert.Equal(t, []string{authtest.UserID}, f.users)
	f.mu.Unlock()
}

// fakeBillLister records each call and answers with bills and err.
type fakeBillLister struct {
	mu    sync.Mutex
	users []string
	bills []savedbill.Bill
	err   error
	hook  func() // runs inside List (for example to cancel the call)
}

func (f *fakeBillLister) List(_ context.Context, userID string) ([]savedbill.Bill, error) {
	f.mu.Lock()
	f.users = append(f.users, userID)
	hook := f.hook
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.bills, nil
}

func (f *fakeBillLister) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.users)
}

// TestGetAllMapsBills: the mapping of bill's controller/myBill GetAll, row by row, in the order of
// the repository.
func TestGetAllMapsBills(t *testing.T) {
	f := &fakeBillLister{bills: []savedbill.Bill{
		{ID: "m1", Name: "n1", Type: "mobile", BillID: "9121234567", Data: "989121234567", Operator: "mci", IsPinned: true, LastOrderDate: "d1"},
		{ID: "w1", Name: "n2", Type: "water", BillID: "123455", Data: "123455", LastOrderDate: "d2"},
		{ID: "m2", Name: "n3", Type: "mobile", Data: "989121234567"},   // same phone: left out
		{ID: "t1", Name: "n4", Type: "tel", BillID: "982112345678"},    // old row, empty data: phone is bill_id
		{ID: "m3", Name: "n5", Type: "mobile", BillID: "982112345678"}, // a tel phone again (one set): left out
		{ID: "g1", Name: "n6", Type: "gas", BillID: "999", Data: "1234567", Operator: "x"},
		{ID: "g2", Name: "n7", Type: "gas", BillID: "123455", Data: "123455"}, // gas saved by bill ID
		{ID: "e1", Name: "n8", Type: "electricity", BillID: "123455", Data: "123455"},
		{ID: "o1", Name: "n9", Type: "mobileIrancell", BillID: "555", Data: "666"}, // not in the enum: gas, bill_id
		{ID: "h1", Name: "n10", Type: "hafhashtadCredit", BillID: "777"},
		{ID: "x1", Name: "n11", Type: "tel"},    // no phone at all: "" is the phone
		{ID: "x2", Name: "n12", Type: "mobile"}, // "" again: left out
	}}
	res, err := NewMyBills(MyBillsDeps{Lister: f, ServiceID: 105}).GetAll(userCtx(), &emptypb.Empty{})
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, int32(105), res.GetServiceId())
	want := []*pb.BillResponse{
		{Id: "m1", Name: "n1", Type: pb.BillResponse_mobile, BillId: "989121234567", Phone: "989121234567", Operator: "mci", IsPined: true, LastOrderDate: "d1"},
		{Id: "w1", Name: "n2", Type: pb.BillResponse_water, BillId: "123455", LastOrderDate: "d2"},
		{Id: "t1", Name: "n4", Type: pb.BillResponse_tel, BillId: "982112345678", Phone: "982112345678"},
		{Id: "g1", Name: "n6", Type: pb.BillResponse_gas, BillId: "999", ParticipateCode: "1234567", Operator: "x"},
		{Id: "g2", Name: "n7", Type: pb.BillResponse_gas, BillId: "123455", ParticipateCode: "123455"},
		{Id: "e1", Name: "n8", Type: pb.BillResponse_electricity, BillId: "123455"},
		{Id: "o1", Name: "n9", Type: pb.BillResponse_gas, BillId: "555"},
		{Id: "h1", Name: "n10", Type: pb.BillResponse_hafhashtadCredit, BillId: "777"},
		{Id: "x1", Name: "n11", Type: pb.BillResponse_tel},
	}
	if assert.Len(t, res.GetData(), len(want)) {
		for i := range want {
			assert.True(t, proto.Equal(want[i], res.GetData()[i]), "row %d: want %v, got %v", i, want[i], res.GetData()[i])
		}
	}
	assert.Equal(t, []string{billsUser}, f.users)
}

func TestGetAllEmpty(t *testing.T) {
	res, err := NewMyBills(MyBillsDeps{Lister: &fakeBillLister{}}).GetAll(userCtx(), &emptypb.Empty{})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Empty(t, res.GetData())
	}
}

func TestGetAllWithoutUser(t *testing.T) {
	f := &fakeBillLister{}
	for name, ctx := range map[string]context.Context{
		"noClaims":  context.Background(),
		"emptyUser": auth.WithClaims(context.Background(), auth.Claims{}),
	} {
		res, err := NewMyBills(MyBillsDeps{Lister: f}).GetAll(ctx, &emptypb.Empty{})
		assert.Nil(t, res, name)
		assert.Equal(t, codes.PermissionDenied, status.Code(err), name)
	}
	assert.Equal(t, 0, f.calls())
}

func TestGetAllErrorIsInternal(t *testing.T) {
	cause := errors.New(`relation "bills" does not exist`)
	log := &errLog{}
	res, err := NewMyBills(MyBillsDeps{Lister: &fakeBillLister{err: cause}, Logger: log}).GetAll(userCtx(), &emptypb.Empty{})
	assert.Nil(t, res)
	if assert.Error(t, err) {
		assert.Equal(t, codes.Internal, status.Code(err))
		assert.NotContains(t, err.Error(), "bills", "the cause is not sent to the client")
	}
	if assert.Len(t, log.lines, 1) && assert.Len(t, log.kvs, 1) {
		assert.Equal(t, "mybill: list failed", log.lines[0])
		assert.Equal(t, []any{"error", cause}, log.kvs[0])
	}

	// Without a logger the answer is the same.
	_, err = NewMyBills(MyBillsDeps{Lister: &fakeBillLister{err: cause}}).GetAll(userCtx(), &emptypb.Empty{})
	assert.Equal(t, codes.Internal, status.Code(err))
}

// TestGetAllContext: a canceled call is a common error (conventions § 10): Canceled and no log line.
func TestGetAllContext(t *testing.T) {
	f := &fakeBillLister{}
	log := &errLog{}
	ctx, cancel := context.WithCancel(userCtx())
	cancel()
	_, err := NewMyBills(MyBillsDeps{Lister: f, Logger: log}).GetAll(ctx, &emptypb.Empty{})
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Equal(t, 0, f.calls(), "a canceled call reads nothing")

	// A nil context has no claims.
	_, err = NewMyBills(MyBillsDeps{Lister: f, Logger: log}).GetAll(nil, &emptypb.Empty{}) //nolint:staticcheck // a nil context is the case under test
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// The read fails because the call was canceled meanwhile: Canceled, not Internal.
	ctx, cancel = context.WithCancel(userCtx())
	f2 := &fakeBillLister{err: context.Canceled, hook: cancel}
	_, err = NewMyBills(MyBillsDeps{Lister: f2, Logger: log}).GetAll(ctx, &emptypb.Empty{})
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Empty(t, log.lines)
}

func TestGetAllWithoutListerIsUnimplemented(t *testing.T) {
	res, err := NewMyBills(MyBillsDeps{Bills: &fakeSaver{}}).GetAll(userCtx(), &emptypb.Empty{})
	assert.Nil(t, res)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

// TestGetAllOverGRPCWithAuth: through a real gRPC server with the JWT check on, the user of the
// token reaches the lister; a call with no token is rejected before it.
func TestGetAllOverGRPCWithAuth(t *testing.T) {
	a, err := NewAuth(AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	f := &fakeBillLister{bills: []savedbill.Bill{{ID: "id", Type: "water", BillID: "123455"}}}
	s, err := NewWithOptions("127.0.0.1:0", a.ServerOptions(), RegisterMyBills(NewMyBills(MyBillsDeps{Lister: f, ServiceID: 7})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) || !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	conn, err := grpc.NewClient(s.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = conn.Close() })
	c := pb.NewMyBillsClient(conn)

	_, err = c.GetAll(context.Background(), &emptypb.Empty{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, 0, f.calls())

	token := authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims())
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	res, err := c.GetAll(ctx, &emptypb.Empty{})
	if assert.NoError(t, err) && assert.NotNil(t, res) && assert.Len(t, res.GetData(), 1) {
		assert.Equal(t, int32(7), res.GetServiceId())
		assert.Equal(t, "id", res.GetData()[0].GetId())
	}
	f.mu.Lock()
	assert.Equal(t, []string{authtest.UserID}, f.users)
	f.mu.Unlock()
}
