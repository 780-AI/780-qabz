package httpserver

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth/authtest"
	grpcserver "qabz/entry/grpc"
)

// authStack starts a gRPC server with the auth check on and a gateway in front of it.
func authStack(t *testing.T) *Gateway {
	t.Helper()
	a, err := grpcserver.NewAuth(grpcserver.AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	gs, err := grpcserver.NewWithOptions("127.0.0.1:0", a.ServerOptions(), grpcserver.RegisterService(grpcserver.NewService(grpcserver.ServiceDeps{})))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterServiceHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) || !assert.NoError(t, gw.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })
	return gw
}

// TestAuthOverHTTP runs every JWT input of the shared table through the gateway: a rejected call
// must be HTTP 403 (not 401, not 500), a valid one HTTP 200.
func TestAuthOverHTTP(t *testing.T) {
	gw := authStack(t)
	for _, tc := range authtest.Cases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+gw.Addr()+"/v1/greeting", nil)
			if !assert.NoError(t, err) || !assert.NotNil(t, req) {
				return
			}
			if !tc.NoHeader {
				req.Header.Set("Authorization", tc.Authorization)
			}
			res, err := http.DefaultClient.Do(req)
			if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.NotNil(t, res.Body) {
				return
			}
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			assert.NoError(t, err)
			if tc.Valid {
				assert.Equal(t, http.StatusOK, res.StatusCode, string(body))
				assert.Contains(t, string(body), `"message":"Hello"`)
				return
			}
			assert.Equal(t, http.StatusForbidden, res.StatusCode, string(body))
			assert.NotContains(t, string(body), tc.Reason+`"`, "the reason is not sent to the client")
		})
	}
}
