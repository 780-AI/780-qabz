package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"qabz/config"
	"qabz/provider"
	"qabz/provider/jiring"
)

// Task 059: the Provider Switcher on the live stack (the seed providers in a run database, the
// composition-root wiring: providerAvailable, the provider metrics, the dev-only hook
// debug.provider_switcher.demote_on_down).

// liveSwitcher is newLiveMCI with Jiring Switch row 28 enabled (priority 10: after Parsian row 39,
// priority 2) and the demote hook on with period.
func liveSwitcher(t *testing.T, ctx context.Context, parsianReturn string, period time.Duration) *liveWaterStack {
	t.Helper()
	return newLiveWaterCfg(t, ctx, config.MockReturnSuccess, `UPDATE providers SET status = 'enable' WHERE id = 28;`, func(c *config.Config) {
		c.JiringSwitchEnabled = true
		c.DebugMockJiringSwitchEnabled = true
		c.DebugMockJiringSwitchReturnType = config.MockReturnSuccess
		c.ParsianEnabled = true
		c.DebugMockParsianEnabled = true
		c.DebugMockParsianReturnType = parsianReturn
		c.CachingMobile = time.Hour
		c.DebugProviderSwitcherDemoteOnDown = period
	})
}

func groupIDs(s *liveWaterStack, billType, action string) (ids []int64, demoted []int64) {
	for _, p := range s.providers.Group(billType, action) {
		ids = append(ids, p.ID)
		if p.Demoted {
			demoted = append(demoted, p.ID)
		}
	}
	return ids, demoted
}

// TestLiveSwitcherInquiry: Parsian (the first row of (mobile, inquiry)) is down: the request answers
// 503 and the hook demotes row 39; the next requests go to Jiring Switch, also after a load; after
// the period one request is the trial (Parsian, down again: demoted again); the request after it
// goes to Jiring Switch. One request calls one provider.
func TestLiveSwitcherInquiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	const period = 400 * time.Millisecond
	s := liveSwitcher(t, ctx, config.MockReturnError, period)
	assert.Equal(t, 1, s.log.count("warn", "provider switcher: debug.provider_switcher.demote_on_down is on: development only, each provider-down inquiry demotes its provider row"))

	_, err := s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	ids, dem := groupIDs(s, "mobile", "inquiry")
	assert.Equal(t, []int64{28, 39}, ids)
	assert.Equal(t, []int64{39}, dem)
	assert.Equal(t, 1, s.log.count("info", "providers: row demoted"))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_switches_total", map[string]string{"bill_type": "mobile", "action": "inquiry", "result": "demoted"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_demoted", map[string]string{"provider": "parsian", "bill_type": "mobile", "action": "inquiry"}))

	res, err := s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone2))
	if assert.NoError(t, err) {
		assert.Equal(t, uint32(jiring.MockFinalTermAmount), res.GetFullTerm().GetPrice(), "Jiring Switch answers")
	}
	assert.NoError(t, s.providers.Load(ctx), "a load during the demotion")
	_, dem = groupIDs(s, "mobile", "inquiry")
	assert.Equal(t, []int64{39}, dem, "kept through the load")
	_, err = s.askPhone(liveUserTwo, mciPhoneReq(liveMCIPhone2))
	assert.NoError(t, err)
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "parsian", "action": "inquiry", "result": "down"}), "Parsian called once")

	time.Sleep(period + 50*time.Millisecond)
	_, err = s.askPhone(liveUserTwo, mciPhoneReq(liveMCIPhone))
	assert.Equal(t, codes.Unavailable, status.Code(err), "the trial: Parsian is down")
	assert.Equal(t, 2.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "parsian", "action": "inquiry", "result": "down"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_trial_total", map[string]string{"bill_type": "mobile", "action": "inquiry", "result": "failed"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_switches_total", map[string]string{"bill_type": "mobile", "action": "inquiry", "result": "redemoted"}))
	assert.Equal(t, 1, s.log.count("info", "providers: trial failed, row demoted again"))
	_, err = s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone2))
	assert.NoError(t, err, "a new period: Jiring Switch")
	assert.Equal(t, 2.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "parsian", "action": "inquiry", "result": "down"}))
}

// TestLiveSwitcherOrderTrial: a demoted order row of Jiring Switch (30, mobile) returns by one
// inquiry trial through its inquiry row 28 (not the first row: Parsian 39 is); a demoted order row
// with no inquiry row of its bill type (29, mobilemidterm) returns with no trial when the period
// ends.
func TestLiveSwitcherOrderTrial(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := liveSwitcher(t, ctx, config.MockReturnSuccess, 0)
	assert.NoError(t, s.providers.Demote(30, 50*time.Millisecond))
	_, dem := groupIDs(s, "mobile", "order")
	assert.Equal(t, []int64{30}, dem)
	time.Sleep(80 * time.Millisecond)

	res, err := s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone))
	if assert.NoError(t, err) {
		assert.Equal(t, uint32(jiring.MockFinalTermAmount), res.GetFullTerm().GetPrice(), "the trial goes to Jiring Switch")
	}
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_trial_total", map[string]string{"bill_type": "mobile", "action": "order", "result": "ok"}))
	_, dem = groupIDs(s, "mobile", "order")
	assert.Empty(t, dem, "back in its place")
	assert.Equal(t, 0.0, s.metric(t, "qabz_provider_demoted", map[string]string{"provider": "jiring-switch", "bill_type": "mobile", "action": "order"}))
	res, err = s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone2))
	if assert.NoError(t, err) {
		assert.NotEqual(t, uint32(jiring.MockFinalTermAmount), res.GetFullTerm().GetPrice(), "no trial: Parsian, the first row")
	}

	assert.NoError(t, s.providers.Demote(29, 50*time.Millisecond))
	assert.Eventually(t, func() bool {
		_, d := groupIDs(s, "mobilemidterm", "order")
		return len(d) == 0
	}, 3*time.Second, 10*time.Millisecond, "returns with no trial")
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_switches_total", map[string]string{"bill_type": "mobilemidterm", "action": "order", "result": "restored"}))
	assert.Equal(t, 0.0, s.metric(t, "qabz_provider_trial_total", map[string]string{"bill_type": "mobilemidterm", "action": "order", "result": "ok"}))
}

// TestLiveSwitcherOneRow: the group of irancell has one row (38): a provider-down inquiry demotes
// nothing (no_next) and the next request goes to the same row.
func TestLiveSwitcherOneRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterCfg(t, ctx, config.MockReturnError, "", func(c *config.Config) {
		c.DebugProviderSwitcherDemoteOnDown = time.Hour
	})
	for range 2 {
		_, err := s.askPhone(liveUserOne, irancellPhoneReq("989351112233"))
		assert.Equal(t, codes.Unavailable, status.Code(err))
	}
	assert.Equal(t, 2.0, s.metric(t, "qabz_provider_switches_total", map[string]string{"bill_type": "irancell", "action": "inquiry", "result": "no_next"}))
	assert.Equal(t, 2.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "irancell", "action": "inquiry", "result": "down"}), "the same row both times")
	p, err := s.providers.PickProvider("irancell", provider.ActionInquiry)
	if assert.NoError(t, err) {
		assert.Equal(t, int64(38), p.ID)
		assert.False(t, p.Demoted)
	}
}
