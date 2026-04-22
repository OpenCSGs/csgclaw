//go:build csghub

package sandboxproviders

import (
	"slices"
	"testing"

	"csgclaw/internal/config"
)

func TestSupportedProvidersIncludeCSGHubWithBuildTag(t *testing.T) {
	supported := SupportedProviders()
	if !slices.Contains(supported, config.CSGHubProvider) {
		t.Fatalf("SupportedProviders() = %v, want %q with csghub tag", supported, config.CSGHubProvider)
	}
}

func TestSupportedProvidersExcludeBoxLiteWithCSGHubBuildTag(t *testing.T) {
	supported := SupportedProviders()
	if slices.Contains(supported, config.BoxLiteCLIProvider) {
		t.Fatalf("SupportedProviders() = %v, did not expect %q with csghub tag", supported, config.BoxLiteCLIProvider)
	}
	if slices.Contains(supported, config.BoxLiteSDKProvider) {
		t.Fatalf("SupportedProviders() = %v, did not expect %q with csghub tag", supported, config.BoxLiteSDKProvider)
	}
}
