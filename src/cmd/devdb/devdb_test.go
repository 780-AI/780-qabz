package devdbcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func configArg(t *testing.T, doc string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	if !assert.NoError(t, os.WriteFile(p, []byte(doc), 0o600)) {
		t.FailNow()
	}
	return "--config=" + p
}

// spy is an opener that records each call and never connects.
type spy struct{ calls int }

var errSpy = errors.New("spy: no connection in unit tests")

func (s *spy) open(context.Context, string) (database, error) {
	s.calls++
	return nil, errSpy
}

func exec(t *testing.T, s *spy, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(args, &o, &e, s.open)
	return code, o.String(), e.String()
}

func modeConfig(mode string) string {
	return "mode: " + mode + "\ndatabase: {postgres: {host: \"127.0.0.1\", port: 1, password: \"s3cret\"}}"
}

func TestUsageErrors(t *testing.T) {
	cfg := configArg(t, modeConfig("dev"))
	for _, args := range [][]string{{}, {cfg}, {cfg, "drop"}, {cfg, "seed", "unseed"}, {cfg, "seed", "x"}} {
		s := &spy{}
		code, _, msg := exec(t, s, args...)
		assert.Equal(t, 2, code, "%v", args)
		assert.Contains(t, msg, "usage: devdb", "%v", args)
		assert.Equal(t, 0, s.calls, "%v: no connection", args)
	}
}

// TestModeGuard: only mode: dev reaches the database. stage and prod exit 1 before they connect; a
// config without mode, or with another value, is not loaded at all (panic, as for every bad config).
func TestModeGuard(t *testing.T) {
	for _, action := range []string{"seed", "unseed"} {
		for _, mode := range []string{"stage", "prod"} {
			s := &spy{}
			code, out, msg := exec(t, s, configArg(t, modeConfig(mode)), action)
			assert.Equal(t, 1, code, "%s %s", mode, action)
			assert.Equal(t, 0, s.calls, "%s %s: no connection", mode, action)
			assert.Contains(t, msg, `devdb: refused: runs only with mode: dev (mode is "`+mode+`"); nothing changed`)
			assert.Empty(t, out)
		}
		for name, doc := range map[string]string{
			"missing": `database: {postgres: {password: "s3cret"}}`,
			"empty":   modeConfig(`""`),
			"other":   modeConfig("development"),
		} {
			s := &spy{}
			assert.Panics(t, func() { exec(t, s, configArg(t, doc), action) }, "%s %s", name, action)
			assert.Equal(t, 0, s.calls, "%s %s: no connection", name, action)
		}
		s := &spy{}
		code, _, msg := exec(t, s, configArg(t, modeConfig("dev")), action)
		assert.Equal(t, 1, code, "dev %s", action)
		assert.Equal(t, 1, s.calls, "dev %s passes the guard and connects", action)
		assert.Contains(t, msg, "devdb: connect: "+errSpy.Error())
	}
}

func TestConfigMissingPanics(t *testing.T) {
	s := &spy{}
	assert.Panics(t, func() { exec(t, s, "seed") })
	assert.Panics(t, func() { exec(t, s, "seed", "--config=/no/such/file.yml") })
	assert.Equal(t, 0, s.calls)
}

// TestFailsWithoutServer runs the real opener against a closed port: exit 1, and the error has no
// password.
func TestFailsWithoutServer(t *testing.T) {
	cfg := configArg(t, "mode: dev\ndatabase:\n  postgres: {host: \"127.0.0.1\", port: 1, password: \"s3cret\"}\n  migration: {timeout: \"5s\"}")
	for _, action := range []string{"seed", "unseed"} {
		var out, errOut bytes.Buffer
		code := run([]string{cfg, action}, &out, &errOut, openPostgres)
		assert.Equal(t, 1, code, action)
		assert.Contains(t, errOut.String(), "devdb "+action+":", action)
		assert.NotContains(t, errOut.String(), "s3cret", action)
		assert.Empty(t, out.String(), action)
	}
}
