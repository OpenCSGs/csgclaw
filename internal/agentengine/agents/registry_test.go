package agents

import (
	"csgclaw/internal/agentengine/registry"
	"csgclaw/internal/runtime"
)

func testRuntimeRegistry(items map[string]runtime.Runtime) *registry.Registry {
	r := &registry.Registry{}
	for _, item := range items {
		if err := r.Register(item); err != nil {
			panic(err)
		}
	}
	return r
}
