package migratecmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func exec(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	return code, errOut.String()
}

func TestUsageErrors(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "x")
	for _, args := range [][]string{{}, {"sideways"}, {"down"}, {"down", "0"}, {"down", "x"}, {"force"}, {"force", "-1"}} {
		code, _ := exec(t, args...)
		assert.Equal(t, 2, code, "%v", args)
	}
}

func TestConfigError(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "")
	code, msg := exec(t, "up")
	assert.Equal(t, 2, code)
	assert.Contains(t, msg, "POSTGRES_PASSWORD is required")
}

func TestUpFailsWithoutServer(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "s3cret")
	t.Setenv("POSTGRES_HOST", "127.0.0.1")
	t.Setenv("POSTGRES_PORT", "1")
	t.Setenv("MIGRATE_TIMEOUT", "5s")
	code, msg := exec(t, "up")
	assert.Equal(t, 1, code)
	assert.NotContains(t, msg, "s3cret")
}
