package taskmeta

import "testing"

func TestSetOmitsEmptyTaskContext(t *testing.T) {
	metadata := Set(map[string]any{"kind": "message"}, "", 0)
	if _, ok := metadata[IDKey]; ok {
		t.Fatal("empty task_id was persisted")
	}
	if _, ok := metadata[AttemptKey]; ok {
		t.Fatal("empty task_attempt was persisted")
	}
}

func TestSetAndReadTaskContext(t *testing.T) {
	metadata := Set(nil, " task-7 ", 2)
	if ID(metadata) != "task-7" || Attempt(metadata) != 2 {
		t.Fatalf("task metadata = %#v", metadata)
	}
}
