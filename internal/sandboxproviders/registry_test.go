package sandboxproviders

import (
	"slices"
	"testing"

	"csgclaw/internal/config"
)

func TestSupportedProvidersAlwaysIncludeBoxLiteCLI(t *testing.T) {
	supported := SupportedProviders()
	if !slices.Contains(supported, config.BoxLiteCLIProvider) {
		t.Fatalf("SupportedProviders() = %v, want %q to be compiled in", supported, config.BoxLiteCLIProvider)
	}
	if len(supported) != 3 {
		t.Fatalf("SupportedProviders() = %v, want exactly the compiled providers", supported)
	}
}

func TestSupportedProvidersIncludeCSGHubWithoutBuildTag(t *testing.T) {
	supported := SupportedProviders()
	if !slices.Contains(supported, config.CSGHubProvider) {
		t.Fatalf("SupportedProviders() = %v, want %q to be compiled in", supported, config.CSGHubProvider)
	}
}

func TestSupportedProvidersIncludeDocker(t *testing.T) {
	supported := SupportedProviders()
	if !slices.Contains(supported, config.DockerProvider) {
		t.Fatalf("SupportedProviders() = %v, want %q to be compiled in", supported, config.DockerProvider)
	}
}
