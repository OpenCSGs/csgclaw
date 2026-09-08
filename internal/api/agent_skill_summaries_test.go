package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/auth"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/runtimecatalog"
	webui "csgclaw/web"
)

// Opt-in full Profile-page verification using real HTTP/Engine metadata reads.
func TestSkillSummariesBrowserFixture(t *testing.T) {
	ready := os.Getenv("CSGCLAW_SKILLS_E2E_READY_FILE")
	if ready == "" {
		t.Skip("set CSGCLAW_SKILLS_E2E_READY_FILE for headless Profile verification")
	}
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))
	item := completeWorkerAgent("agent-skills", "Skills Verification")
	item.RuntimeKind = agent.RuntimeKindCodex
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{item}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))
	layout, err := controller.AgentLayout(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		dir := filepath.Join(layout.SkillsRoot, fmt.Sprintf("skill-%03d", i))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(fmt.Sprintf("---\ndescription: Batch description %03d\n---\n# Body loaded only on demand\n", i)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	engine := agentengine.New(controller)
	h := NewHandlerWithAuth(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin", Name: "admin", Role: "admin"}}}), nil, nil, nil, nil, "", true)
	h.SetParticipantService(participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "pt-skills", Channel: "csgclaw", Type: participant.TypeAgent, AgentID: item.ID, Name: "Skills Verification", ChannelUserRef: "user-skills", ChannelUserKind: participant.ChannelUserKindLocalUserID}}), participant.WithAgentEngine(engine)))
	h.SetAgentRuntimeService(runtimecatalog.NewService())
	router := h.Routes()
	finished := make(chan struct{})
	router.Post("/__e2e/finish", func(w http.ResponseWriter, _ *http.Request) { close(finished); w.WriteHeader(204) })
	router.Handle("/*", webui.Handler())
	server := httptest.NewServer(router)
	defer server.Close()
	data, _ := json.Marshal(map[string]string{"url": server.URL})
	if err := os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(150 * time.Second):
		t.Fatal("browser verification timed out")
	}
}

func TestAgentSkillSummariesThroughEngineHTTP(t *testing.T) {
	for _, kind := range []string{agent.RuntimeKindCodex, agent.RuntimeKindOpenClawSandbox, agent.RuntimeKindPicoClawSandbox} {
		t.Run(kind, func(t *testing.T) {
			item := completeWorkerAgent("agent-skills", "skills")
			item.RuntimeKind = kind
			controller := mustNewSeededServiceWithOptions(t, []agent.Agent{item}, agent.WithRuntime(fakeCompatRuntime{kind: kind}))
			layout, err := controller.AgentLayout(item.ID)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 1000; i++ {
				dir := filepath.Join(layout.SkillsRoot, fmt.Sprintf("skill-%04d", i))
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(fmt.Sprintf("---\ndescription: Skill %d\n---\nprivate-body-marker", i)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			engine := agentengine.New(controller)
			// This endpoint needs only the Engine, not API-owned Runtime paths.
			h := NewHandler(AgentServices{}, engine, nil, nil, nil, nil, nil)
			w := httptest.NewRecorder()
			h.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-skills/skill-summaries", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("HTTP %d %s", w.Code, w.Body)
			}
			var items []agentengine.SkillSummary
			if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
				t.Fatal(err)
			}
			if len(items) != 1000 || items[0].Name != "skill-0000" || items[999].Description != "Skill 999" {
				t.Fatalf("unexpected summaries: count=%d", len(items))
			}
			if strings.Contains(w.Body.String(), "private-body-marker") || strings.Contains(w.Body.String(), layout.SkillsRoot) {
				t.Fatal("summary leaked body or Runtime path")
			}
			plain, err := engine.Agents().Get(context.Background(), item.ID, agentengine.AgentGetOptions{})
			if err != nil || plain.Status.SkillSummaries != nil {
				t.Fatalf("summaries were not opt-in: %v", err)
			}
		})
	}
}

func TestAgentSkillSummariesMemoryClientHTTP(t *testing.T) {
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-skills", Spec: agentengine.AgentSpec{Name: "skills", Skills: []string{"beta", "alpha"}}, Status: agentengine.AgentStatus{SkillSummaries: []agentengine.SkillSummary{{Name: "beta", Description: "B"}, {Name: "alpha", Description: "A"}}}})
	h := NewHandler(AgentServices{}, engine, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-skills/skill-summaries", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"description":"A"`) {
		t.Fatalf("HTTP %d %s", w.Code, w.Body)
	}
	item, err := engine.Agents().Get(context.Background(), "agent-skills", agentengine.AgentGetOptions{IncludeSkillSummaries: true})
	if err != nil {
		t.Fatal(err)
	}
	item.Status.SkillSummaries[0].Description = "mutated"
	item, _ = engine.Agents().Get(context.Background(), "agent-skills", agentengine.AgentGetOptions{IncludeSkillSummaries: true})
	if item.Status.SkillSummaries[0].Description != "A" {
		t.Fatal("summary leaked mutable state")
	}
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/agents/missing/skill-summaries", nil))
	if w.Code != 404 {
		t.Fatalf("missing Agent HTTP=%d", w.Code)
	}
}

func TestAgentSkillSummariesReadErrorsHTTP(t *testing.T) {
	for _, tc := range []struct {
		name        string
		invalidRoot bool
		agentID     string
		wantStatus  int
		wantBody    string
	}{
		{name: "storage failure", invalidRoot: true, agentID: "agent-skills", wantStatus: http.StatusInternalServerError, wantBody: "Agent skill metadata is unavailable\n"},
		{name: "missing skills directory", agentID: "agent-skills", wantStatus: http.StatusOK, wantBody: "[]\n"},
		{name: "missing agent", agentID: "missing", wantStatus: http.StatusNotFound, wantBody: "agent not found\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := completeWorkerAgent("agent-skills", "skills")
			item.RuntimeKind = agent.RuntimeKindCodex
			controller := mustNewSeededServiceWithOptions(t, []agent.Agent{item}, agent.WithRuntime(fakeCompatRuntime{kind: item.RuntimeKind}))
			layout, err := controller.AgentLayout(item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.invalidRoot {
				if err := os.MkdirAll(filepath.Dir(layout.SkillsRoot), 0700); err != nil {
					t.Fatal(err)
				}
				// A regular file reliably makes OpenRoot fail, even when tests run as root.
				if err := os.WriteFile(layout.SkillsRoot, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			h := NewHandler(AgentServices{}, agentengine.New(controller), nil, nil, nil, nil, nil)
			server := httptest.NewServer(h.Routes())
			defer server.Close()
			resp, err := server.Client().Get(server.URL + "/api/v1/agents/" + tc.agentID + "/skill-summaries")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.wantStatus || string(body) != tc.wantBody {
				t.Fatalf("HTTP %d %q, want %d %q", resp.StatusCode, body, tc.wantStatus, tc.wantBody)
			}
			if strings.Contains(string(body), layout.SkillsRoot) {
				t.Fatal("response leaked Runtime path")
			}
		})
	}
}
