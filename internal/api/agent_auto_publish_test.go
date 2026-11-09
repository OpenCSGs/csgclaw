package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/relay"
)

func TestAgentAutoPublicationCanQueueBeforeRuntimeReady(t *testing.T) {
	t.Setenv("CSGCLAW_NAME", "platform-content-id")
	t.Setenv("OPENCSG_SANDBOX_TYPE", "csgclaw")
	h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-startup", "Startup")}, &fakeSessionEngine{}, "")
	// The manager waits on this stopped local runtime; registration must not run yet.
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"agent-startup","status":"stopped","runtime":{"name":"codex","state":"stopped"}}`))
	}))
	defer local.Close()
	root := t.TempDir()
	m, err := relay.NewManager(context.Background(), root, apiclient.New(local.URL, "", local.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h.relayManager = m
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-startup/relay", strings.NewReader(`{"auto_publish":true}`))
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("queue status=%d body=%s", rec.Code, rec.Body.String())
	}
	var status relay.PublicationStatus
	if err = json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Publishing || status.Published {
		t.Fatal("queue did not expose pending state")
	}
	data, err := os.ReadFile(filepath.Join(root, "publications.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Targets map[string]relay.CommunityTarget `json:"targets"`
	}
	if json.Unmarshal(data, &saved) != nil || saved.Targets["agent-startup"].ContentID != "platform-content-id" {
		t.Fatal("did not persist CSGCLAW_NAME as instance content_id")
	}
	if err = m.Unpublish(context.Background(), "agent-startup"); err != nil {
		t.Fatal(err)
	}
	if m.Status(context.Background(), "agent-startup").Publishing {
		t.Fatal("unpublish did not cancel queue")
	}
}
