package agentengine

import (
	"path/filepath"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/agentmemory"
)

func TestMemoryDisabledPreservesUnboundConversationKey(t *testing.T) {
	for _, mode := range []string{"disabled", "local"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			store, err := agentmemory.Open(root, filepath.Join(root, "state.json"), mode)
			if err != nil {
				t.Fatal(err)
			}
			engine := &Engine{}
			engine.SetMemory(store)
			source := &contract.MemorySource{}
			if mode == "local" {
				source.UserID = "user-admin"
				doc, err := store.GetOrInitialize(agentmemory.Scope{AgentID: "agent-test", UserID: source.UserID})
				if err != nil {
					t.Fatal(err)
				}
				enabled := false
				if _, err := store.Update(agentmemory.Scope{AgentID: "agent-test", UserID: source.UserID}, agentmemory.Update{Revision: &doc.Revision, Enabled: &enabled}); err != nil {
					t.Fatal(err)
				}
			}
			c := &conversations{engine: engine, agentID: "agent-test"}
			request := TurnRequest{ConversationKey: "private-room", MemorySource: source}
			c.prepareMemory(&request)
			if key := contract.RuntimeSessionKey(request); key != "private-room" {
				t.Fatalf("disabled memory changes existing session key: %q", key)
			}
		})
	}
}
