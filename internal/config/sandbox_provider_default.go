package config

// DefaultSandboxProvider defaults to the CLI-backed provider in common builds.
// SDK-tagged builds override this value from sandbox_provider_boxlite_sdk.go.
var DefaultSandboxProvider = BoxLiteCLIProvider
