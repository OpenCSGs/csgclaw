package agent

import (
	"encoding/json"
	"testing"
)

func TestCreateAgentSpecMarshalJSONPreservesExplicitMCPServersValues(t *testing.T) {
	tests := []struct {
		name    string
		servers map[string]any
		want    string
	}{
		{name: "empty map", servers: map[string]any{}, want: "{}"},
		{name: "null", servers: nil, want: "null"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(CreateAgentSpec{
				Name:          "worker",
				MCPServers:    test.servers,
				MCPServersSet: true,
			})
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if got := string(fields["mcpServers"]); got != test.want {
				t.Fatalf("mcpServers = %s, want %s", got, test.want)
			}
		})
	}
}
