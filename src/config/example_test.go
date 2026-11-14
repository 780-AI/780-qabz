package config

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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
// zero value when the key has no default. The required password and mode are the only keys set for
// the check.
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
	assert.Equal(t, "", m["mode"], "the example has no mode")
	pg["password"] = "x"
	m["mode"] = ModeProd
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
	want.Mode = ModeProd
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

// TestProvidersGroups (task 041): the providers section has only general, internal and external;
// internal holds our own services and external the third-party services. Each Config path tag under
// providers is in one of the three groups, and each provider of the example is in its group.
func TestProvidersGroups(t *testing.T) {
	providers, ok := readYAML(t, examplePath)["providers"].(map[string]any)
	if !assert.True(t, ok, "config.yml.example has no providers object") {
		return
	}
	keys := func(v any) []string {
		m, _ := v.(map[string]any)
		return slices.Sorted(maps.Keys(m))
	}
	assert.Equal(t, []string{"external", "general", "internal"}, keys(providers))
	assert.Equal(t, []string{"irancell", "jiring_switch", "parsian", "payment_server"}, keys(providers["internal"]))
	assert.Equal(t, []string{"karashop", "qabzino", "tavanir"}, keys(providers["external"]))

	for _, p := range configPaths() {
		parts := strings.Split(p, ".")
		if parts[0] != "providers" {
			continue
		}
		assert.Contains(t, []string{"general", "internal", "external"}, parts[1], "path %s is outside the providers groups", p)
	}
}
