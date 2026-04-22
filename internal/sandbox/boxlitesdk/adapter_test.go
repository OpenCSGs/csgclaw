package boxlitesdk

import (
	"context"
	"errors"
	"testing"
	"time"

	boxlitesdk "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/sandbox"
)

func TestProviderImplementsSandboxProvider(t *testing.T) {
	var _ sandbox.Provider = NewProvider()
	if got, want := NewProvider().Name(), "boxlite-sdk"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

func TestBoxOptionsValidateSpec(t *testing.T) {
	_, err := boxOptions(sandbox.CreateSpec{
		Env: map[string]string{"": "value"},
	})
	if err == nil {
		t.Fatal("empty env key should fail")
	}

	_, err = boxOptions(sandbox.CreateSpec{
		Mounts: []sandbox.Mount{{HostPath: "/host"}},
	})
	if err == nil {
		t.Fatal("empty mount guest path should fail")
	}
}

func TestBoxOptionsAcceptSupportedSpec(t *testing.T) {
	opts, err := boxOptions(sandbox.CreateSpec{
		Name:       "agent",
		Detach:     true,
		AutoRemove: false,
		Env:        map[string]string{"HOME": "/home/picoclaw"},
		Mounts: []sandbox.Mount{
			{HostPath: "/host/rw", GuestPath: "/guest/rw"},
			{HostPath: "/host/ro", GuestPath: "/guest/ro", ReadOnly: true},
		},
		Entrypoint: []string{"/bin/sh"},
		Cmd:        []string{"-c", "echo ok"},
	})
	if err != nil {
		t.Fatalf("boxOptions() error = %v", err)
	}
	if len(opts) == 0 {
		t.Fatal("boxOptions() returned no options")
	}
}

func TestBoxStateMapping(t *testing.T) {
	tests := []struct {
		name  string
		state boxlitesdk.State
		want  sandbox.State
	}{
		{name: "configured", state: boxlitesdk.StateConfigured, want: sandbox.StateCreated},
		{name: "running", state: boxlitesdk.StateRunning, want: sandbox.StateRunning},
		{name: "stopping", state: boxlitesdk.StateStopping, want: sandbox.StateUnknown},
		{name: "stopped", state: boxlitesdk.StateStopped, want: sandbox.StateStopped},
		{name: "unknown", state: boxlitesdk.State("other"), want: sandbox.StateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boxState(tt.state); got != tt.want {
				t.Fatalf("boxState(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestBoxInfoMapping(t *testing.T) {
	createdAt := time.Unix(123, 0)
	got := boxInfo(boxlitesdk.BoxInfo{
		ID:        "box-id",
		Name:      "agent",
		State:     boxlitesdk.StateRunning,
		CreatedAt: createdAt,
	})
	if got.ID != "box-id" || got.Name != "agent" || got.State != sandbox.StateRunning || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("boxInfo() = %#v", got)
	}
}

func TestWrapErrorMapsNotFound(t *testing.T) {
	err := wrapError("get", &boxlitesdk.Error{Code: boxlitesdk.ErrNotFound, Message: "missing"})
	if !sandbox.IsNotFound(err) {
		t.Fatalf("wrapped error should be sandbox not found: %v", err)
	}
	if !errors.Is(err, sandbox.ErrNotFound) {
		t.Fatalf("wrapped error should match ErrNotFound: %v", err)
	}
}

func TestWrapErrorPreservesOtherErrors(t *testing.T) {
	err := wrapError("get", errors.New("boom"))
	if err == nil {
		t.Fatal("wrapError() returned nil")
	}
	if sandbox.IsNotFound(err) {
		t.Fatalf("non-not-found error should not map to ErrNotFound: %v", err)
	}
}

func TestNilHandles(t *testing.T) {
	ctx := context.Background()
	rt := (*Runtime)(nil)
	if _, err := rt.Create(ctx, sandbox.CreateSpec{}); err == nil {
		t.Fatal("nil runtime Create should fail")
	}
	if _, err := rt.Get(ctx, "box"); err == nil {
		t.Fatal("nil runtime Get should fail")
	}
	if err := rt.Remove(ctx, "box", sandbox.RemoveOptions{Force: true}); err == nil {
		t.Fatal("nil runtime Remove should fail")
	}

	inst := (*Instance)(nil)
	if err := inst.Start(ctx); err == nil {
		t.Fatal("nil instance Start should fail")
	}
	if err := inst.Stop(ctx, sandbox.StopOptions{}); err == nil {
		t.Fatal("nil instance Stop should fail")
	}
	if _, err := inst.Info(ctx); err == nil {
		t.Fatal("nil instance Info should fail")
	}
	if _, err := inst.Run(ctx, sandbox.CommandSpec{}); err == nil {
		t.Fatal("nil instance Run should fail")
	}
}
