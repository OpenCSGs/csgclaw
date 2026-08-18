package dockercli

import (
	"testing"
	"time"

	"csgclaw/internal/sandbox"
)

func TestParseInspectDockerJSON(t *testing.T) {
	data := []byte(`[{"Id":"abc123","Name":"/agent-one","Created":"2026-04-18T07:31:25.471080Z","State":{"Status":"running","Running":true}}]`)
	info, err := parseInspect(data)
	if err != nil {
		t.Fatalf("parseInspect() error = %v", err)
	}
	if info.ID != "abc123" {
		t.Fatalf("ID = %q", info.ID)
	}
	if info.Name != "agent-one" {
		t.Fatalf("Name = %q", info.Name)
	}
	if info.State != sandbox.StateRunning {
		t.Fatalf("State = %q", info.State)
	}
	wantCreated, err := time.Parse(time.RFC3339Nano, "2026-04-18T07:31:25.471080Z")
	if err != nil {
		t.Fatalf("Parse want time: %v", err)
	}
	if !info.CreatedAt.Equal(wantCreated) {
		t.Fatalf("CreatedAt = %v, want %v", info.CreatedAt, wantCreated)
	}
}

func TestParseInspectEmptyArrayNotFound(t *testing.T) {
	_, err := parseInspect([]byte(`[]`))
	if !sandbox.IsNotFound(err) {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestParseInspectPreservesDockerLifecycleState(t *testing.T) {
	tests := []struct {
		status string
		want   sandbox.State
	}{
		{status: "created", want: sandbox.StateCreated},
		{status: "running", want: sandbox.StateRunning},
		{status: "paused", want: sandbox.StatePaused},
		{status: "restarting", want: sandbox.StateRestarting},
		{status: "removing", want: sandbox.StateRemoving},
		{status: "exited", want: sandbox.StateExited},
		{status: "dead", want: sandbox.StateDead},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			data := []byte(`[{"Id":"abc123","Name":"/agent-one","State":{"Status":"` + tt.status + `"}}]`)
			info, err := parseInspect(data)
			if err != nil {
				t.Fatalf("parseInspect() error = %v", err)
			}
			if info.State != tt.want {
				t.Fatalf("State = %q, want %q", info.State, tt.want)
			}
		})
	}
}
