package cli

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestExecuteAppUsesRegisteredCommand(t *testing.T) {
	t.Setenv("CSGCLAW_CALLER_AGENT_ID", "")
	var out bytes.Buffer
	app := &App{stdout: &out, stderr: &bytes.Buffer{}, httpClient: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/apps" {
			t.Fatalf("unexpected App path %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"items":[{"app_id":"gitlab","name":"GitLab"}]}`), nil
	})}
	if err := app.Execute(context.Background(), []string{"--output", "json", "app", "catalog"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "gitlab") {
		t.Fatal("registered App command did not print catalog")
	}
}
