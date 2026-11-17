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
// composition-root wiring: providerAvailable, the provider metrics). Task 060: the circuit breaker
// starts the demotion (it replaced the dev-only hook debug.provider_switcher.demote_on_down).

// liveBreaker turns the circuit breaker of c on: the first provider-down call opens the row
// (min_calls 1), and the row stays demoted for openTime.
func liveBreaker(c *config.Config, openTime time.Duration) {
	c.CircuitBreakerEnabled = true
	c.CircuitBreakerMinCalls = 1
	c.CircuitBreakerFailureRatio = 30
	c.CircuitBreakerWindow = time.Minute
	c.CircuitBreakerOpenTime = openTime
}

// liveSwitcher is newLiveMCI with Jiring Switch row 28 enabled (priority 10: after Parsian row 39,
// priority 2) and the circuit breaker on with the open time period (0: the breaker is off).
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
		if period > 0 {
			liveBreaker(c, period)
		}
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

// waitDemoted waits until the circuit breaker (its own goroutine) has demoted the rows want of the
// group.
func waitDemoted(t *testing.T, s *liveWaterStack, billType, action string, want []int64) {
	t.Helper()
	assert.Eventually(t, func() bool {
		_, d := groupIDs(s, billType, action)
		return assert.ObjectsAreEqual(want, d)
	}, 3*time.Second, 5*time.Millisecond, "demoted %v", want)
}

// TestLiveSwitcherInquiry: Parsian (the first row of (mobile, inquiry)) is down: the request answers
// 503 and the circuit breaker demotes row 39 (task 060); the next requests go to Jiring Switch, also
// after a load; after the period one request is the trial (Parsian, down again: demoted again); the request after it
// goes to Jiring Switch. One request calls one provider.
func TestLiveSwitcherInquiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	const period = 400 * time.Millisecond
	s := liveSwitcher(t, ctx, config.MockReturnError, period)

	_, err := s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	waitDemoted(t, s, "mobile", "inquiry", []int64{39})
	ids, _ := groupIDs(s, "mobile", "inquiry")
	assert.Equal(t, []int64{28, 39}, ids)
	// The Demote runs in the goroutine of the breaker: the line comes after the new order is stored.
	assert.Eventually(t, func() bool { return s.log.count("info", "providers: row demoted") == 1 }, 3*time.Second, 5*time.Millisecond)
	assert.Equal(t, 1, s.log.count("info", "circuit breaker: open, row demoted"))
	breakerLabels := map[string]string{"provider": "parsian", "bill_type": "mobile", "action": "inquiry"}
	assert.Equal(t, 1.0, s.metric(t, "qabz_circuit_breaker_state", breakerLabels))
	assert.Equal(t, 1.0, s.metric(t, "qabz_circuit_breaker_transitions_total", map[string]string{"provider": "parsian", "bill_type": "mobile", "action": "inquiry", "to": "open"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_switches_total", map[string]string{"bill_type": "mobile", "action": "inquiry", "result": "demoted"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_demoted", map[string]string{"provider": "parsian", "bill_type": "mobile", "action": "inquiry"}))

	res, err := s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone2))
	if assert.NoError(t, err) {
		assert.Equal(t, uint32(jiring.MockFinalTermAmount), res.GetFullTerm().GetPrice(), "Jiring Switch answers")
	}
	assert.NoError(t, s.providers.Load(ctx), "a load during the demotion")
	_, dem := groupIDs(s, "mobile", "inquiry")
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
	assert.Equal(t, 1.0, s.metric(t, "qabz_circuit_breaker_state", breakerLabels), "a failed trial: still open")
	_, err = s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone2))
	assert.NoError(t, err, "a new period: Jiring Switch")
	assert.Equal(t, 2.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "parsian", "action": "inquiry", "result": "down"}))
}

// TestLiveBreakerReturn (task 060, feature ON): a good answer opens nothing; a demotion of row 39
// (the call of the breaker at its threshold; TestLiveSwitcherInquiry opens it by real down calls)
// opens the breaker; after the open time the trial is good (the Parsian mock answers), the breaker
// closes and the row is back in its place.
func TestLiveBreakerReturn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	const period = 300 * time.Millisecond
	s := liveSwitcher(t, ctx, config.MockReturnSuccess, period)
	breakerLabels := map[string]string{"provider": "parsian", "bill_type": "mobile", "action": "inquiry"}

	// A good answer is a good call: nothing opens.
	_, err := s.askPhone(liveUserOne, mciPhoneReq(liveMCIPhone))
	assert.NoError(t, err)
	assert.NoError(t, s.providers.Demote(39, period), "the open of the breaker (as after a provider-down call)")
	waitDemoted(t, s, "mobile", "inquiry", []int64{39})
	assert.Equal(t, 1.0, s.metric(t, "qabz_circuit_breaker_state", breakerLabels))
	time.Sleep(period + 50*time.Millisecond)
	_, err = s.askPhone(liveUserTwo, mciPhoneReq(liveMCIPhone2))
	assert.NoError(t, err, "the trial: Parsian answers")
	waitDemoted(t, s, "mobile", "inquiry", nil)
	assert.Equal(t, 0.0, s.metric(t, "qabz_circuit_breaker_state", breakerLabels))
	assert.Equal(t, 1.0, s.metric(t, "qabz_circuit_breaker_transitions_total", map[string]string{"provider": "parsian", "bill_type": "mobile", "action": "inquiry", "to": "closed"}))
	assert.Equal(t, 1, s.log.count("info", "circuit breaker: closed, row back"))
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
// nothing (no_next) and the next request goes to the same row. Task 060 (decision 7): after no_next
// the row does not count for the open time, so the second down call tries no demotion.
func TestLiveSwitcherOneRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterCfg(t, ctx, config.MockReturnError, "", func(c *config.Config) { liveBreaker(c, time.Hour) })
	noNext := map[string]string{"bill_type": "irancell", "action": "inquiry", "result": "no_next"}
	_, err := s.askPhone(liveUserOne, irancellPhoneReq("989351112233"))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Eventually(t, func() bool { return s.metric(t, "qabz_provider_switches_total", noNext) == 1 }, 3*time.Second, 5*time.Millisecond)
	_, err = s.askPhone(liveUserOne, irancellPhoneReq("989351112233"))
	assert.Equal(t, codes.Unavailable, status.Code(err))
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1.0, s.metric(t, "qabz_provider_switches_total", noNext), "paused for the open time")
	assert.Equal(t, 1, s.log.count("info", "providers: demote skipped, no next available provider"))
	assert.Equal(t, 0, s.log.count("info", "circuit breaker: open, row demoted"))
	assert.Equal(t, 2.0, s.metric(t, "qabz_provider_calls_total", map[string]string{"provider": "irancell", "action": "inquiry", "result": "down"}), "the same row both times")
	p, err := s.providers.PickProvider("irancell", provider.ActionInquiry)
	if assert.NoError(t, err) {
		assert.Equal(t, int64(38), p.ID)
		assert.False(t, p.Demoted)
	}
}
