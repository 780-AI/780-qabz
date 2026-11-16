package app

import (
	"fmt"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"

	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/system/nats"
)

// natsNeeded reports whether the Irancell payment needs the NATS client (task 056): the Irancell
// adapter is enabled and its mock does not answer (the mock applies only in dev mode).
func natsNeeded(cfg config.Config) bool {
	mock := cfg.Mode == config.ModeDev && cfg.DebugMockIrancellEnabled
	return cfg.IrancellEnabled && !mock
}

// newNATS builds the NATS client of the Irancell payment (task 056; bill: client/broker nats_dto.go)
// when natsNeeded, else returns nil and writes nothing. With no nats.endpoints or no
// providers.internal.irancell.nats_subject it writes one warn line and returns nil: every Irancell
// payment is then pending (as a provider that is down). When no server answers, the client still
// exists and connects again in the background (system/nats.New; operator decision Q2 (a)). With
// metrics on:
//   - qabz_nats_requests_total{subject,result}: each request (ok, timeout, no_responders,
//     no_connection, error);
//   - qabz_nats_request_duration_ms{subject}: the duration of each request, in milliseconds;
//   - qabz_nats_connected: 1 while the connection is up, else 0.
func newNATS(cfg config.Config, metrics *prometheus.Server, logger Logger) (*nats.Client, error) {
	if !natsNeeded(cfg) {
		return nil, nil
	}
	if len(cfg.NATSEndpoints) == 0 || cfg.IrancellNATSSubject == "" {
		if logger != nil {
			logger.Warn("nats client off (nats.endpoints or providers.internal.irancell.nats_subject is empty): every Irancell payment is pending",
				"endpoints", len(cfg.NATSEndpoints), "subject_set", cfg.IrancellNATSSubject != "")
		}
		return nil, nil
	}
	var onRequest func(subject, result string, d time.Duration)
	var onConnected func(up bool)
	if reg := metrics.Registry(); reg != nil {
		requests := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_nats_requests_total",
			Help: "NATS requests (task 056: the Irancell bill payment) by subject and result (ok: an answer came; timeout: no answer before the limit, also while the client connects again; no_responders: no subscriber of the subject; no_connection: the connection is closed; error: any other failure).",
		}, []string{"subject", "result"})
		duration := promclient.NewHistogramVec(promclient.HistogramOpts{
			Name:    "qabz_nats_request_duration_ms",
			Help:    "Duration of each NATS request in milliseconds, by subject, for every result.",
			Buckets: []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000},
		}, []string{"subject"})
		connected := promclient.NewGauge(promclient.GaugeOpts{
			Name: "qabz_nats_connected",
			Help: "1 while the NATS client is connected, else 0 (task 056; the client exists only while the Irancell adapter is enabled and its mock is off).",
		})
		for _, c := range []promclient.Collector{requests, duration, connected} {
			if err := reg.Register(c); err != nil {
				return nil, fmt.Errorf("nats: register metric: %w", err)
			}
		}
		onRequest = func(subject, result string, d time.Duration) {
			requests.WithLabelValues(subject, result).Inc()
			duration.WithLabelValues(subject).Observe(float64(d) / float64(time.Millisecond))
		}
		onConnected = func(up bool) {
			if up {
				connected.Set(1)
			} else {
				connected.Set(0)
			}
		}
	}
	var log nats.Logger
	if logger != nil {
		log = logger
	}
	return nats.New(nats.Options{
		Name:           cfg.ServiceName,
		Servers:        cfg.NATSEndpoints,
		User:           cfg.NATSUsername,
		Password:       cfg.NATSPassword.Reveal(),
		Secure:         cfg.NATSAuth,
		AllowReconnect: cfg.NATSAllowReconnect,
		MaxReconnect:   cfg.NATSMaxReconnect,
		ReconnectWait:  cfg.NATSReconnectWait,
		Timeout:        cfg.NATSTimeout,
		Logger:         log,
		OnRequest:      onRequest,
		OnConnected:    onConnected,
	})
}
