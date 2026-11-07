package httpserver

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "qabz/api/pb/qabz"
	grpcserver "qabz/entry/grpc"
)

// TestRequestsOverHTTP: a gateway call is measured by the gRPC interceptor with the gRPC method as
// the path. An unknown HTTP route ends in the gateway (404) and is not measured, so the path label
// cannot grow with the raw URL.
func TestRequestsOverHTTP(t *testing.T) {
	var (
		mu    sync.Mutex
		paths []string
	)
	r, err := grpcserver.NewRequests(func(path, code string, _ time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, path+" "+code)
	})
	if !assert.NoError(t, err) || !assert.NotNil(t, r) {
		return
	}
	gs, err := grpcserver.NewWithOptions("127.0.0.1:0", r.ServerOptions(), grpcserver.RegisterService(grpcserver.NewService(grpcserver.ServiceDeps{})))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		return
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterServiceHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) || !assert.NoError(t, gw.Start(context.Background())) {
		return
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })

	get := func(path string) int {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+gw.Addr()+path, nil)
		if !assert.NoError(t, err) || !assert.NotNil(t, req) {
			return 0
		}
		res, err := http.DefaultClient.Do(req)
		if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.NotNil(t, res.Body) {
			return 0
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		return res.StatusCode
	}
	for range 3 {
		assert.Equal(t, http.StatusOK, get("/v1/greeting"))
	}
	assert.Equal(t, http.StatusNotFound, get("/v1/no-such-route/123"))

	mu.Lock()
	defer mu.Unlock()
	greeting := pb.Service_Greeting_FullMethodName + " OK"
	assert.Equal(t, []string{greeting, greeting, greeting}, paths)
}
