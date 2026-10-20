package apps

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/connectorcli"
)

func failedCLIMigration(t *testing.T, failure string) (*Service, *cliFixture, string) {
	t.Helper()
	s, f, id := newCLIFixture(t)
	f.runtime.running["running"] = true
	install := s.options.CLI.Install
	switch failure {
	case "stop":
		f.runtime.fail = "stop:running"
	case "install":
		s.options.CLI.Install = func(context.Context, string, func(string)) (connectorcli.Package, error) {
			return connectorcli.Package{}, errors.New("installation unavailable")
		}
	case "configure":
		f.runtime.fail = "configure:running"
	}
	source := migrationInput(t)
	if err := s.PrepareCLIMigration(t.Context(), []LegacyWorker{
		{AgentID: "running", Running: true, ConfigFile: source},
		{AgentID: "stopped", ConfigFile: source},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreCLIMigration(t.Context()); err == nil {
		t.Fatal("expected migration failure")
	}
	s.options.CLI.Install = install
	f.runtime.mu.Lock()
	f.runtime.fail = ""
	f.runtime.mu.Unlock()
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	return s, f, id
}

func assertMigrationDisconnected(t *testing.T, s *Service, f *cliFixture, id string) {
	t.Helper()
	resource, err := s.Get(t.Context(), "", id)
	if err != nil {
		t.Fatal(err)
	}
	if !resource.Disconnected || resource.Authorized || resource.LastErrorCode != "" || resource.Migration != nil {
		t.Fatalf("resource was not disconnected: %+v", resource)
	}
	m := s.migrationSnapshot()
	if m.Status != "completed" || len(m.SourceConfig) != 0 || len(m.Workers) != 0 || m.Selected != "" {
		t.Fatalf("migration still retains recovery data: %+v", m)
	}
	if len(resource.Bindings) != 2 {
		t.Fatalf("bindings=%+v", resource.Bindings)
	}
	for _, binding := range resource.Bindings {
		if !binding.Enabled {
			t.Fatalf("binding disabled: %+v", binding)
		}
		if err := s.CheckCLIReady(binding.AgentID); err != nil {
			t.Fatal(err)
		}
	}
	f.runtime.mu.Lock()
	defer f.runtime.mu.Unlock()
	if !f.runtime.running["running"] || f.runtime.running["stopped"] {
		t.Fatalf("worker running intent changed: %v", f.runtime.running)
	}
	for _, worker := range []string{"running", "stopped"} {
		if f.runtime.configured[worker] != nil {
			t.Fatalf("worker %s retained its extension", worker)
		}
	}
}

func TestCLIDisconnectFailedMigrationThenReconnect(t *testing.T) {
	for _, failure := range []string{"stop", "install", "configure"} {
		t.Run(failure, func(t *testing.T) {
			s, f, id := failedCLIMigration(t, failure)
			original := s.migrationSnapshot()
			backup := filepath.Join(s.dataRoot, id, "migration", "recovery.preparing")
			if err := os.MkdirAll(backup, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(backup, "config.json"), []byte("recovery data"), 0600); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			f.calls = nil
			f.mu.Unlock()
			op := waitCLIOperation(t, s, id, startCLITest(t, s, id, "disconnect", "disconnect"))
			if op.Status != "completed" {
				t.Fatalf("disconnect=%+v", op)
			}
			assertMigrationDisconnected(t, s, f, id)
			if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("recovery files remain: %v", err)
			}
			for worker, old := range original.Workers {
				if _, err := s.Get(t.Context(), worker, old.BindingID); err != nil {
					t.Fatalf("binding identity changed: %v", err)
				}
			}
			f.mu.Lock()
			for _, call := range f.calls {
				if strings.HasPrefix(call, "config init") || strings.HasPrefix(call, "auth login") {
					t.Errorf("disconnect imported authorization: %s", call)
				}
			}
			f.calls = nil
			f.mu.Unlock()
			op = waitCLIOperation(t, s, id, startCLITest(t, s, id, "connect", "reconnect"))
			if op.Status != "completed" {
				t.Fatalf("reconnect=%+v", op)
			}
			f.mu.Lock()
			initialized := false
			for _, call := range f.calls {
				initialized = initialized || strings.HasPrefix(call, "config init --new")
			}
			f.mu.Unlock()
			if !initialized {
				t.Fatal("reconnection reused the old application configuration")
			}
			f.runtime.mu.Lock()
			defer f.runtime.mu.Unlock()
			for _, worker := range []string{"running", "stopped"} {
				if f.runtime.configured[worker] == nil || f.runtime.configured[worker].ResourceID != id {
					t.Fatalf("worker %s was not reconnected", worker)
				}
			}
			if !f.runtime.running["running"] || f.runtime.running["stopped"] {
				t.Fatalf("reconnection changed worker running intent: %v", f.runtime.running)
			}
		})
	}
}

func TestCLIMigrationDisconnectFailureCanRetry(t *testing.T) {
	for _, failure := range []string{"stop", "logout", "configure"} {
		t.Run(failure, func(t *testing.T) {
			s, f, id := failedCLIMigration(t, "configure")
			f.runtime.mu.Lock()
			if failure == "stop" {
				f.runtime.fail = "stop:running"
			} else if failure == "configure" {
				f.runtime.fail = "configure:running"
			}
			f.runtime.mu.Unlock()
			if failure == "logout" {
				s.options.CLI.Run = func(ctx context.Context, c connectorcli.Client, in io.Reader, args ...string) ([]byte, error) {
					if strings.Join(args, " ") == "auth logout" {
						return nil, errors.New("logout unavailable")
					}
					return f.run(ctx, c, in, args...)
				}
			}
			op := waitCLIOperation(t, s, id, startCLITest(t, s, id, "disconnect", "failed-disconnect"))
			if op.Status != "failed" || op.ErrorCode != "cleanup_pending" {
				t.Fatalf("operation=%+v", op)
			}
			wantStage := map[string]string{"stop": "stopping_workers", "logout": "revoking_authorization", "configure": "applying"}[failure]
			if op.FailureStage != wantStage {
				t.Fatalf("failure stage=%q, want %q", op.FailureStage, wantStage)
			}
			if s.migrationSnapshot().Status != "disconnecting" || s.CheckCLIReady("running") == nil {
				t.Fatal("unfinished disconnect allowed worker startup")
			}
			resource, err := s.Get(t.Context(), "", id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartCLI(t.Context(), id, CLIRequest{Action: "connect", OperationID: "early-connect", Revision: resource.Revision}); !errors.Is(err, ErrConflict) {
				t.Fatalf("connect during cleanup: %v", err)
			}
			f.runtime.mu.Lock()
			f.runtime.fail = ""
			f.runtime.mu.Unlock()
			s.options.CLI.Run = f.run
			op = waitCLIOperation(t, s, id, startCLITest(t, s, id, "disconnect", "retry-disconnect"))
			if op.Status != "completed" {
				t.Fatalf("retry=%+v", op)
			}
			assertMigrationDisconnected(t, s, f, id)
		})
	}
}

func TestCLIMigrationDisconnectResumesAfterRestart(t *testing.T) {
	s, f, id := failedCLIMigration(t, "configure")
	applying := make(chan struct{})
	var once sync.Once
	f.runtime.mu.Lock()
	f.runtime.beforeApply = func(ctx context.Context) error {
		once.Do(func() { close(applying) })
		<-ctx.Done()
		return ctx.Err()
	}
	f.runtime.mu.Unlock()
	op := startCLITest(t, s, id, "disconnect", "interrupted-disconnect")
	select {
	case <-applying:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect did not reach worker configuration")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	f.runtime.mu.Lock()
	f.runtime.beforeApply = nil
	f.runtime.mu.Unlock()
	restored, err := NewService(s.path, s.options)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreCLIMigration(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := restored.RestoreAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := waitCLIOperation(t, restored, id, op); got.Status != "completed" {
		t.Fatalf("restored operation=%+v", got)
	}
	assertMigrationDisconnected(t, restored, f, id)
}

func TestCLIMigrationDisconnectCancellationFinishesCommittedCleanup(t *testing.T) {
	s, f, id := failedCLIMigration(t, "configure")
	applying := make(chan struct{})
	var once sync.Once
	f.runtime.mu.Lock()
	f.runtime.beforeApply = func(ctx context.Context) (err error) {
		once.Do(func() {
			close(applying)
			<-ctx.Done()
			err = ctx.Err()
		})
		return err
	}
	f.runtime.mu.Unlock()
	op := startCLITest(t, s, id, "disconnect", "cancel-disconnect")
	select {
	case <-applying:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect did not reach worker configuration")
	}
	if _, err := s.CancelCLI(t.Context(), id, op.SessionID); err != nil {
		t.Fatal(err)
	}
	if got := waitCLIOperation(t, s, id, op); got.Status != "cancelled" {
		t.Fatalf("cancelled operation=%+v", got)
	}
	assertMigrationDisconnected(t, s, f, id)
}

func TestCLIMigrationDisconnectPreservesDisabledBinding(t *testing.T) {
	s, f, id := failedCLIMigration(t, "configure")
	s.cliWorkMu.Lock()
	w := s.migrationSnapshot().Workers["running"]
	s.mu.Lock()
	e := s.entries[w.BindingID]
	e.record.Enabled = false
	err := s.persistLocked(w.BindingID, &e.record)
	s.mu.Unlock()
	s.cliWorkMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"disconnect", "connect"} {
		if op := waitCLIOperation(t, s, id, startCLITest(t, s, id, action, action)); op.Status != "completed" {
			t.Fatalf("%s: %+v", action, op)
		}
		binding, err := s.Get(t.Context(), "running", w.BindingID)
		if err != nil || binding.Enabled {
			t.Fatalf("disabled binding after %s: %+v %v", action, binding, err)
		}
		f.runtime.mu.Lock()
		running, configured := f.runtime.running["running"], f.runtime.configured["running"]
		f.runtime.mu.Unlock()
		if !running || configured != nil {
			t.Fatalf("disabled worker after %s: running=%v configured=%v", action, running, configured)
		}
	}
}

func TestCLIMigrationDisconnectRecoversUnrecordedExecutable(t *testing.T) {
	s, f, id := failedCLIMigration(t, "configure")
	if err := s.editCLI(id, func(r *record) error {
		r.CLI.Executable = ""
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
	if op := waitCLIOperation(t, s, id, startCLITest(t, s, id, "disconnect", "disconnect")); op.Status != "completed" {
		t.Fatal(op)
	}
	assertMigrationDisconnected(t, s, f, id)
	f.mu.Lock()
	defer f.mu.Unlock()
	logout := false
	for _, call := range f.calls {
		logout = logout || call == "auth logout"
	}
	if !logout {
		t.Fatal("disconnect skipped imported credentials")
	}
	r, err := s.cliSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.CLI.ConfigDir, "config.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("imported configuration remains: %v", err)
	}
}

func TestCLIMigrationDisconnectPendingAtStartupSkipsImport(t *testing.T) {
	s, f, id := failedCLIMigration(t, "install")
	s.cliWorkMu.Lock()
	op := startCLITest(t, s, id, "disconnect", "pending-disconnect")
	s.cancel()
	s.cliWorkMu.Unlock()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := NewService(s.path, s.options)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreCLIMigration(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := restored.RestoreAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := waitCLIOperation(t, restored, id, op); got.Status != "completed" {
		t.Fatal(got)
	}
	assertMigrationDisconnected(t, restored, f, id)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 0 {
		t.Fatalf("startup imported old authorization: %v", f.calls)
	}
}
