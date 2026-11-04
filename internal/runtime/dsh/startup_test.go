package dsh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
)

func TestColdACPInitializationHonorsCallerBudget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX launcher")
	}
	for _, tc := range []struct {
		name, delay string
		timeout     time.Duration
		cancelled   bool
	}{
		{name: "cold startup", delay: "16s", timeout: 30 * time.Second},
		{name: "caller deadline", delay: "1m", timeout: 200 * time.Millisecond, cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			launcher := filepath.Join(root, "dsh-test")
			if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexec \"$DSH_TEST_BINARY\" -test.run=TestDSHHelperProcess\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GO_WANT_DSH_HELPER_PROCESS", "1")
			t.Setenv("DSH_TEST_BINARY", os.Args[0])
			t.Setenv("DSH_TEST_INITIALIZE_DELAY", tc.delay)
			profile := agentruntime.Profile{BaseURL: "https://gateway.example/v1", APIKey: "fixture-key", ModelID: "test-model"}
			rt := New(Dependencies{ResolveBinary: func(context.Context) (dshcli.Info, error) {
				return dshcli.Info{Path: launcher, Version: "0.1.5-rc.3"}, nil
			}, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
				return AgentRef{ID: "test", RuntimeID: "cold-start", Profile: profile}, nil
			}, AgentHome: func(string) (string, error) { return filepath.Join(root, "agent"), nil }})
			t.Cleanup(func() { _ = rt.Close() })
			if err := rt.Provision(t.Context(), agentruntime.ProvisionRequest{RuntimeID: "cold-start", AgentID: "test", Profile: profile}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), tc.timeout)
			defer cancel()
			started := time.Now()
			_, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "cold-start", AgentID: "test", Profile: profile})
			if tc.cancelled {
				if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 5*time.Second {
					t.Fatalf("caller cancellation was not honored: %v", err)
				}
			} else if err != nil {
				t.Fatalf("valid cold startup was rejected: %v", err)
			}
		})
	}
}
