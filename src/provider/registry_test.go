package provider

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	a := &stubAdapter{name: "qabzino"}
	assert.NoError(t, r.Register(a))
	got, ok := r.Get("qabzino")
	assert.True(t, ok)
	assert.Same(t, a, got)
	_, ok = r.Get("parsian")
	assert.False(t, ok)

	assert.ErrorContains(t, r.Register(&stubAdapter{name: "qabzino"}), "registered already")
	assert.Error(t, r.Register(nil))
	assert.Error(t, r.Register(&stubAdapter{}))
}

func TestRegistryNilAndZeroValue(t *testing.T) {
	var nilReg *Registry
	assert.Error(t, nilReg.Register(&stubAdapter{name: "a"}))
	_, ok := nilReg.Get("a")
	assert.False(t, ok)

	var zero Registry
	assert.NoError(t, zero.Register(&stubAdapter{name: "a"}), "the zero value is usable")
	_, ok = zero.Get("a")
	assert.True(t, ok)
}

func TestRegistryConcurrent(t *testing.T) {
	r := NewRegistry()
	assert.NoError(t, r.Register(&stubAdapter{name: "a"}))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_, _ = r.Get("a")
			}
		}()
	}
	wg.Wait()
}
