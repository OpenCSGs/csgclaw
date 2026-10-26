package larkcli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSelectedMigrationConfigRetainsCredentialReferencesAndIdentityPolicy(t *testing.T) {
	input := []byte(`{"currentApp":"selected","strictMode":true,"apps":[{"name":"other","appId":"cli_other"},{"name":"selected","appId":"cli_selected","appSecret":{"source":"keychain"},"users":[{"openId":"ou_existing","scope":"docs:read"}],"defaultAs":"auto"}]}`)
	data, identity, err := SelectConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if identity.AppID != "cli_selected" {
		t.Fatalf("identity=%+v", identity)
	}
	var selected struct {
		StrictMode bool                         `json:"strictMode"`
		Apps       []map[string]json.RawMessage `json:"apps"`
	}
	if err := json.Unmarshal(data, &selected); err != nil {
		t.Fatal(err)
	}
	if !selected.StrictMode || len(selected.Apps) != 1 || !strings.Contains(string(selected.Apps[0]["users"]), "ou_existing") || !strings.Contains(string(selected.Apps[0]["appSecret"]), "keychain") {
		t.Fatalf("selected=%s", data)
	}
}
func TestManagementEnvironmentUsesOnlyResourceConfiguration(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "host-config")
	t.Setenv("LARK_CHANNEL_CONFIG", "channel-config")
	t.Setenv("LARKSUITE_CLI_USER_ACCESS_TOKEN", "host-token")
	environment := (Client{ConfigDir: "resource-config", OperationID: "operation"}).command().Environment()
	values := strings.Join(environment, "\n")
	for _, forbidden := range []string{"host-config", "channel-config", "host-token", "host-dsh"} {
		if strings.Contains(values, forbidden) {
			t.Fatalf("inherited configuration: %s", forbidden)
		}
	}
	if !strings.Contains(values, "LARKSUITE_CLI_CONFIG_DIR=resource-config") {
		t.Fatal("resource configuration missing")
	}
}
