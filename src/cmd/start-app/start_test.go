package startapp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func configFile(t *testing.T, doc string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	assert.NoError(t, os.WriteFile(p, []byte(doc), 0o600))
	return "--config=" + p
}

func TestRunWithoutConfigPanics(t *testing.T) {
	assert.Panics(t, func() { Run(nil) })
	assert.Panics(t, func() { Run([]string{"--config=/no/such/file.yml"}) })
}

func TestRunStartFailure(t *testing.T) {
	arg := configFile(t, `
mode: dev
database:
  postgres: {host: "127.0.0.1", port: 1, password: "x"}
metrics:
  prometheus: {addr: "127.0.0.1:0"}
`)
	assert.Equal(t, 1, Run([]string{arg}))
}

func TestRunBadLogLevel(t *testing.T) {
	arg := configFile(t, `
mode: dev
logs: {level: "loud"}
database: {postgres: {password: "x"}}
`)
	assert.Equal(t, 1, Run([]string{arg}))
}
