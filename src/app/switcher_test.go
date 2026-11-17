package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/core/inquiry"
	"qabz/provider"
)

// supportAdapter inquires only the bill type inq.
type supportAdapter struct {
	provider.ProviderInterface
	inq string
}

func (a supportAdapter) SupportsInquiry(billType string) bool { return billType == a.inq }

type nameAdapters map[string]provider.ProviderInterface

func (n nameAdapters) Get(name string) (provider.ProviderInterface, bool) {
	a, ok := n[name]
	return a, ok
}

// TestProviderAvailable (task 059): an inquiry row needs an adapter that inquires its bill type; an
// order row an adapter and a payment route; another action or no adapter is not available.
func TestProviderAvailable(t *testing.T) {
	av := providerAvailable(nameAdapters{"parsian": supportAdapter{inq: "mobile"}, "nil": nil},
		map[string]map[string]bool{"parsian": {"water": true}})
	for _, tc := range []struct {
		p    provider.Provider
		want bool
	}{
		{provider.Provider{Name: "parsian", BillType: "mobile", Action: "inquiry"}, true},
		{provider.Provider{Name: "parsian", BillType: "water", Action: "inquiry"}, false},
		{provider.Provider{Name: "parsian", BillType: "water", Action: "order"}, true},
		{provider.Provider{Name: "parsian", BillType: "gas", Action: "order"}, false},
		{provider.Provider{Name: "parsian", BillType: "water", Action: "settle"}, false},
		{provider.Provider{Name: "eways2", BillType: "water", Action: "order"}, false},
		{provider.Provider{Name: "nil", BillType: "mobile", Action: "inquiry"}, false},
	} {
		assert.Equal(t, tc.want, av(tc.p), "%+v", tc.p)
	}
	// The payment routes of the App: the order rows of the seed that pay.
	routes := payRoutes()
	assert.True(t, routes["jiring-switch"][provider.BillTypeMobile])
	assert.True(t, routes["parsian"][provider.BillTypeWater])
	routes["parsian"][provider.BillTypeWater] = false
	assert.True(t, payRoutes()["parsian"][provider.BillTypeWater], "a new map for each call")
}

// demoteSpy records the Demote calls.
type demoteSpy struct {
	inquiry.Selector
	ids []int64
	per []time.Duration
}

func (d *demoteSpy) Demote(id int64, period time.Duration) error {
	d.ids, d.per = append(d.ids, id), append(d.per, period)
	return provider.ErrNoNextProvider
}

// TestDemoteOnDown (task 059): the hook demotes the row of the call for the period (an error of
// Demote is ignored: the switcher logs it); 0 or a selector with no Demote gives no hook; one warn
// line at start.
func TestDemoteOnDown(t *testing.T) {
	log := &fakeLogger{c: &calls{}}
	spy := &demoteSpy{}
	hook := demoteOnDown(30*time.Second, spy, log)
	if assert.NotNil(t, hook) {
		hook(provider.Provider{ID: 39})
		assert.Equal(t, []int64{39}, spy.ids)
		assert.Equal(t, []time.Duration{30 * time.Second}, spy.per)
	}
	assert.Nil(t, demoteOnDown(0, spy, log))
	assert.Nil(t, demoteOnDown(time.Second, cacheTestSelector{}, log), "no Demote method")
	assert.NotNil(t, demoteOnDown(time.Second, spy, nil), "no logger")
}

// TestNewProvidersSwitcherMetrics (task 059): qabz_provider_switches_total, qabz_provider_demoted
// and qabz_provider_trial_total count each change and each trial result.
func TestNewProvidersSwitcherMetrics(t *testing.T) {
	repo := &providerRepo{rows: []provider.Provider{
		{ID: 1, Name: "qabzino", BillType: "water", Action: "inquiry", Priority: 1, Status: provider.StatusEnable},
		{ID: 2, Name: "parsian", BillType: "water", Action: "inquiry", Priority: 2, Status: provider.StatusEnable},
		{ID: 3, Name: "karashop", BillType: "tel", Action: "inquiry", Priority: 1, Status: provider.StatusEnable},
	}}
	on := prometheus.New("127.0.0.1:0", true)
	s, err := newProviders(config.NewHolder(testConfig(t)), repo, on, &fakeLogger{c: &calls{}}, nil)
	if !assert.NoError(t, err) || !assert.NoError(t, s.Load(context.Background())) {
		return
	}
	value := func(name string, labels map[string]string) float64 {
		t.Helper()
		v, _ := metricValue(t, on.Registry(), name, labels)
		return v
	}
	water := func(result string) map[string]string {
		return map[string]string{"bill_type": "water", "action": "inquiry", "result": result}
	}
	gauge := map[string]string{"provider": "qabzino", "bill_type": "water", "action": "inquiry"}

	assert.ErrorIs(t, s.Demote(3, time.Minute), provider.ErrNoNextProvider)
	assert.Equal(t, 1.0, value("qabz_provider_switches_total", map[string]string{"bill_type": "tel", "action": "inquiry", "result": "no_next"}))

	assert.NoError(t, s.Demote(1, time.Millisecond))
	assert.Equal(t, 1.0, value("qabz_provider_switches_total", water("demoted")))
	assert.Equal(t, 1.0, value("qabz_provider_demoted", gauge))
	time.Sleep(5 * time.Millisecond)
	_, trial, _ := s.PickInquiry("water")
	trial.Done(provider.TrialReleased)
	_, trial, _ = s.PickInquiry("water")
	trial.Done(provider.TrialFailed)
	assert.Equal(t, 1.0, value("qabz_provider_switches_total", water("redemoted")))
	time.Sleep(5 * time.Millisecond)
	_, trial, _ = s.PickInquiry("water")
	trial.Done(provider.TrialOK)
	assert.Equal(t, 1.0, value("qabz_provider_trial_total", water("released")))
	assert.Equal(t, 1.0, value("qabz_provider_trial_total", water("failed")))
	assert.Equal(t, 1.0, value("qabz_provider_trial_total", water("ok")))
	assert.Equal(t, 1.0, value("qabz_provider_switches_total", water("restored")))
	assert.Equal(t, 0.0, value("qabz_provider_demoted", gauge))

	assert.NoError(t, s.Demote(1, time.Minute))
	assert.NoError(t, s.Disable(1))
	assert.Equal(t, 1.0, value("qabz_provider_switches_total", water("cleared")))
	assert.Equal(t, 0.0, value("qabz_provider_demoted", gauge))
}
