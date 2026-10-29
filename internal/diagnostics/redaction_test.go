package diagnostics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProtectCommandLineCredentials(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want []string
	}{
		{"split token", []string{"tools-server", "--token", "opaque credential with spaces", "--port", "9000"}, []string{"tools-server", "--token", "[redacted]", "--port", "9000"}},
		{"inline API key", []string{"--api-key=opaque-inline-value", "--transport=stdio"}, []string{"--api-key=[redacted]", "--transport=stdio"}},
		{"multiple options", []string{"--client-secret", "opaque-client-value", "--password=opaque-password", "--access-token", "opaque-access-value", "public-position"}, []string{"--client-secret", "[redacted]", "--password=[redacted]", "--access-token", "[redacted]", "public-position"}},
		{"option spelling", []string{"--API_KEY", "opaque-upper-value", "-auth", "opaque-auth-value", "--refresh_token=opaque-refresh-value"}, []string{"--API_KEY", "[redacted]", "-auth", "[redacted]", "--refresh_token=[redacted]"}},
		{"missing value", []string{"tools-server", "--token"}, []string{"tools-server", "--token"}},
		{"public arguments", []string{"tools-server", "public-token", "--port", "9000", "--verbose", "--token-count", "100"}, []string{"tools-server", "public-token", "--port", "9000", "--verbose", "--token-count", "100"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The same array can appear directly in runtime configuration or in
			// a JSON-encoded configuration/tool payload. Exercise both paths.
			for _, embedded := range []bool{false, true} {
				input := map[string]any{"args": test.args}
				var value any = input
				if embedded {
					raw, _ := json.Marshal(input)
					value = map[string]string{"arguments": string(raw)}
				}
				raw := Protect(value)
				// Journaling and export both protect the same evidence.
				if again := Protect(raw); string(again) != string(raw) {
					t.Fatal("repeated protection changed the redacted evidence")
				}
				if embedded {
					var outer map[string]string
					if err := json.Unmarshal(raw, &outer); err != nil {
						t.Fatal(err)
					}
					raw = json.RawMessage(outer["arguments"])
				}
				var result struct {
					Args []string `json:"args"`
				}
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(result.Args, test.want) {
					t.Fatalf("args changed (embedded=%v): got %q, want %q", embedded, result.Args, test.want)
				}
			}
		})
	}
}

func TestProtectEmbeddedJSONKeepsNumericTokensAndCompleteText(t *testing.T) {
	for _, arguments := range []string{
		`{"record_id":9007199254740993}`,
		`{"bigint":-90071992547409931234567890,"decimal":1.000000000000000000001,"negative_zero":-0}`,
		`{"huge":1e400,"small":1e-400}`,
		`[9007199254740993,{"nested":"{\"record_id\":9007199254740993}"}]`,
		`{"record_id":9007199254740993} trailing text`,
		`{"record_id":9007199254740993} {"record_id":2}`,
	} {
		t.Run(arguments, func(t *testing.T) {
			raw := Protect(map[string]string{"arguments": arguments})
			for i := 0; i < 3; i++ {
				raw = Protect(raw)
			}
			var result map[string]string
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if result["arguments"] != arguments {
				t.Fatalf("embedded JSON changed: got %s, want %s", result["arguments"], arguments)
			}
		})
	}
}

func TestProtectLimitsArgumentSemanticsToCommandFields(t *testing.T) {
	input := map[string]any{
		"enum":     []string{"--token", "public-option"},
		"messages": []string{"--api-key", "public-message"},
		"argv":     []string{"tools-server", "--token", "opaque-argv-value"},
		"command":  []string{"tools-server", "--password", "opaque-command-value"},
		"args":     `["tools-server","--token","opaque-encoded-value"]`,
	}
	want := map[string]any{
		"enum":     []string{"--token", "public-option"},
		"messages": []string{"--api-key", "public-message"},
		"argv":     []string{"tools-server", "--token", "[redacted]"},
		"command":  []string{"tools-server", "--password", "[redacted]"},
		"args":     `["tools-server","--token","[redacted]"]`,
	}
	raw := Protect(input)
	expected, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(expected) {
		t.Fatalf("command credentials or unrelated array content changed: got %s, want %s", raw, expected)
	}
}

func TestRuntimeConfigurationCredentialsAndNumbersSurviveRestart(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	jsonPath := filepath.Join(root, "config.json")
	for path, content := range map[string]string{
		tomlPath: "[mcp_servers.tools]\ncommand = 'tools-server'\nargs = ['--token', 'opaque-toml-credential', '--port', '9000']\n",
		jsonPath: `{"record_id":9007199254740993,"mcp_servers":{"tools":{"args":["--api-key","opaque-json-credential","--transport","stdio"]}}}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(root, "diagnostics")
	store := New(dir)
	record := store.Begin("room", "source", "", "manager", "turn")
	record.CaptureRuntimeFiles(tomlPath, jsonPath)
	record.Capture("runtime.configuration", map[string]any{"args": []string{"--token", "opaque-runtime-credential"}})
	record.Finish("succeeded")
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries := contextEntries(t, New(dir), record.Snapshot().ID)
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"opaque-toml-credential", "opaque-json-credential", "opaque-runtime-credential"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("credential persisted in diagnostic journal: %s", secret)
		}
	}
	for _, keep := range []string{"9007199254740993", "tools-server", "--port", "9000", "--transport", "stdio"} {
		if !strings.Contains(string(raw), keep) {
			t.Errorf("execution evidence changed after journal restart: %s", keep)
		}
	}
}
