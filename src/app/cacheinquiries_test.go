package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/common/monitoring/prometheus"
	"qabz/core/inquiry"
	"qabz/provider"
	"qabz/system/postgres"
)

// cacheTestSelector always picks one water inquiry provider.
type cacheTestSelector struct{}

func (cacheTestSelector) PickProvider(string, string) (provider.Provider, error) {
	return provider.Provider{ID: 36, Name: "stub", BillType: provider.BillTypeWater, Action: provider.ActionInquiry, Status: provider.StatusEnable}, nil
}

// cacheTestAdapter answers every inquiry and counts the calls.
type cacheTestAdapter struct {
	provider.ProviderInterface
	calls int
}

func (*cacheTestAdapter) Name() string                { return "stub" }
func (*cacheTestAdapter) SupportsInquiry(string) bool { return true }
func (a *cacheTestAdapter) Inquiry(context.Context, provider.InquiryRequest) (*provider.InquiryResponse, error) {
	a.calls++
	return &provider.InquiryResponse{Status: provider.ResponseSuccess, BillID: "1234567890125",
		Inquiries: []provider.InquiryItem{{PayID: "2500012345", Amount: 25000000, InquiryType: "regular"}}}, nil
}

type cacheTestAdapters struct{ a *cacheTestAdapter }

func (r cacheTestAdapters) Get(string) (provider.ProviderInterface, bool) { return r.a, true }

// TestNewInquiryFlowCacheInquiries (task 031): debug.cache_inquiries reaches the flow. The
// database of testConfig is not reachable (port 1): with the key false the inquiry does not use it
// and succeeds, and one warn line names the key; with the key true (the default) the flow uses the
// database, so the call fails at the request row, and no warn line is written.
func TestNewInquiryFlowCacheInquiries(t *testing.T) {
	for _, store := range []bool{false, true} {
		cfg := testConfig(t)
		assert.True(t, cfg.DebugCacheInquiries, "default")
		cfg.DebugCacheInquiries = store
		cfg.CachingWater = time.Hour
		db, err := postgres.New(context.Background(), cfg.PostgresDSN()) // lazy: no server needed
		if !assert.NoError(t, err) || !assert.NotNil(t, db) {
			t.FailNow()
		}
		log := &liveLogger{}
		adapter := &cacheTestAdapter{}
		flow, err := newInquiryFlow(cfg, db, cacheTestSelector{}, cacheTestAdapters{adapter}, prometheus.New("127.0.0.1:0", false), log)
		if !assert.NoError(t, err) || !assert.NotNil(t, flow) {
			db.Close()
			t.FailNow()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		res, err := flow.Inquire(ctx, inquiry.Request{BillType: provider.BillTypeWater, InquiryID: "1234567890125", UserID: liveUserOne})
		cancel()
		db.Close()
		warn := log.count("warn", "inquiry store disabled (debug.cache_inquiries: false): development only, every inquiry calls the provider and nothing is saved")
		if store {
			assert.ErrorContains(t, err, "save request row", "the flow uses the database")
			assert.Equal(t, 0, adapter.calls)
			assert.Equal(t, 0, warn)
		} else {
			if assert.NoError(t, err) && assert.Len(t, res.Inquiries, 1) {
				assert.False(t, res.FromDatabase)
			}
			assert.Equal(t, 1, adapter.calls)
			assert.Equal(t, 1, warn)
			assert.Equal(t, 0, log.count("error", "inquiry: read saved inquiries failed"), "no read was tried")
		}
	}
}
