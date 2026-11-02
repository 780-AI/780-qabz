package startapp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunConfigError(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "")
	assert.Equal(t, 2, Run())
}

func TestRunStartFailure(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "x")
	t.Setenv("POSTGRES_HOST", "127.0.0.1")
	t.Setenv("POSTGRES_PORT", "1")
	t.Setenv("METRICS_ADDR", "127.0.0.1:0")
	assert.Equal(t, 1, Run())
}

func TestRunBadLogLevel(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "x")
	t.Setenv("LOG_LEVEL", "loud")
	assert.Equal(t, 1, Run())
}
