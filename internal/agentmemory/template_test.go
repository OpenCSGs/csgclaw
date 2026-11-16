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

func TestTemplateSeedCanBeCorrectedByBackgroundLearning(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	if err := store.SeedTemplate(testScope.AgentID, "# Preferences\n- Use tabs.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrInitialize(testScope); err != nil {
		t.Fatal(err)
	}
	_, beforeBinding, _, err := store.Snapshot(testScope, "")
	if err != nil {
		t.Fatal(err)
	}
	Worker{Store: store}.process(context.Background(), testScope)
	enqueueMemory(t, store, "correction", "Use spaces instead of tabs.")
	Worker{Store: store, Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
		return []Change{{EntryID: input.Entries[0].ID, Kind: "preference", Text: "Use spaces.", SourceMessageID: "correction"}}, nil
	}}.process(context.Background(), testScope)
	doc := getMemory(t, store, testScope)
	if len(doc.Entries) != 1 || doc.Entries[0].Text != "Use spaces." || doc.Entries[0].ID == "" {
		t.Fatalf("correction did not replace seeded preference: %+v", doc)
	}
	_, afterBinding, _, err := store.Snapshot(testScope, "")
	if err != nil || beforeBinding == afterBinding {
		t.Fatalf("correction retained stale runtime memory binding: %q, %v", afterBinding, err)
	}

}
