package httpserver

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"

	pb "qabz/api/pb/qabz"
	"qabz/core/billtype"
	grpcserver "qabz/entry/grpc"
)

// langLister records the language of each call and returns fixed rows.
type langLister struct {
	mu    sync.Mutex
	rows  []billtype.Service
	langs []string
}

func (l *langLister) List(_ context.Context, lang string) ([]billtype.Service, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.langs = append(l.langs, lang)
	return l.rows, nil
}

// stackWith starts a gRPC server with svc and a gateway in front of it. It stops the test if a step fails.
func stackWith(t *testing.T, svc *grpcserver.Service) *Gateway {
	t.Helper()
	gs, err := grpcserver.New("127.0.0.1:0", grpcserver.RegisterService(svc))
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

// getServices sends GET /v1/services (with Accept-Language when lang is not empty) and decodes the
// body. It stops the test if the request fails or the answer is not HTTP 200.
func getServices(t *testing.T, gw *Gateway, lang string) *pb.ServicesResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+gw.Addr()+"/v1/services", nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, req) {
		t.FailNow()
	}
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
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
	if !assert.Equal(t, http.StatusOK, res.StatusCode, string(body)) {
		t.FailNow()
	}
	var out pb.ServicesResponse
	// Test code, not a hot path; protojson reads the gateway's proto JSON (conventions § 1).
	if !assert.NoError(t, protojson.Unmarshal(body, &out), string(body)) {
		t.FailNow()
	}
	return &out
}

func TestServicesOverHTTP(t *testing.T) {
	rows := []billtype.Service{
		{Name: "آب", Type: "water", Tag: "new", IsActive: true, Inquiry: true, Icon: "w.svg"},
		{Name: "Gas", Type: "gas", IsActive: false, Inquiry: false},
	}
	l := &langLister{rows: rows}
	gw := stackWith(t, grpcserver.NewService(grpcserver.ServiceDeps{Services: l, ServiceID: 105}))

	got := getServices(t, gw, "en-US")
	assert.Equal(t, int32(105), got.GetServiceId())
	assertServices(t, rows, got.GetServices())

	_ = getServices(t, gw, "") // no header: fa-IR
	l.mu.Lock()
	assert.Equal(t, []string{"en-US", billtype.DefaultLang}, l.langs, "Accept-Language reaches the handler")
	l.mu.Unlock()
}

// assertServices checks that got has exactly the rows of want, field by field, in the same order.
func assertServices(t *testing.T, want []billtype.Service, got []*pb.ServicesResponse_Service) {
	t.Helper()
	if !assert.Len(t, got, len(want)) {
		return
	}
	for i, w := range want {
		g := got[i]
		if !assert.NotNil(t, g) {
			continue
		}
		assert.Equal(t, w, billtype.Service{Name: g.GetName(), Type: g.GetType(), Tag: g.GetTag(),
			IsActive: g.GetIsActive(), Inquiry: g.GetInquiry(), Icon: g.GetIcon()}, "row %d", i)
	}
}
