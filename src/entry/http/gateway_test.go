package httpserver

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	pb "qabz/api/pb/qabz"
	grpcserver "qabz/entry/grpc"
)

// stack starts a gRPC server and a gateway in front of it, both on free ports.
// It stops the test if a step fails.
func stack(t *testing.T) *Gateway {
	t.Helper()
	gs, err := grpcserver.New("127.0.0.1:0", grpcserver.RegisterService(grpcserver.NewService(grpcserver.ServiceDeps{})))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) {
		t.FailNow()
	}
	if !assert.NoError(t, gs.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterServiceHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) {
		t.FailNow()
	}
	if !assert.NoError(t, gw.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })
	return gw
}

// get sends a GET to the gateway and returns the status code and the body.
// It stops the test if the request fails.
func get(t *testing.T, gw *Gateway, path string) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+gw.Addr()+path, nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, req) {
		t.FailNow()
	}
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.NotNil(t, res.Body) {
		t.FailNow()
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	return res.StatusCode, body
}

func TestGreetingOverHTTP(t *testing.T) {
	code, body := get(t, stack(t), "/v1/greeting")
	assert.Equal(t, http.StatusOK, code)
	var got struct {
		Message string `json:"message"`
	}
	// Test code, not a hot path, so encoding/json/v2 is the right reader (conventions § 1).
	if assert.NoError(t, json.Unmarshal(body, &got), string(body)) {
		assert.Equal(t, "Hello", got.Message)
	}
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	code, _ := get(t, stack(t), "/v1/no-such-route")
	assert.Equal(t, http.StatusNotFound, code)
}

// A Service without a lister (ServiceDeps{}) answers Get with Unimplemented, which is HTTP 501.
func TestServicesWithoutListerIsNotImplemented(t *testing.T) {
	code, _ := get(t, stack(t), "/v1/services")
	assert.Equal(t, http.StatusNotImplemented, code)
}

func TestNewErrors(t *testing.T) {
	target := func() string { return "127.0.0.1:1" }
	_, err := New("", target, pb.RegisterServiceHandler)
	assert.ErrorContains(t, err, "address is required")
	_, err = New("127.0.0.1:0", nil, pb.RegisterServiceHandler)
	assert.ErrorContains(t, err, "target is required")
	_, err = New("127.0.0.1:0", target)
	assert.ErrorContains(t, err, "at least one service")
	_, err = New("127.0.0.1:0", target, pb.RegisterServiceHandler, nil)
	assert.ErrorContains(t, err, "register function 1 is nil")
}

func TestStartErrorsCloseWhatOpened(t *testing.T) {
	gw, err := New("127.0.0.1:0", func() string { return "" }, pb.RegisterServiceHandler)
	if assert.NoError(t, err) && assert.NotNil(t, gw) {
		assert.ErrorContains(t, gw.Start(context.Background()), "target is empty")
	}

	failing := func(context.Context, *runtime.ServeMux, *grpc.ClientConn) error { return errors.New("bad route") }
	gw, err = New("127.0.0.1:0", func() string { return "127.0.0.1:1" }, failing)
	if assert.NoError(t, err) && assert.NotNil(t, gw) {
		assert.ErrorContains(t, gw.Start(context.Background()), "bad route")
		assert.NoError(t, gw.Stop(context.Background()), "a failed Start leaves nothing to stop")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) || !assert.NotNil(t, ln) {
		return
	}
	defer ln.Close()
	gw, err = New(ln.Addr().String(), func() string { return "127.0.0.1:1" }, pb.RegisterServiceHandler)
	if assert.NoError(t, err) && assert.NotNil(t, gw) {
		assert.ErrorContains(t, gw.Start(context.Background()), "listen")
	}
}

func TestStartStopLifecycle(t *testing.T) {
	gw, err := New("127.0.0.1:0", func() string { return "127.0.0.1:1" }, pb.RegisterServiceHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) {
		return
	}
	assert.Equal(t, "127.0.0.1:0", gw.Addr(), "before Start: the configured address")
	if !assert.NoError(t, gw.Start(context.Background())) {
		return
	}
	assert.NotEqual(t, "127.0.0.1:0", gw.Addr(), "after Start: the bound port")
	assert.ErrorContains(t, gw.Start(context.Background()), "already started")
	assert.NoError(t, gw.Stop(context.Background()))
	assert.NoError(t, gw.Stop(context.Background()), "second Stop is a no-op")
	assert.NoError(t, gw.Stop(nil), "a nil context does not panic") //nolint:staticcheck
	assert.ErrorContains(t, gw.Start(context.Background()), "already stopped")
}

func TestZeroGatewayDoesNotPanic(t *testing.T) {
	var g Gateway
	assert.ErrorContains(t, g.Start(context.Background()), "not created with New")
	assert.NoError(t, g.Stop(context.Background()))
}
