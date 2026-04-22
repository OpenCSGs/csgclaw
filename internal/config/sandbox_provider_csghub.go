//go:build csghub

package config

// csghub builds default to the dedicated Hub-backed sandbox provider and avoid
// compiling BoxLite-backed defaults into that build shape.
const DefaultSandboxProvider = CSGHubProvider
