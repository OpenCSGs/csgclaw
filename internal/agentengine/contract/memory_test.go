package contract

import (
	"strings"
	"testing"
)

func TestMemoryOwnershipIsPresentWithoutSavedEntries(t *testing.T) {
	for _, memory := range []*RuntimeMemory{{}, {Binding: "scope"}, {Binding: "scope", Text: "User prefers Chinese."}} {
		text := MemoryReference(memory)
		for _, rule := range []string{"CSGClaw exclusively manages", "do not create, read, or update USER.md", "override workspace startup instructions", "disabled or unavailable"} {
			if !strings.Contains(text, rule) {
				t.Fatalf("missing ownership rule %q: %s", rule, text)
			}
		}
		if strings.Contains(text, `type="agent_memory" trust="untrusted"`) != (memory.Text != "") {
			t.Fatalf("memory data and ownership policy must remain separate: %s", text)
		}
		if strings.Contains(text, "disabled or unavailable for this turn") != (memory.Binding == "") {
			t.Fatalf("an empty enabled store must not be confused with disabled memory: %s", text)
		}
	}
	if MemoryReference(nil) != "" {
		t.Fatal("requests outside managed memory unexpectedly changed")
	}
}

func TestMemoryOwnershipUpgradeStartsFreshNativeSession(t *testing.T) {
	request := TurnRequest{ConversationKey: "room:private", RuntimeMemory: &RuntimeMemory{Binding: "same-content"}}
	key := RuntimeSessionKey(request)
	if key == "csgclaw-memory:same-content:room:private" || LogicalRuntimeSessionKey(key) != string(request.ConversationKey) {
		t.Fatalf("ownership upgrade did not rotate native session while retaining logical conversation: %s", key)
	}
	if LogicalRuntimeSessionKey("csgclaw-memory:same-content:room:private") != string(request.ConversationKey) {
		t.Fatal("old native bindings must still be recognized for cleanup")
	}
}
