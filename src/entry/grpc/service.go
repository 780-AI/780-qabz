package grpcserver

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
)

// GreetingText is the answer of Greeting.
const GreetingText = "Hello"

// Service implements the gRPC service haf80.api.bill.Service. Methods that are not written yet
// return codes.Unimplemented through the embedded type.
type Service struct {
	pb.UnimplementedServiceServer // embedded by value, as protoc-gen-go-grpc requires
}

// NewService returns the Service implementation.
func NewService() *Service { return &Service{} }

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
