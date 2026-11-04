package prometheus

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServeMetrics(t *testing.T) {
	s := New("127.0.0.1:0")
	if !assert.NoError(t, s.Start(context.Background())) {
		return
	}
	defer func() { assert.NoError(t, s.Stop(context.Background())) }()
	resp, err := http.Get("http://" + s.Addr() + "/metrics")
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(body), "go_goroutines")
}

func TestStartFailsOnBusyPort(t *testing.T) {
	a := New("127.0.0.1:0")
	if !assert.NoError(t, a.Start(context.Background())) {
		return
	}
	defer a.Stop(context.Background())
	assert.Error(t, New(a.Addr()).Start(context.Background()))
}
