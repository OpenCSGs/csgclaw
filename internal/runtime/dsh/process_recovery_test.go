package dsh

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/processgroup"
	agentruntime "csgclaw/internal/runtime"
)

func TestDSHRecoveryProcessHelper(t *testing.T) {
	switch os.Getenv("CSGCLAW_DSH_RECOVERY_TEST") {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestDSHRecoveryProcessHelper$")
		child.Env = append(os.Environ(), "CSGCLAW_DSH_RECOVERY_TEST=child")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		for {
			if _, err := os.Stat(os.Getenv("CSGCLAW_DSH_RECOVERY_READY")); err == nil {
				os.Exit(0)
			}
			time.Sleep(5 * time.Millisecond)
		}
	case "child":
		if err := os.WriteFile(os.Getenv("CSGCLAW_DSH_RECOVERY_READY"), []byte("ready"), 0600); err != nil {
			os.Exit(2)
		}
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}
func TestStopCleansPersistedProcessAfterParentExited(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestDSHRecoveryProcessHelper$")
	cmd.Env = append(os.Environ(), "CSGCLAW_DSH_RECOVERY_TEST=parent", "CSGCLAW_DSH_RECOVERY_READY="+filepath.Join(root, "ready"))
	if err := processgroup.Start(cmd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = processgroup.Stop(cmd.Process.Pid, true) })
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent did not exit")
	}
	rt := New(Dependencies{})
	rt.roots["runtime-test"] = root
	if err := writeMetadata(root, runtimeMetadata{RuntimeID: "runtime-test", PID: cmd.Process.Pid, State: agentruntime.StateRunning}); err != nil {
		t.Fatal(err)
	}
	if state, err := rt.Stop(t.Context(), agentruntime.Handle{RuntimeID: "runtime-test"}); err != nil || state != agentruntime.StateStopped {
		t.Fatalf("stop=%s error=%v", state, err)
	}
	if alive, err := processgroup.Alive(cmd.Process.Pid); err != nil || alive {
		t.Fatalf("descendant alive=%v error=%v", alive, err)
	}
	meta, err := readMetadata(root)
	if err != nil || meta.PID != 0 {
		t.Fatalf("metadata=%+v error=%v", meta, err)
	}
}
