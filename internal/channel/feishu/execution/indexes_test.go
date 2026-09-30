package execution

import (
	"context"
	"testing"

	"csgclaw/internal/agentengine"
	state "csgclaw/internal/channel/feishu/state"
)

func TestConversationIndexesReleaseOnlyIdleEntries(t *testing.T) {
	r, _ := NewRunner(RunnerOptions{Engine: fakeEngine{&fakeConversation{}}, State: state.NewStore()})
	for _, key := range []string{"idle", "running", "admitting", "question"} {
		r.latest[key] = "turn"
		r.workerContexts[key] = context.Background()
	}
	run := &activeRun{key: "running"}
	r.runs[run] = struct{}{}
	release, err := r.acquireControl(context.Background(), "admitting")
	if err != nil {
		t.Fatal(err)
	}
	message := runnerMessage("e", "turn", "question", "question")
	r.interactions["question"] = &pendingInteraction{message: message, request: agentengine.InteractionRequest{Detached: true}}
	r.pruneConversationIndexes()
	if _, ok := r.latest["idle"]; ok {
		t.Fatal("idle index retained")
	}
	for _, key := range []string{"running", "admitting", "question"} {
		if r.latest[key] != "turn" || r.workerContexts[key] == nil {
			t.Fatalf("live index %s removed", key)
		}
	}
	release()
	delete(r.runs, run)
	r.interactions["question"].finished = true
	r.pruneConversationIndexes()
	if len(r.latest) != 0 || len(r.workerContexts) != 0 {
		t.Fatal("terminal indexes retained")
	}
}

func TestOldInteractionDoesNotPinNewerConversationIndex(t *testing.T) {
	r, _ := NewRunner(RunnerOptions{Engine: fakeEngine{&fakeConversation{}}, State: state.NewStore()})
	r.latest["chat"] = "new"
	r.workerContexts["chat"] = context.Background()
	r.interactions["old"] = &pendingInteraction{message: runnerMessage("e", "old", "chat", "old")}
	r.pruneConversationIndexes()
	if len(r.latest) != 0 || len(r.workerContexts) != 0 {
		t.Fatal("stale question pinned newer completed turn")
	}
}
