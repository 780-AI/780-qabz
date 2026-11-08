package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func writeConfig(t *testing.T, doc string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	assert.NoError(t, os.WriteFile(p, []byte(doc), 0o600))
	return p
}

func TestSplitArgs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		path string
		rest []string
		ok   bool
	}{
		{[]string{"--config=a.yml"}, "a.yml", nil, true},
		{[]string{"--config", "a.yml"}, "a.yml", nil, true},
		{[]string{"-config=a.yml", "up"}, "a.yml", []string{"up"}, true},
		{[]string{"force", "3", "--config=a.yml"}, "a.yml", []string{"force", "3"}, true},
		{[]string{"up"}, "", []string{"up"}, false},
		{nil, "", nil, false},
		{[]string{"--config"}, "", nil, false},
		{[]string{"--config="}, "", nil, false},
	} {
		path, rest, err := SplitArgs(tc.args)
		assert.Equal(t, tc.ok, err == nil, "%v", tc.args)
		assert.Equal(t, tc.path, path, "%v", tc.args)
		assert.Equal(t, tc.rest, rest, "%v", tc.args)
	}
}

func TestBootLoads(t *testing.T) {
	p := writeConfig(t, fullYAML)
	var c Config
	var rest []string
	if !assert.NotPanics(t, func() { c, rest = Boot([]string{"up", "--config=" + p}) }) {
		return
	}
	assert.Equal(t, "svc", c.ServiceName)
	assert.Equal(t, []string{"up"}, rest)
}

// Each unusable --config panics: the App never starts with defaults.
func TestBootPanics(t *testing.T) {
	dir := t.TempDir()
	unreadable := filepath.Join(dir, "locked.yml")
	assert.NoError(t, os.WriteFile(unreadable, []byte(fullYAML), 0o000))
	cases := map[string][]string{
		"flag missing":      nil,
		"flag without path": {"--config"},
		"file missing":      {"--config=" + filepath.Join(dir, "none.yml")},
		"file is a folder":  {"--config=" + dir},
		"invalid yaml":      {"--config=" + writeConfig(t, "mode: dev\ndatabase: {postgres: [")},
		"empty file":        {"--config=" + writeConfig(t, "")},
		"rule broken":       {"--config=" + writeConfig(t, "mode: dev\ndatabase: {postgres: {password: pw, port: 0}}")},
	}
	if os.Geteuid() != 0 { // root can read any file
		cases["file unreadable"] = []string{"--config=" + unreadable}
	}
	for name, args := range cases {
		assert.Panics(t, func() { Boot(args) }, name)
	}
}
