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
	assert.NoError(t, s.Start(context.Background()))
	resp, err := http.Get("http://" + s.Addr() + "/metrics")
	assert.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(body), "go_goroutines")
	assert.NoError(t, s.Stop(context.Background()))
}

func TestStartFailsOnBusyPort(t *testing.T) {
	a := New("127.0.0.1:0")
	assert.NoError(t, a.Start(context.Background()))
	defer a.Stop(context.Background())
	assert.Error(t, New(a.Addr()).Start(context.Background()))
}
