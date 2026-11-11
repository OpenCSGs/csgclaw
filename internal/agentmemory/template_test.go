package agentmemory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateSeedIsAdoptedOnceAndSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	store := openTestStore(t, root, "local")
	if err := store.SeedTemplate(testScope.AgentID, "# Preferences\n- Use concise reports.\n"); err != nil {
		t.Fatal(err)
	}
	first, err := store.GetOrInitialize(testScope)
	if err != nil || len(first.Entries) != 1 || first.Entries[0].Kind != "preference" {
		t.Fatalf("seed: %+v %v", first, err)
	}
	store = openTestStore(t, root, "local")
	again, err := store.GetOrInitialize(testScope)
	if err != nil || again.Entries[0].ID != first.Entries[0].ID {
		t.Fatalf("seed replayed on restart: %+v %v", again, err)
	}
	Worker{Store: store}.process(context.Background(), testScope)
	doc := getMemory(t, store, testScope)
	reset, err := store.Update(testScope, Update{Revision: &doc.Revision, Reset: true})
	if err != nil || len(reset.Entries) != 0 {
		t.Fatalf("reset: %+v %v", reset, err)
	}
	_, _, text, err := store.Snapshot(testScope, "")
	if err != nil || strings.Contains(text, "concise") {
		t.Fatalf("reset revived template: %q %v", text, err)
	}
	disabled, err := Open(root, filepath.Join(root, "global-state.json"), "disabled")
	if err != nil {
		t.Fatal(err)
	}
	other := testScope
	other.UserID = "user-other"
	off, err := disabled.GetOrInitialize(other)
	if err != nil || off.Ready || off.Enabled {
		t.Fatalf("disabled scope adopted template: %+v %v", off, err)
	}
}
