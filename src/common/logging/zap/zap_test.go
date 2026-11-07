package zap

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// capture swaps os.Stdout and os.Stderr for pipes while fn runs and returns what each received.
// zap resolves the "stdout" and "stderr" sinks when New builds the logger, so New must run inside fn.
func capture(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if !assert.NoError(t, err) || outR == nil || outW == nil {
		t.FailNow()
	}
	errR, errW, err := os.Pipe()
	if !assert.NoError(t, err) || errR == nil || errW == nil {
		t.FailNow()
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	outC, errC := make(chan string, 1), make(chan string, 1)
	go func() { b, _ := io.ReadAll(outR); outC <- string(b) }()
	go func() { b, _ := io.ReadAll(errR); errC <- string(b) }()

	fn()
	assert.NoError(t, outW.Close())
	assert.NoError(t, errW.Close())
	return <-outC, <-errC
}

func TestNewWritesToStdout(t *testing.T) {
	stdout, stderr := capture(t, func() {
		l, err := New("debug", "qabz")
		if !assert.NoError(t, err) || !assert.NotNil(t, l) {
			return
		}
		l.Debug("d-line", "k", "v")
		l.Info("i-line", "k", "v")
		l.Warn("w-line", "k", "v")
		l.Error("e-line", "k", "v")
		_ = l.Sync() // Sync on a pipe may fail on some systems
	})
	for _, msg := range []string{"d-line", "i-line", "w-line", "e-line"} {
		assert.Contains(t, stdout, `"msg":"`+msg+`"`)
	}
	assert.Contains(t, stdout, `"service":"qabz"`)
	assert.Equal(t, 4, strings.Count(stdout, "\n"), "one JSON line per log call")
	assert.Empty(t, stderr)
}

func TestNewLevelFilters(t *testing.T) {
	stdout, _ := capture(t, func() {
		l, err := New("warn", "qabz")
		if !assert.NoError(t, err) || !assert.NotNil(t, l) {
			return
		}
		l.Info("hidden", "k", "v")
		l.Warn("shown", "k", "v")
		_ = l.Sync()
	})
	assert.NotContains(t, stdout, "hidden")
	assert.Contains(t, stdout, `"msg":"shown"`)
}

func TestNewLevels(t *testing.T) {
	l, err := New("debug", "qabz")
	if !assert.NoError(t, err) || !assert.NotNil(t, l) {
		return
	}
	l.Debug("d", "k", "v")
	l.Info("i", "k", "v")
	l.Warn("w", "k", "v")
	l.Error("e", "k", "v")
	_ = l.Sync() // stdout sync may fail on some systems

	_, err = New("loud", "qabz")
	assert.Error(t, err)
}
