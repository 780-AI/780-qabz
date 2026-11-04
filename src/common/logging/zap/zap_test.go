package zap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLevels(t *testing.T) {
	l, err := New("debug", "qabz")
	if !assert.NoError(t, err) || !assert.NotNil(t, l) {
		return
	}
	l.Debug("d", "k", "v")
	l.Info("i", "k", "v")
	l.Warn("w", "k", "v")
	l.Error("e", "k", "v")
	_ = l.Sync() // stderr sync may fail on some systems

	_, err = New("loud", "qabz")
	assert.Error(t, err)
}
