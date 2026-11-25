package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/dsh"
)

// Exercise both tools in one real DSH turn. The registry and the model are
// local fixtures; the CLI, DSH's child environment and MCP transport are real.
func TestWorkerSkillInstallNativeDSH(t *testing.T) {
	binary, cli := os.Getenv("CSGCLAW_TEST_DSH_BINARY"), os.Getenv("CSGCLAW_TEST_CLI_BINARY")
	if binary == "" || cli == "" {
		t.Skip("set CSGCLAW_TEST_DSH_BINARY and CSGCLAW_TEST_CLI_BINARY")
	}
	item := completeWorkerAgent("agent-installer", "installer")
	item.RuntimeKind, item.RuntimeName = agent.RuntimeKindDSH, agent.RuntimeKindDSH
	var home string
	ref := dsh.AgentRef{ID: item.ID, Name: item.Name, RuntimeID: item.RuntimeID}
	rt := dsh.New(dsh.Dependencies{
		ResolveBinary:       func(context.Context) (dshcli.Info, error) { return dshcli.Info{Path: binary}, nil },
		ResolveAgent:        func(agentruntime.Handle) (dsh.AgentRef, error) { return ref, nil },
		AgentHome:           func(string) (string, error) { return home, nil },
		DisableNativeMemory: true,
	})
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{item}, agent.WithRuntime(rt))
	t.Cleanup(func() { _ = controller.Close() })
	layout, err := controller.AgentLayout(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	dshRoot := filepath.Dir(layout.WorkspaceRoot)
	home = filepath.Dir(dshRoot)
	h := &Handler{svc: controller, agentEngine: agentengine.New(controller), agentRuntime: controller}
	if err := h.EnableConnectors(filepath.Join(t.TempDir(), "apps.json")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.CloseConnectors() })

	const manifest = "---\nname: demo\ndescription: native install probe\n---\n# Demo"
	mux := http.NewServeMux()
	mux.Handle("/api/v1/agents/", h.Routes())
	mux.HandleFunc("/api/v1/skills/demo", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"skill":{"slug":"demo"},"latestVersion":{"version":"1.0.0"}}`)
	})
	archive := mustZipBytes(t, map[string]string{"SKILL.md": manifest})
	mux.HandleFunc("/api/v1/download/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) })
	view := filepath.Join(dshRoot, "skill-view", "skills", "demo", "SKILL.md")
	var calls atomic.Int32
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tools []struct{ Function struct{ Name string } }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		n := calls.Add(1)
		name := ""
		var args map[string]string
		switch n {
		case 1:
			name = "bash"
			args = map[string]string{"command": `test -z "$CSGCLAW_ACCESS_TOKEN" && "$CSGCLAW_CLI" --output json skill install demo --registry opencsg`, "description": "Install demo"}
		case 2:
			if data, err := os.ReadFile(filepath.Join(layout.SkillsRoot, "demo", "SKILL.md")); err != nil || string(data) != manifest {
				t.Errorf("DSH shell did not install the fixture: %v", err)
			}
			if _, err := os.Stat(view); !os.IsNotExist(err) {
				t.Errorf("view changed before MCP: %v", err)
			}
			name, args = "mcp__csgclaw__skill_ensure_enabled", map[string]string{"name": "demo"}
		}
		delta, finish := map[string]any{"role": "assistant", "content": "done"}, "stop"
		if name != "" {
			found := false
			for _, tool := range body.Tools {
				if tool.Function.Name == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s not advertised in DSH", name)
			}
			raw, _ := json.Marshal(args)
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("step-%d", n), "type": "function", "function": map[string]string{"name": name, "arguments": string(raw)}}}}
			finish = "tool_calls"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, choice := range []map[string]any{{"index": 0, "delta": delta, "finish_reason": nil}, {"index": 0, "delta": map[string]any{}, "finish_reason": finish}} {
			raw, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{choice}})
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	configPath, err := config.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf("[skill]\nbase_url = %q\n", server.URL)), 0600); err != nil {
		t.Fatal(err)
	}
	profile, err := controller.RuntimeHost().ResolveRuntimeProfile(agentruntime.Handle{RuntimeID: item.RuntimeID})
	if err != nil {
		t.Fatal(err)
	}
	profile.BaseURL, profile.ModelID = server.URL+"/v1", "fixture"
	profile.Env["CSGCLAW_BASE_URL"], profile.Env["CSGCLAW_CLI"] = server.URL, cli
	ref.Profile = profile
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	if err := rt.Provision(ctx, agentruntime.ProvisionRequest{AgentID: item.ID, RuntimeID: item.RuntimeID, Profile: profile}); err != nil {
		t.Fatal(err)
	}
	handle, err := rt.New(ctx, agentruntime.Spec{AgentID: item.ID, RuntimeID: item.RuntimeID, Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	pid := func() int {
		data, err := os.ReadFile(filepath.Join(dshRoot, "runtime.json"))
		var meta struct{ PID int }
		if err != nil || json.Unmarshal(data, &meta) != nil || meta.PID <= 0 {
			t.Fatal("missing DSH PID")
		}
		return meta.PID
	}
	before := pid()
	result := rt.Conversation(handle.RuntimeID).Run(ctx, contract.TurnRequest{ID: "install", ConversationKey: "install-room", Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "Install demo and enable it."}}}, nil)
	if result.Status != contract.TurnSucceeded || strings.TrimSpace(result.Output) != "done" || calls.Load() != 3 {
		t.Fatalf("turn status=%s requests=%d error=%v", result.Status, calls.Load(), result.Error)
	}
	if data, err := os.ReadFile(view); err != nil || string(data) != manifest || before != pid() {
		t.Fatalf("MCP did not sync the skill in the original process: %v", err)
	}
}
