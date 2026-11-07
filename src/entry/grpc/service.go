package grpcserver

import (
	"context"
	"errors"
	"math"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "qabz/api/pb/qabz"
	"qabz/core/billtype"
)

// GreetingText is the answer of Greeting.
const GreetingText = "Hello"

// langHeader is the metadata key of the HTTP Accept-Language header: grpc-gateway adds the
// "grpcgateway-" prefix. bill reads the same key and uses its raw value.
const langHeader = "grpcgateway-accept-language"

// errInternal is the answer to a failed read. The cause is not sent to the client.
var errInternal = status.Error(codes.Internal, "internal error")

// ServiceLister returns the service list of one language (billtype.ServiceCatalog).
type ServiceLister interface {
	List(ctx context.Context, lang string) ([]billtype.Service, error)
}

// ErrorLogger writes serious errors (the App logger has this method).
type ErrorLogger interface {
	Error(msg string, kv ...any)
}

// ServiceDeps are the dependencies of Service. A zero value is valid: Get then answers
// codes.Unimplemented.
type ServiceDeps struct {
	// Services gives Get the service list. Nil: Get is not available.
	Services ServiceLister
	// ServiceID is sent as serviceId by Get (config service.id).
	ServiceID int32
	// Logger gets one error line for each failed read. Nil: no log line.
	Logger ErrorLogger
}

// Service implements the gRPC service haf80.api.bill.Service. Methods that are not written yet
// return codes.Unimplemented through the embedded type.
type Service struct {
	pb.UnimplementedServiceServer // embedded by value, as protoc-gen-go-grpc requires

	services  ServiceLister
	serviceID int32
	logger    ErrorLogger
}

// NewService returns the Service implementation.
func NewService(deps ServiceDeps) *Service {
	return &Service{services: deps.Services, serviceID: deps.ServiceID, logger: deps.Logger}
}

// ServiceIDFromConfig converts the config value service.id to the int32 of the proto.
func ServiceIDFromConfig(id int) (int32, error) {
	if id < 0 || id > math.MaxInt32 {
		return 0, errors.New("service.id is out of the int32 range")
	}
	return int32(id), nil
}

// RegisterService returns a Register that adds svc to a gRPC server. For a nil svc it returns nil,
// so New rejects it (a nil svc would make every call panic).
func RegisterService(svc pb.ServiceServer) Register {
	if svc == nil {
		return nil
	}
	if p, ok := svc.(*Service); ok && p == nil { // a typed nil pointer in the interface
		return nil
	}
	return func(r grpc.ServiceRegistrar) { pb.RegisterServiceServer(r, svc) }
}

// Greeting pings the service. It returns "Hello". It can be removed later.
func (s *Service) Greeting(ctx context.Context, _ *pb.GreetingRequest) (*pb.GreetingResponse, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, status.FromContextError(err).Err()
		}
	}
	return &pb.GreetingResponse{Message: GreetingText}, nil
}

// Get returns the service list (the bill types) in the language of the Accept-Language header
// (bill: the raw header value; fa-IR when it is missing), in display order. The list comes from
// the services table through a cache (config.CacheServicesTTL). A failed read gives codes.Internal
// with no detail and one error log line with the cause.
func (s *Service) Get(ctx context.Context, _ *emptypb.Empty) (*pb.ServicesResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if s.services == nil {
		return nil, status.Error(codes.Unimplemented, "method Get not available")
	}
	list, err := s.services.List(ctx, requestLang(ctx))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, status.FromContextError(ctxErr).Err() // the client left: not a server error
		}
		// A failed read of the service list is a serious error: log the cause, send no detail.
		if s.logger != nil {
			s.logger.Error("services: list failed", "error", err)
		}
		return nil, errInternal
	}
	out := make([]*pb.ServicesResponse_Service, 0, len(list))
	for _, v := range list {
		out = append(out, &pb.ServicesResponse_Service{
			Name:     v.Name,
			IsActive: v.IsActive,
			Tag:      v.Tag,
			Type:     v.Type,
			Inquiry:  v.Inquiry, // @todo bill always sends true here; see billtype.Service.Inquiry
			Icon:     v.Icon,
		})
	}
	return &pb.ServicesResponse{ServiceId: s.serviceID, Services: out}, nil
}

// requestLang returns the first Accept-Language value that grpc-gateway forwarded, else
// billtype.DefaultLang.
func requestLang(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return billtype.DefaultLang
	}
	if v := md.Get(langHeader); len(v) > 0 && v[0] != "" {
		return v[0]
	}
	return billtype.DefaultLang
}
