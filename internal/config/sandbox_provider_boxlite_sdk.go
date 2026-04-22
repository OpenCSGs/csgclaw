//go:build boxlite_sdk

package config

// When boxlite_sdk is enabled, prefer the SDK-backed provider as the default.
const DefaultSandboxProvider = BoxLiteSDKProvider
