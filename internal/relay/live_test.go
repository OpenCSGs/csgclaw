package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/apiclient"
	"github.com/google/uuid"
)

// Opt-in smoke test for a running CSGRelay and an actual local runtime.
func TestLiveRelayLocalAgent(t *testing.T) {
	endpoint, token, localURL, localToken, agentID := os.Getenv("CSGCLAW_TEST_RELAY_URL"), os.Getenv("CSGCLAW_TEST_RELAY_TOKEN"), os.Getenv("CSGCLAW_TEST_LOCAL_URL"), os.Getenv("CSGCLAW_TEST_LOCAL_TOKEN"), os.Getenv("CSGCLAW_TEST_LOCAL_AGENT")
	if endpoint == "" || localURL == "" || agentID == "" {
		t.Skip("live relay/local agent not configured")
	}
	client, err := NewClient(endpoint, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := &LocalExecutor{Client: apiclient.New(localURL, localToken, nil), AgentID: agentID, Scope: "live-smoke-" + uuid.NewString()}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if !handler.Ready(ctx) {
		t.Fatal("local agent is not ready")
	}
	p, err := client.Publish(ctx, uuid.NewString(), PublishRequest{Name: "CSGClaw integration smoke test", SourceAgentID: "smoke-" + uuid.NewString(), Executor: "csgclaw", Visibility: "private"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = client.Request(cleanupCtx, http.MethodDelete, "/v1/agents/"+p.Agent.ID, "", nil, nil)
	}()
	e := &Executor{Client: client, AgentID: p.Agent.ID, StatePath: filepath.Join(t.TempDir(), "state.json"), Handler: handler}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- e.Run(runCtx) }()
	defer func() {
		stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	waitOnline(t, client, p.Agent.ID)
	raw, _ := json.Marshal("Do not use tools or change any files. Reply with exactly CSGCLAW_RELAY_OK.")
	result, err := client.Call(ctx, uuid.NewString(), CallRequest{Model: p.Agent.ID, Input: raw})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.Text() != "CSGCLAW_RELAY_OK" {
		t.Fatalf("unexpected runtime result status=%s", result.Status)
	}
}
