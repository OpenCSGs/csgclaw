//go:build boxlite_sdk

package sandboxproviders

import (
	"slices"
	"testing"

	"csgclaw/internal/config"
)

func TestSupportedProvidersIncludeBoxLiteSDKWithBuildTag(t *testing.T) {
	supported := SupportedProviders()
	if !slices.Contains(supported, config.BoxLiteSDKProvider) {
		t.Fatalf("SupportedProviders() = %v, want %q with boxlite_sdk tag", supported, config.BoxLiteSDKProvider)
	}
}
