// Package registry owns the Runtime implementations available to Agent Engine.
package registry

import (
	"csgclaw/internal/runtime"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// Registry is explicit: missing adapters are errors, never alternate execution paths.
type Registry struct {
	mu       sync.RWMutex
	runtimes map[string]runtime.Runtime
	sealed   bool
}

func (r *Registry) Register(adapter runtime.Runtime) error {
	if adapter == nil || strings.TrimSpace(adapter.Kind()) == "" {
		return fmt.Errorf("Runtime adapter and kind are required")
	}
	kind := strings.TrimSpace(adapter.Kind())
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runtimes == nil {
		r.runtimes = make(map[string]runtime.Runtime)
	}
	if r.sealed {
		return fmt.Errorf("Runtime registry is sealed")
	}
	r.runtimes[kind] = adapter
	return nil
}

// Seal completes composition. Options may select a replacement before this
// point, but execution never races an adapter replacement.
func (r *Registry) Seal() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.sealed = true
	r.mu.Unlock()
}
func (r *Registry) Get(kind string) (runtime.Runtime, error) {
	if r == nil {
		return nil, fmt.Errorf("Runtime registry is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter := r.runtimes[strings.TrimSpace(kind)]
	if adapter == nil {
		return nil, fmt.Errorf("Runtime adapter %q is not registered", kind)
	}
	return adapter, nil
}
func (r *Registry) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	items := r.runtimes
	r.runtimes = nil
	r.mu.Unlock()
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	var result error
	for _, name := range names {
		if closer, ok := items[name].(io.Closer); ok {
			if err := closer.Close(); err != nil {
				result = errors.Join(result, fmt.Errorf("close Runtime %q: %w", name, err))
			}
		}
	}
	return result
}
