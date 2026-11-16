package app

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/provider"
	"qabz/provider/irancellproxy"
)

// TestNATSNeeded (task 056): the client is needed only while the Irancell adapter is enabled and its
// mock does not answer (the mock applies only in dev mode).
func TestNATSNeeded(t *testing.T) {
	for _, c := range []struct {
		mode          string
		enabled, mock bool
		want          bool
	}{
		{config.ModeDev, false, false, false},
		{config.ModeDev, true, true, false},
		{config.ModeDev, true, false, true},
		{config.ModeProd, true, true, true}, // the mock never applies outside dev
		{config.ModeProd, false, false, false},
	} {
		cfg := testConfig(t)
		cfg.Mode, cfg.IrancellEnabled, cfg.DebugMockIrancellEnabled = c.mode, c.enabled, c.mock
		assert.Equal(t, c.want, natsNeeded(cfg), "%+v", c)
	}
}

// TestNewNATSOff: not needed: no client and no line; needed with no endpoints or no subject: no
// client and one warn line.
func TestNewNATSOff(t *testing.T) {
	log := &liveLogger{}
	cfg := testConfig(t)
	c, err := newNATS(cfg, prometheus.New("127.0.0.1:0", true), log)
	assert.NoError(t, err)
	assert.Nil(t, c)
	assert.Empty(t, log.lines)

	const warn = "nats client off (nats.endpoints or providers.internal.irancell.nats_subject is empty): every Irancell payment is pending"
	for name, change := range map[string]func(*config.Config){
		"no endpoints": func(c *config.Config) { c.IrancellNATSSubject = "irancell-bill" },
		"no subject":   func(c *config.Config) { c.NATSEndpoints = []string{"nats://127.0.0.1:4222"} },
	} {
		log := &liveLogger{}
		cfg := testConfig(t)
		cfg.IrancellEnabled = true
		change(&cfg)
		c, err := newNATS(cfg, prometheus.New("127.0.0.1:0", true), log)
		assert.NoError(t, err, name)
		assert.Nil(t, c, name)
		assert.Equal(t, 1, log.count("warn", warn), name)
	}
}

// TestNewNATSServerDown (Q2 (a)): with no server the client exists (the App starts), the gauge is 0,
// a payment through the registry is pending ("provider down") and is counted as a timeout.
func TestNewNATSServerDown(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	metrics := prometheus.New("127.0.0.1:0", true)
	cfg := testConfig(t)
	cfg.IrancellEnabled, cfg.IrancellNATSSubject, cfg.NATSEndpoints = true, "irancell-bill", []string{"nats://" + addr}
	cfg.IrancellPayTimeout = 100 * time.Millisecond
	log := &liveLogger{}
	c, err := newNATS(cfg, metrics, log)
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	defer func() { _ = c.Close(context.Background()) }()
	v, ok := metricValue(t, metrics.Registry(), "qabz_nats_connected", nil)
	assert.True(t, ok)
	assert.Equal(t, 0.0, v)
	assert.Equal(t, 1, log.count("error", "nats: connect failed, trying again in the background"))

	reg, err := newProviderRegistryWith(cfg, nil, c)
	if !assert.NoError(t, err) {
		return
	}
	a, ok := reg.Get(irancellproxy.Name)
	if !assert.True(t, ok) {
		return
	}
	_, err = a.Pay(context.Background(), provider.PaymentRequest{BillType: provider.BillTypeIrancell, OrderID: "o", RRN: "1"})
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	v, _ = metricValue(t, metrics.Registry(), "qabz_nats_requests_total", map[string]string{"subject": "irancell-bill", "result": "timeout"})
	assert.Equal(t, 1.0, v)
}
