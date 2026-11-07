package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
)

// The config files of the repo root (conventions § 8).
var (
	examplePath = filepath.Join("..", "..", "config.yml.example")
	localPath   = filepath.Join("..", "..", "config.yml")
)

// configPaths returns the path tag of every Config field, sorted.
func configPaths() []string {
	var paths []string
	t := reflect.TypeFor[Config]()
	for i := range t.NumField() {
		if p, ok := t.Field(i).Tag.Lookup("path"); ok && p != "" {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return paths
}

// leafPaths returns the dotted path of every leaf value in a YAML mapping, sorted.
func leafPaths(m map[string]any) []string {
	var paths []string
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		sub, ok := v.(map[string]any)
		if !ok {
			paths = append(paths, prefix)
			return
		}
		for k, child := range sub {
			if prefix == "" {
				walk(k, child)
			} else {
				walk(prefix+"."+k, child)
			}
		}
	}
	walk("", m)
	slices.Sort(paths)
	return paths
}

// readYAML reads the YAML mapping in path. It stops the test if that fails.
func readYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	var m map[string]any
	if !assert.NoError(t, yaml.Unmarshal(data, &m)) || !assert.NotNil(t, m) {
		t.FailNow()
	}
	return m
}

// TestExampleHasEveryKey: config.yml.example has exactly the Config keys, no more and no less.
func TestExampleHasEveryKey(t *testing.T) {
	assert.Equal(t, configPaths(), leafPaths(readYAML(t, examplePath)))
}

// TestExampleHasDefaultValues: every value in config.yml.example is the default of its key, or the
// zero value when the key has no default. The required password is the only key set for the check.
func TestExampleHasDefaultValues(t *testing.T) {
	m := readYAML(t, examplePath)
	db, ok := m["database"].(map[string]any)
	if !assert.True(t, ok) {
		t.FailNow()
	}
	pg, ok := db["postgres"].(map[string]any)
	if !assert.True(t, ok) {
		t.FailNow()
	}
	assert.Equal(t, "", pg["password"], "the example has no password")
	pg["password"] = "x"
	data, err := yaml.Marshal(m)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	got, err := Parse(data)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	want := defaults()
	want.PostgresPassword = "x"
	assert.Equal(t, want, got)
}

// TestLocalConfigInSync: the local config.yml (git-ignored) has the same keys as the example. It is
// skipped where config.yml does not exist, for example in CI.
func TestLocalConfigInSync(t *testing.T) {
	if _, err := os.Stat(localPath); err != nil {
		t.Skip("no local config.yml")
	}
	assert.Equal(t, leafPaths(readYAML(t, examplePath)), leafPaths(readYAML(t, localPath)),
		"config.yml and config.yml.example must have the same keys (conventions § 8)")
}

func TestLeafPaths(t *testing.T) {
	m := map[string]any{"a": 1, "b": map[string]any{"c": "x", "d": map[string]any{"e": true}}}
	assert.Equal(t, []string{"a", "b.c", "b.d.e"}, leafPaths(m))
}
