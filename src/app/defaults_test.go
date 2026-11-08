package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth/authtest"
	"qabz/common/monitoring/prometheus"
	"qabz/config"
	grpcserver "qabz/entry/grpc"
	"qabz/system/postgres"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	c, err := config.Parse([]byte(`
mode: dev
database:
  postgres: {host: "127.0.0.1", port: 1, password: "s3cret"}
  redis: {host: "127.0.0.1", port: 1}
metrics:
  prometheus: {addr: "127.0.0.1:0"}
server:
  grpc: {addr: "127.0.0.1:0"}
  http: {addr: "127.0.0.1:0"}
debug:
  auth: {disable: true}
`))
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	return c
}

// defaultApp builds the App from the real dependencies. It stops the test if a step fails.
func defaultApp(t *testing.T) *App {
	t.Helper()
	b, err := NewDefaultBuilder(context.Background(), testConfig(t))
	if !assert.NoError(t, err) || !assert.NotNil(t, b) {
		t.FailNow()
	}
	return build(t, b)
}

func TestDefaultBuilderBuilds(t *testing.T) {
	a := defaultApp(t)
	defer a.Database().Close()
	defer a.Cache().Close()
}

func TestDefaultStartFailsWithoutServers(t *testing.T) {
	a := defaultApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := a.Start(ctx)
	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), "s3cret")
	}
}

func TestDefaultBuilderRejectsBadLogLevel(t *testing.T) {
	cfg := testConfig(t)
	cfg.LogLevel = "loud"
	_, err := NewDefaultBuilder(context.Background(), cfg)
	assert.Error(t, err)
}

// TestDefaultBuilderAuthRules: with the check on, the start needs the HMAC secret; with
// debug.auth.disable it does not. The secret is never in the start error.
func TestDefaultBuilderAuthRules(t *testing.T) {
	cases := []struct {
		name    string
		disable bool
		secret  config.Secret
		ok      bool
	}{
		{name: "disabled, no secret", disable: true, ok: true},
		{name: "on, secret", secret: authtest.Secret, ok: true},
		{name: "on, no secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.DebugAuthDisable = tc.disable
			cfg.JWTHMACSecret = tc.secret
			b, err := NewDefaultBuilder(context.Background(), cfg)
			if !tc.ok {
				if assert.Error(t, err) {
					assert.NotContains(t, err.Error(), authtest.Secret)
				}
				return
			}
			if assert.NoError(t, err) && assert.NotNil(t, b) {
				a := build(t, b)
				a.Database().Close()
				a.Cache().Close()
			}
		})
	}
}

func TestNewServiceRules(t *testing.T) {
	cfg := testConfig(t)
	db, err := postgres.New(context.Background(), cfg.PostgresDSN()) // lazy: no server needed
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	defer db.Close()
	metrics := prometheus.New("127.0.0.1:0", true)

	svc, err := newService(cfg, db, metrics, nil)
	assert.NoError(t, err)
	assert.NotNil(t, svc)

	_, err = newService(cfg, db, metrics, nil) // the metric is already registered
	assert.ErrorContains(t, err, "services: register metric")

	cfg.ServiceID = -1
	_, err = newService(cfg, db, prometheus.New("127.0.0.1:0", true), nil)
	assert.ErrorContains(t, err, "service.id")
}

// registered reports whether reg already has a collector with the metric name: registering a
// probe with the same name then fails (the probe has other labels and help, so the error is a
// descriptor conflict).
func registered(t *testing.T, reg *promclient.Registry, name string) bool {
	t.Helper()
	probe := promclient.NewCounter(promclient.CounterOpts{Name: name, Help: "probe"})
	if err := reg.Register(probe); err != nil {
		return true
	}
	reg.Unregister(probe)
	return false
}

// metricsServer returns the metrics endpoint of b. It stops the test if it is not a *prometheus.Server.
func metricsServer(t *testing.T, b *Builder) *prometheus.Server {
	t.Helper()
	m, ok := b.metrics.(*prometheus.Server)
	if !assert.True(t, ok) || !assert.NotNil(t, m) {
		t.FailNow()
	}
	return m
}

var qabzMetrics = []string{
	prometheus.RequestDurationName, "qabz_auth_rejected_total", "qabz_services_cache_total",
	"qabz_mybill_insert_total", "qabz_mybill_delete_total",
}

// TestDefaultBuilderMetricsSwitch: with metrics on, the composition root registers every qabz
// metric; with metrics.prometheus.enabled false it registers none and the endpoint is disabled.
func TestDefaultBuilderMetricsSwitch(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("enabled=%v", enabled), func(t *testing.T) {
			cfg := testConfig(t)
			cfg.MetricsEnabled = enabled
			cfg.DebugAuthDisable = false // the auth metric exists only with the check on
			cfg.JWTHMACSecret = authtest.Secret
			b, err := NewDefaultBuilder(context.Background(), cfg)
			if !assert.NoError(t, err) || !assert.NotNil(t, b) {
				return
			}
			a := build(t, b)
			defer a.Database().Close()
			defer a.Cache().Close()
			m := metricsServer(t, b)
			assert.Equal(t, enabled, m.Enabled())
			if !enabled {
				assert.Nil(t, m.Registry(), "nothing can be registered")
				return
			}
			if !assert.NotNil(t, m.Registry()) {
				return
			}
			assert.False(t, registered(t, m.Registry(), "qabz_no_such_metric"), "positive control: a free name")
			for _, name := range qabzMetrics {
				assert.True(t, registered(t, m.Registry(), name), name)
			}
		})
	}
}

// TestRequestMetricsWiring: with metrics on newRequestMetrics gives the interceptor options and
// registers the metrics once; with metrics off it gives no option.
func TestRequestMetricsWiring(t *testing.T) {
	on := prometheus.New("127.0.0.1:0", true)
	opts, err := newRequestMetrics(on)
	assert.NoError(t, err)
	assert.Len(t, opts, 2, "one unary and one stream interceptor")
	_, err = newRequestMetrics(on)
	assert.ErrorContains(t, err, "requests: register metric")

	opts, err = newRequestMetrics(prometheus.New("127.0.0.1:0", false))
	assert.NoError(t, err)
	assert.Empty(t, opts)
}

// TestMetricsOffRegistersNothing: with metrics off, newAuth and newService create no metric, so a
// second call cannot conflict (with metrics on the second newService fails, see TestNewServiceRules).
func TestMetricsOffRegistersNothing(t *testing.T) {
	cfg := testConfig(t)
	cfg.DebugAuthDisable = false
	cfg.JWTHMACSecret = authtest.Secret
	db, err := postgres.New(context.Background(), cfg.PostgresDSN()) // lazy: no server needed
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	defer db.Close()
	off := prometheus.New("127.0.0.1:0", false)
	for range 2 {
		svc, err := newService(cfg, db, off, nil)
		assert.NoError(t, err)
		assert.NotNil(t, svc)
		check, err := newAuth(cfg, off, nil)
		assert.NoError(t, err)
		assert.NotNil(t, check)
	}

	on := prometheus.New("127.0.0.1:0", true)
	_, err = newAuth(cfg, on, nil)
	assert.NoError(t, err)
	_, err = newAuth(cfg, on, nil)
	assert.ErrorContains(t, err, "auth: register metric")
}

// TestDefaultBuilderCountsRejectedCalls: in the composition root the request metrics come before
// the auth check, so a call without a token is counted (histogram sample count) with code PermissionDenied.
func TestDefaultBuilderCountsRejectedCalls(t *testing.T) {
	cfg := testConfig(t)
	cfg.DebugAuthDisable = false
	cfg.JWTHMACSecret = authtest.Secret
	b, err := NewDefaultBuilder(context.Background(), cfg)
	if !assert.NoError(t, err) || !assert.NotNil(t, b) {
		return
	}
	a := build(t, b)
	defer a.Database().Close()
	defer a.Cache().Close()
	gs, ok := b.grpc.(*grpcserver.Server)
	if !assert.True(t, ok) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		return
	}
	defer func() { assert.NoError(t, gs.Stop(context.Background())) }()
	conn, err := grpc.NewClient(gs.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = pb.NewServiceClient(conn).Greeting(ctx, &pb.GreetingRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	reg := metricsServer(t, b).Registry()
	if !assert.NotNil(t, reg) {
		return
	}
	families, err := reg.Gather()
	if !assert.NoError(t, err) {
		return
	}
	found := false
	for _, f := range families {
		if f == nil || f.GetName() != prometheus.RequestDurationName {
			continue
		}
		for _, m := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["path"] == pb.Service_Greeting_FullMethodName && labels["code"] == "PermissionDenied" {
				found = true
				assert.Equal(t, uint64(1), m.GetHistogram().GetSampleCount())
			}
		}
	}
	assert.True(t, found, "the rejected call is counted")
}

// TestNewMyBillsRules: with metrics on the insert and delete counters are registered once (a second
// call conflicts, also when only the delete counter is taken); with metrics off nothing is registered, so a second call works; a service.id out of
// the int32 range is rejected.
func TestNewMyBillsRules(t *testing.T) {
	cfg := testConfig(t)
	db, err := postgres.New(context.Background(), cfg.PostgresDSN()) // lazy: no server needed
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	defer db.Close()

	on := prometheus.New("127.0.0.1:0", true)
	svc, err := newMyBills(cfg, db, on, nil)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
	_, err = newMyBills(cfg, db, on, nil)
	assert.ErrorContains(t, err, "mybill: register metric")

	onlyDelete := prometheus.New("127.0.0.1:0", true)
	if assert.NotNil(t, onlyDelete.Registry()) {
		taken := promclient.NewCounter(promclient.CounterOpts{Name: "qabz_mybill_delete_total", Help: "taken"})
		assert.NoError(t, onlyDelete.Registry().Register(taken))
		svc, err = newMyBills(cfg, db, onlyDelete, nil)
		assert.ErrorContains(t, err, "mybill: register metric")
		assert.Nil(t, svc)
		assert.True(t, registered(t, onlyDelete.Registry(), "qabz_mybill_insert_total"), "the insert counter came first")
	}

	off := prometheus.New("127.0.0.1:0", false)
	for range 2 {
		svc, err := newMyBills(cfg, db, off, nil)
		assert.NoError(t, err)
		assert.NotNil(t, svc)
	}

	bad := cfg
	bad.ServiceID = -1
	svc, err = newMyBills(bad, db, off, nil)
	assert.ErrorContains(t, err, "service.id")
	assert.Nil(t, svc)
}

// TestDefaultBuilderServesMyBills: the composition root registers the MyBills service. With
// debug.auth.disable a call has no user, so Insert and GetAll answer PermissionDenied and Delete
// answers Unknown (as bill); an unregistered service would answer Unimplemented.
func TestDefaultBuilderServesMyBills(t *testing.T) {
	b, err := NewDefaultBuilder(context.Background(), testConfig(t))
	if !assert.NoError(t, err) || !assert.NotNil(t, b) {
		return
	}
	a := build(t, b)
	defer a.Database().Close()
	defer a.Cache().Close()
	gs, ok := b.grpc.(*grpcserver.Server)
	if !assert.True(t, ok) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		return
	}
	defer func() { assert.NoError(t, gs.Stop(context.Background())) }()
	conn, err := grpc.NewClient(gs.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := pb.NewMyBillsClient(conn)
	_, err = c.Insert(ctx, &pb.InsertBillRequest{Type: pb.InsertBillRequest_water, BillId: "123455"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = c.GetAll(ctx, &emptypb.Empty{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	res, err := c.Delete(ctx, &pb.DeleteBillRequest{Id: "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"})
	assert.Nil(t, res)
	assert.Equal(t, codes.Unknown, status.Code(err), "no user: bill's answer")
	_, err = c.Update(ctx, &pb.UpdateBillRequest{Id: "x"}) // positive control: a method that is not written yet
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

// TestDefaultBuilderMockUser: with debug.auth.disable and debug.auth.mock_user_id, Insert, GetAll
// and Delete get past the user check and reach the database. The test database does not listen
// (port 1), so the answer is Internal (Insert, GetAll), not PermissionDenied, and Unknown (Delete,
// as bill).
func TestDefaultBuilderMockUser(t *testing.T) {
	cfg := testConfig(t)
	cfg.DebugAuthMockUserID = "5a2c8173-cae5-4a6b-a109-af45f760824b"
	b, err := NewDefaultBuilder(context.Background(), cfg)
	if !assert.NoError(t, err) || !assert.NotNil(t, b) {
		return
	}
	a := build(t, b)
	defer a.Database().Close()
	defer a.Cache().Close()
	gs, ok := b.grpc.(*grpcserver.Server)
	if !assert.True(t, ok) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		return
	}
	defer func() { assert.NoError(t, gs.Stop(context.Background())) }()
	conn, err := grpc.NewClient(gs.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = pb.NewMyBillsClient(conn).Insert(ctx, &pb.InsertBillRequest{Type: pb.InsertBillRequest_water, BillId: "123455"})
	assert.Equal(t, codes.Internal, status.Code(err))
	_, err = pb.NewMyBillsClient(conn).GetAll(ctx, &emptypb.Empty{})
	assert.Equal(t, codes.Internal, status.Code(err))
	_, err = pb.NewMyBillsClient(conn).Delete(ctx, &pb.DeleteBillRequest{Id: "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"})
	assert.Equal(t, codes.Unknown, status.Code(err))
}
