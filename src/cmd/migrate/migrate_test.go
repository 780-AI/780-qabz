package migratecmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func configArg(t *testing.T, doc string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	assert.NoError(t, os.WriteFile(p, []byte(doc), 0o600))
	return "--config=" + p
}

func exec(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	return code, errOut.String()
}

const okConfig = `database: {postgres: {password: "x"}}`

func TestUsageErrors(t *testing.T) {
	cfg := configArg(t, okConfig)
	for _, args := range [][]string{{}, {cfg}, {cfg, "sideways"}, {cfg, "down", "1"}, {cfg, "force"}, {cfg, "force", "-1"}} {
		code, _ := exec(t, args...)
		assert.Equal(t, 2, code, "%v", args)
	}
}

func TestConfigMissingPanics(t *testing.T) {
	assert.Panics(t, func() { exec(t, "up") })
	assert.Panics(t, func() { exec(t, "up", "--config=/no/such/file.yml") })
	assert.Panics(t, func() { exec(t, "up", configArg(t, `database: {postgres: {port: 5432}}`)) }) // no password
}

func TestUpFailsWithoutServer(t *testing.T) {
	cfg := configArg(t, `
database:
  postgres: {host: "127.0.0.1", port: 1, password: "s3cret"}
  migration: {timeout: "5s"}
`)
	code, msg := exec(t, "up", cfg)
	assert.Equal(t, 1, code)
	assert.NotContains(t, msg, "s3cret")
}
