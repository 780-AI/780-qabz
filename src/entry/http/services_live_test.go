package httpserver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/config"
	"qabz/core/billtype"
	grpcserver "qabz/entry/grpc"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
)

// TestLiveServicesEndToEnd is the integration test of GET /v1/services: mock rows in a mock
// services table (a throw-away schema of a real Postgres) go through the repository, the cache,
// the gRPC server and the gateway, and must come back exactly as inserted, in display order.
// It needs QABZ_TEST_RUN_DSN (make test).
func TestLiveServicesEndToEnd(t *testing.T) {
	dsn := postgrestest.RunDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgres.New(ctx, postgrestest.ServicesSchema(t, ctx, dsn))
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	defer db.Close()

	want := []billtype.Service{ // in "order" order
		{Name: "قبض آب", Type: "water", Tag: "جدید", IsActive: true, Inquiry: true, Icon: "https://cdn.example/water.svg"},
		{Name: "قبض برق", Type: "electricity", Tag: "", IsActive: true, Inquiry: false, Icon: "e.svg"},
		{Name: "قبض گاز", Type: "gas", Tag: "off", IsActive: false, Inquiry: true, Icon: ""},
	}
	_, err = db.Pool().Exec(ctx, `INSERT INTO services (id, name, type, tag, "order", is_active, lang, inquiry, icon) VALUES
		(10, $1, $2, $3, 1, $4, 'fa-IR', $5, $6),
		(11, $7, $8, $9, 2, $10, 'fa-IR', $11, $12),
		(12, $13, $14, $15, 3, $16, 'fa-IR', $17, $18),
		(13, 'Water', 'water', '', 1, true, 'en-US', true, 'w.svg')`,
		want[0].Name, want[0].Type, want[0].Tag, want[0].IsActive, want[0].Inquiry, want[0].Icon,
		want[1].Name, want[1].Type, want[1].Tag, want[1].IsActive, want[1].Inquiry, want[1].Icon,
		want[2].Name, want[2].Type, want[2].Tag, want[2].IsActive, want[2].Inquiry, want[2].Icon)
	if !assert.NoError(t, err) {
		return
	}

	var mu sync.Mutex
	var results []string
	catalog, err := billtype.NewServiceCatalog(postgres.NewServiceRepository(db), billtype.CatalogOptions{
		TTL: config.CacheServicesTTL, OnResult: func(r string) { mu.Lock(); results = append(results, r); mu.Unlock() }})
	if !assert.NoError(t, err) || !assert.NotNil(t, catalog) {
		t.FailNow()
	}
	gw := stackWith(t, grpcserver.NewService(grpcserver.ServiceDeps{Services: catalog, ServiceID: 105}))

	got := getServices(t, gw, "") // no Accept-Language: fa-IR, as in bill
	assert.Equal(t, int32(105), got.GetServiceId())
	assertServices(t, want, got.GetServices())

	en := getServices(t, gw, "en-US")
	assertServices(t, []billtype.Service{{Name: "Water", Type: "water", IsActive: true, Inquiry: true, Icon: "w.svg"}}, en.GetServices())

	// Inside the TTL the list comes from the cache: deleted rows are still served.
	_, err = db.Pool().Exec(ctx, `DELETE FROM services`)
	assert.NoError(t, err)
	assertServices(t, want, getServices(t, gw, "fa-IR").GetServices())
	mu.Lock()
	assert.Equal(t, []string{billtype.CacheMiss, billtype.CacheMiss, billtype.CacheHit}, results)
	mu.Unlock()
}
