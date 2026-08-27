package knowledgebase

import (
	"testing"

	"csgclaw/internal/auth"
)

func TestManagedConnectionPrefersCSGHubAccessToken(t *testing.T) {
	t.Setenv("CSGHUB_API_BASE_URL", "https://hub.example.test")
	t.Setenv("CSGHUB_ACCESS_TOKEN", "preferred-access-token")
	t.Setenv("CSGHUB_USER_TOKEN", "legacy-user-token")
	t.Setenv("CSGHUB_AIGATEWAY_BASE_URL", "https://gateway.example.test")

	connection, ok := ManagedConnection()
	if !ok {
		t.Fatal("ManagedConnection() = false")
	}
	if got, want := connection.CSGHubAccessToken, "preferred-access-token"; got != want {
		t.Fatalf("CSGHubAccessToken = %q, want %q", got, want)
	}
	if got, want := connection.AIGatewayBaseURL, "https://gateway.example.test/v1"; got != want {
		t.Fatalf("AIGatewayBaseURL = %q, want %q", got, want)
	}
}

func TestManagedConnectionUsesOfficialDefaultsWithoutBaseURLEnvironment(t *testing.T) {
	t.Setenv("CSGHUB_API_BASE_URL", "")
	t.Setenv("CSGHUB_ACCESS_TOKEN", "managed-access-token")
	t.Setenv("CSGHUB_USER_TOKEN", "")
	t.Setenv("CSGHUB_AIGATEWAY_BASE_URL", "")
	t.Setenv("CSGHUB_AIGATEWAY_URL", "")
	t.Setenv("CSGCLAW_LLM_BASE_URL", "")

	connection, ok := ManagedConnection()
	if !ok {
		t.Fatal("ManagedConnection() = false")
	}
	if got, want := connection.CSGHubBaseURL, auth.DefaultCSGHubBaseURL; got != want {
		t.Fatalf("CSGHubBaseURL = %q, want %q", got, want)
	}
	if got, want := connection.AIGatewayBaseURL, auth.DefaultAIGatewayBaseURL; got != want {
		t.Fatalf("AIGatewayBaseURL = %q, want %q", got, want)
	}
}
