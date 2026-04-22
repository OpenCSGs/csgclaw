//go:build !boxlite_sdk

package config

// DefaultSandboxProvider varies by build shape so that binaries compiled
// without the BoxLite SDK still have a usable sandbox backend by default.
const DefaultSandboxProvider = BoxLiteCLIProvider
