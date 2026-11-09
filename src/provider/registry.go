package provider

import (
	"errors"
	"fmt"
	"sync"
)

// ProviderRegistry maps a provider name (providers.name) to its adapter (system.html, Provider
// Adapter Interface). The flow picks a provider row with ProvidersService and gets the adapter of
// its name from the registry.
type ProviderRegistry interface {
	Register(p ProviderInterface) error
	Get(name string) (ProviderInterface, bool)
}

// Registry implements ProviderRegistry. It is safe for concurrent use.
type Registry struct {
	mu sync.RWMutex
	m  map[string]ProviderInterface
}

var _ ProviderRegistry = (*Registry)(nil)

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{m: map[string]ProviderInterface{}} }

// Register adds p under p.Name(). A nil adapter, an empty name and a name that is registered
// already are errors.
func (r *Registry) Register(p ProviderInterface) error {
	if r == nil {
		return errors.New("provider: registry is nil")
	}
	if p == nil {
		return errors.New("provider: adapter is nil")
	}
	name := p.Name()
	if name == "" {
		return errors.New("provider: adapter has an empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = map[string]ProviderInterface{}
	}
	if _, dup := r.m[name]; dup {
		return fmt.Errorf("provider: %q is registered already", name)
	}
	r.m[name] = p
	return nil
}

// Get returns the adapter of the provider name.
func (r *Registry) Get(name string) (ProviderInterface, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.m[name]
	return p, ok
}
