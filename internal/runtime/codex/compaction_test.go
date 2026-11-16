package codex

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelcap"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestManualCompactionResumeFailurePreservesOriginalBinding(t *testing.T) {
	withAppServerHelperCommand(t, "resume-fallback")
	spec := testAppServerSessionSpec(t.TempDir())
	key := "csgclaw-memory:managed-v2-owner:room:test"
	spec.ConversationSessions = map[string]string{key: "saved-history-thread"}
	spec.FilePublishingConversations = map[string]bool{key: true}
	savedPath := filepath.Join(t.TempDir(), "saved-bindings.json")
	initial, _ := json.Marshal(spec.ConversationSessions)
	if err := os.WriteFile(savedPath, initial, 0600); err != nil {
		t.Fatal(err)
	}
	changes := 0
	deps := testAppServerManagerDeps()
	deps.OnConversationSessionsChange = func(_ *Session, bindings map[string]string, _ map[string]bool) error {
		changes++
		data, _ := json.Marshal(bindings)
		return os.WriteFile(savedPath, data, 0600)
	}
	manager := newAppServerManager(deps)
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
	before, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatal(err)
	}
	initialChanges := changes
	runtime := New(Dependencies{Manager: manager})
	used := int64(50000)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := runtime.CompactConversation(ctx, spec.RuntimeID, contract.CompactRequest{ConversationKey: "room:test", Usage: modelcap.ContextUsage{SessionID: "saved-history-thread", ModelID: spec.Profile.ModelID, UsedTokens: &used, ContextWindow: 100000}})
	if err != nil || result.Status != "unavailable" {
		t.Fatalf("resume failure result: %+v %v", result, err)
	}
	live := manager.liveSession(spec.RuntimeID)
	live.mu.Lock()
	mapping := cloneConversationSessions(live.conversationSessions)
	loaded := live.loadedConversations[key]
	live.mu.Unlock()
	if mapping[key] != "saved-history-thread" || loaded || changes != initialChanges {
		t.Fatalf("manual compaction replaced original history: mapping=%v loaded=%v persistence calls=%d", mapping, loaded, changes-initialChanges)
	}
	after, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed compaction rewrote session metadata")
	}
}

func TestManualCompactionStrictResumeChecksSavedThreadIdentity(t *testing.T) {
	for _, test := range []struct {
		name, saved string
		wantError   bool
	}{{"same thread without file tools", "resumed-thread", false}, {"different returned thread", "original-thread", true}} {
		t.Run(test.name, func(t *testing.T) {
			withAppServerHelperCommand(t, "resume-success")
			spec := testAppServerSessionSpec(t.TempDir())
			spec.ConversationSessions = map[string]string{"room:test": test.saved}
			manager := newAppServerManager(testAppServerManagerDeps())
			if _, err := manager.Start(context.Background(), spec); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
			live := manager.liveSession(spec.RuntimeID)
			err := manager.resumeCompactionThread(context.Background(), live, "room:test", test.saved)
			if (err != nil) != test.wantError {
				t.Fatalf("strict resume: %v", err)
			}
			live.mu.Lock()
			defer live.mu.Unlock()
			if live.conversationSessions["room:test"] != test.saved || live.loadedConversations["room:test"] == test.wantError || live.filePublishingThreads[test.saved] {
				t.Fatalf("strict resume changed binding/capabilities: %v", live.conversationSessions)
			}
		})
	}
}
