package grpcserver

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/core/savedbill"
)

// Insert results for MyBillsDeps.OnInsert.
const (
	InsertCreated  = "created"  // a new row
	InsertExisting = "existing" // the user already had the bill; its id is returned
)

// errInvalidParameter is the answer to a request that breaks a rule. The rule is not named.
// @todo bill answers gRPC Unknown (HTTP 500) with a Persian message. See docs/this_vs_bill.md, item 5.
var errInvalidParameter = status.Error(codes.InvalidArgument, "invalid parameter")

// BillSaver saves a bill of a user (savedbill.Store).
type BillSaver interface {
	Insert(ctx context.Context, userID string, req savedbill.InsertRequest) (savedbill.Bill, bool, error)
}

// BillLister lists the saved bills of a user (savedbill.Store).
type BillLister interface {
	List(ctx context.Context, userID string) ([]savedbill.Bill, error)
}

// MyBillsDeps are the dependencies of MyBills. A zero value is valid: Insert and GetAll then
// answer codes.Unimplemented.
type MyBillsDeps struct {
	// Bills saves the bills of Insert. Nil: Insert is not available.
	Bills BillSaver
	// Lister reads the bills of GetAll. Nil: GetAll is not available.
	Lister BillLister
	// ServiceID is sent as serviceId by GetAll (config service.id).
	ServiceID int32
	// Logger gets one error line for each failed write or read. Nil: no log line.
	Logger ErrorLogger
	// OnInsert is called once per saved bill with InsertCreated or InsertExisting. Nil: no call.
	OnInsert func(result string)
}

// MyBills implements the gRPC service haf80.api.bill.MyBills (My Bills). Methods that are not
// written yet return codes.Unimplemented through the embedded type.
type MyBills struct {
	pb.UnimplementedMyBillsServer // embedded by value, as protoc-gen-go-grpc requires

	bills     BillSaver
	lister    BillLister
	serviceID int32
	logger    ErrorLogger
	onInsert  func(result string)
}

// NewMyBills returns the MyBills implementation.
func NewMyBills(deps MyBillsDeps) *MyBills {
	return &MyBills{
		bills:     deps.Bills,
		lister:    deps.Lister,
		serviceID: deps.ServiceID,
		logger:    deps.Logger,
		onInsert:  deps.OnInsert,
	}
}

// RegisterMyBills returns a Register that adds svc to a gRPC server. For a nil svc it returns nil,
// so New rejects it (a nil svc would make every call panic).
func RegisterMyBills(svc pb.MyBillsServer) Register {
	if svc == nil {
		return nil
	}
	if p, ok := svc.(*MyBills); ok && p == nil { // a typed nil pointer in the interface
		return nil
	}
	return func(r grpc.ServiceRegistrar) { pb.RegisterMyBillsServer(r, svc) }
}

// Insert saves a bill for the user of the JWT (POST /v1/mybill). A bill that the user already has
// is not saved again: the answer has the existing id. The answer echoes the request (the default
// name applied), as bill does. A request that breaks a rule gives codes.InvalidArgument and writes
// nothing; a failed write gives codes.Internal with no detail and one error log line with the cause.
func (s *MyBills) Insert(ctx context.Context, req *pb.InsertBillRequest) (*pb.BillResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if s.bills == nil {
		return nil, status.Error(codes.Unimplemented, "method Insert not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		// Only with debug.auth.disable and an empty debug.auth.mock_user_id: the call has no user.
		// @todo bill answers gRPC Unknown here. See docs/this_vs_bill.md, item 6.
		return nil, errRejected
	}
	if req == nil {
		return nil, errInvalidParameter
	}
	typ, ok := pb.InsertBillRequest_TypeEnum_name[int32(req.GetType())]
	if !ok {
		return nil, errInvalidParameter
	}
	b, created, err := s.bills.Insert(ctx, claims.UserID, savedbill.InsertRequest{
		Type:            typ,
		BillID:          req.GetBillId(),
		Name:            req.GetName(),
		Operator:        req.GetOperator(),
		Phone:           req.GetPhone(),
		ParticipateCode: req.GetParticipateCode(),
	})
	if errors.Is(err, savedbill.ErrInvalid) {
		// A client error: no log line.
		// @todo bill logs each invalid request at error level. See docs/this_vs_bill.md, item 7.
		return nil, errInvalidParameter
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, status.FromContextError(ctxErr).Err() // the client left: not a server error
		}
		if s.logger != nil {
			s.logger.Error("mybill: insert failed", "error", err)
		}
		return nil, errInternal
	}
	if s.onInsert != nil {
		result := InsertExisting
		if created {
			result = InsertCreated
		}
		s.onInsert(result)
	}
	return &pb.BillResponse{
		Id:       b.ID,
		Name:     b.Name,
		Type:     pb.BillResponse_TypeEnum(pb.BillResponse_TypeEnum_value[b.Type]),
		BillId:   req.GetBillId(),
		Operator: req.GetOperator(), // as sent: bill does not show the mci default here
		Phone:    req.GetPhone(),
	}, nil
}

// GetAll returns the saved bills of the user of the JWT (GET /v1/mybill), newest first, as bill
// does (controller/myBill GetAll):
//   - tel and mobile: billId and phone are the phone (data, or bill_id when data is empty); a later
//     row with a phone already in the list is left out (the phones of tel and mobile rows share
//     one set);
//   - gas: billId is bill_id and participateCode is data;
//   - other types: billId is bill_id. A type that is not in the proto enum is sent as gas (value 0).
//
// A user with no bills gets an empty list. A failed read gives codes.Internal with no detail and
// one error log line with the cause.
func (s *MyBills) GetAll(ctx context.Context, _ *emptypb.Empty) (*pb.BillsResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if s.lister == nil {
		return nil, status.Error(codes.Unimplemented, "method GetAll not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		// Only with debug.auth.disable and an empty debug.auth.mock_user_id: the call has no user.
		// @todo bill answers gRPC Unknown here. See docs/this_vs_bill.md, item 6.
		return nil, errRejected
	}
	bills, err := s.lister.List(ctx, claims.UserID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, status.FromContextError(ctxErr).Err() // the client left: not a server error
		}
		if s.logger != nil {
			s.logger.Error("mybill: list failed", "error", err)
		}
		// @todo bill answers gRPC Unknown with a Persian message. See docs/this_vs_bill.md, item 10.
		return nil, errInternal
	}
	return &pb.BillsResponse{ServiceId: s.serviceID, Data: billResponses(bills)}, nil
}

// billResponses maps saved bills to the GetAll answer with the rules of bill (see GetAll).
func billResponses(bills []savedbill.Bill) []*pb.BillResponse {
	out := make([]*pb.BillResponse, 0, len(bills))
	phones := make(map[string]bool)
	for _, b := range bills {
		r := &pb.BillResponse{
			Id:            b.ID,
			Name:          b.Name,
			Type:          pb.BillResponse_TypeEnum(pb.BillResponse_TypeEnum_value[b.Type]), // not in the enum: 0 (gas), as in bill
			LastOrderDate: b.LastOrderDate,
			BillId:        b.BillID,
			Operator:      b.Operator,
			IsPined:       b.IsPinned,
		}
		switch b.Type {
		case savedbill.TypeTel, savedbill.TypeMobile:
			phone := b.Data
			if phone == "" {
				phone = b.BillID
			}
			if phones[phone] {
				continue
			}
			phones[phone] = true
			r.BillId = phone
			r.Phone = phone
		case savedbill.TypeGas:
			r.ParticipateCode = b.Data
		}
		out = append(out, r)
	}
	return out
}
