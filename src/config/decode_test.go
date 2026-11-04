package config

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
)

type testPool struct {
	Name string `path:"name"`
	Size uint16 `path:"size"`
}

type testNode struct {
	Host string   `path:"host"`
	Tags []string `path:"tags"`
}

// testDoc is a custom struct: every kind the mapper supports, at several depths.
type testDoc struct {
	Flat       string              `path:"flat"`
	Single     int                 `path:"single"`
	Deep       string              `path:"a.b.c.d"`
	Ints       []int               `path:"ints"`
	Small      int8                `path:"small"`
	Big        int64               `path:"big"`
	Huge       uint64              `path:"huge"`
	Unsigned   uint32              `path:"unsigned"`
	Words      []string            `path:"words"`
	Pools      []testPool          `path:"pools"`
	Matrix     [][]int             `path:"matrix"`
	Pair       [2]string           `path:"pair"`
	Labels     map[string]string   `path:"labels"`
	Limits     map[string]int      `path:"limits"`
	Nodes      map[string]testNode `path:"cluster.nodes"`
	Nested     testNode            `path:"nested"`
	Timeout    time.Duration       `path:"timeout"`
	Timeouts   []time.Duration     `path:"timeouts"`
	Enabled    bool                `path:"enabled"`
	Secret     Secret              `path:"secret"`
	Untagged   string
	Missing    string `path:"not.there"`
	privateTag string `path:"flat"`
}

func decodeDoc(doc string) (testDoc, error) {
	var root any
	if err := yaml.Unmarshal([]byte(doc), &root); err != nil {
		return testDoc{}, err
	}
	m, ok := root.(map[string]any)
	if !ok {
		return testDoc{}, errors.New("test document is not an object")
	}
	d := testDoc{Missing: "kept"}
	err := decodeStruct(m, reflectValue(&d), "")
	return d, err
}

const complexYAML = `
flat: "text"
single: 7
a: {b: {c: {d: "deep"}}}
ints: [1, 2, 3]
small: -128
big: 9007199254740993
unsigned: 4000000000
words: [a, b]
pools:
  - {name: "p1", size: 10}
  - {name: "p2", size: 65535}
matrix: [[1, 2], [3], []]
pair: [x, y]
labels: {env: "prod", tier: "gold"}
limits: {cpu: 4, mem: 512}
cluster:
  nodes:
    n1: {host: "h1", tags: [t1, t2]}
    n2: {host: "h2", tags: []}
nested: {host: "inner", tags: [z]}
timeout: "1m30s"
timeouts: ["1s", "2h"]
enabled: true
secret: "s3cret"
Untagged: "ignored"
`

func TestDecodeComplexDocument(t *testing.T) {
	d, err := decodeDoc(complexYAML)
	assert.NoError(t, err)
	assert.Equal(t, "text", d.Flat)
	assert.Equal(t, 7, d.Single)
	assert.Equal(t, "deep", d.Deep)
	assert.Equal(t, []int{1, 2, 3}, d.Ints)
	assert.Equal(t, int8(-128), d.Small)
	assert.Equal(t, int64(9007199254740993), d.Big)
	assert.Equal(t, uint32(4000000000), d.Unsigned)
	assert.Equal(t, []string{"a", "b"}, d.Words)
	assert.Equal(t, []testPool{{"p1", 10}, {"p2", 65535}}, d.Pools)
	assert.Equal(t, [][]int{{1, 2}, {3}, {}}, d.Matrix)
	assert.Equal(t, [2]string{"x", "y"}, d.Pair)
	assert.Equal(t, map[string]string{"env": "prod", "tier": "gold"}, d.Labels)
	assert.Equal(t, map[string]int{"cpu": 4, "mem": 512}, d.Limits)
	assert.Equal(t, map[string]testNode{"n1": {"h1", []string{"t1", "t2"}}, "n2": {"h2", []string{}}}, d.Nodes)
	assert.Equal(t, testNode{"inner", []string{"z"}}, d.Nested)
	assert.Equal(t, 90*time.Second, d.Timeout)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Hour}, d.Timeouts)
	assert.True(t, d.Enabled)
	assert.Equal(t, Secret("s3cret"), d.Secret)
	assert.Empty(t, d.Untagged)
	assert.Equal(t, "kept", d.Missing) // a missing path keeps the value
}

func TestDecodeLargeUnsigned(t *testing.T) {
	d, err := decodeDoc("huge: 18446744073709551615")
	assert.NoError(t, err)
	assert.Equal(t, uint64(18446744073709551615), d.Huge)
}

func TestDecodeSingleKeyAndEmpty(t *testing.T) {
	d, err := decodeDoc("single: 1")
	assert.NoError(t, err)
	assert.Equal(t, 1, d.Single)
	assert.Empty(t, d.Flat)
	d, err = decodeDoc("{}")
	assert.NoError(t, err)
	assert.Equal(t, "kept", d.Missing)
}

// Every wrong pairing of a YAML kind and a Go type is an error.
func TestDecodeKindMismatch(t *testing.T) {
	for name, doc := range map[string]string{
		"[]T given an object":        "words: {a: b}",
		"[]T given a scalar":         "words: abc",
		"[]T element is an object":   "words: [{a: b}]",
		"[]T element is an array":    "words: [[a]]",
		"[]T element has wrong type": "ints: [1, two]",
		"array length differs":       "pair: [x]",
		"struct given an array":      "nested: [a]",
		"struct given a scalar":      "nested: abc",
		"map given an array":         "labels: [a]",
		"map given a scalar":         "labels: abc",
		"map value has wrong type":   "limits: {cpu: many}",
		"scalar given an array":      "flat: [a]",
		"scalar given an object":     "flat: {a: b}",
		"string given an int":        "flat: 5",
		"string given a bool":        "flat: true",
		"int given a string":         "single: \"1\"",
		"int given a float":          "single: 1.5",
		"int given a bool":           "single: true",
		"int8 overflow":              "small: 128",
		"uint given a negative":      "unsigned: -1",
		"int64 above max":            "big: 9223372036854775808",
		"uint64 given a negative":    "huge: -1",
		"uint32 overflow":            "unsigned: 4294967296",
		"bool given a string":        "enabled: \"true\"",
		"bool given an int":          "enabled: 1",
		"duration given an int":      "timeout: 30",
		"duration is not valid":      "timeout: soon",
		"null":                       "flat:",
		"path crosses a scalar":      "a: {b: 5}",
		"path crosses an array":      "a: [1]",
	} {
		_, err := decodeDoc(doc)
		assert.Error(t, err, name)
	}
}

func TestDecodeErrorNamesPathNotValue(t *testing.T) {
	_, err := decodeDoc("pools:\n  - {name: p1, size: 70000}")
	if assert.ErrorContains(t, err, "pools[0].size") {
		assert.NotContains(t, err.Error(), "70000")
	}
	_, err = decodeDoc("secret: [s3cret]")
	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), "s3cret")
	}
}

func TestDecodeUnsupportedGoType(t *testing.T) {
	var v struct {
		F float64        `path:"f"`
		M map[int]string `path:"m"`
	}
	assert.Error(t, decodeStruct(map[string]any{"f": 1.5}, reflectValue(&v), ""))
	assert.Error(t, decodeStruct(map[string]any{"m": map[string]any{"1": "a"}}, reflectValue(&v), ""))
}

// A target that is not a settable struct is an error, not a crash.
func TestDecodeBadTarget(t *testing.T) {
	n := 5
	assert.NotPanics(t, func() {
		assert.Error(t, decodeStruct(map[string]any{}, reflect.Value{}, ""))
		assert.Error(t, decodeStruct(map[string]any{}, reflectValue(&n), ""))
		assert.Error(t, decodeStruct(map[string]any{}, reflect.ValueOf(testDoc{}), ""))
	})
}
