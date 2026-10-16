package apps

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func (r *cliRuntimeStub) RestoreLegacy(_ context.Context, id string, _ json.RawMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configured[id] = nil
	r.events = append(r.events, "restore:"+id)
	return nil
}
func (r *cliRuntimeStub) Verify(_ context.Context, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[id] && r.configured[id] != nil, nil
}
func migrationInput(t *testing.T) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(name, []byte(`{"apps":[{"name":"csgclaw","appId":"cli_test","appSecret":{"source":"keychain"}}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}
func TestFeishuCLIMigrationPreservesRunningIntentAndSelection(t *testing.T) {
	s, f, _ := newCLIFixture(t)
	ctx := context.Background()
	source := migrationInput(t)
	f.runtime.running["running"] = true
	candidates := []LegacyWorker{{AgentID: "running", Running: true, ConfigFile: source}, {AgentID: "stopped", ConfigFile: source}}
	if err := s.PrepareCLIMigration(ctx, candidates); err != nil {
		t.Fatal(err)
	}
	original := s.migrationSnapshot()
	if original.Selected == "" || original.SourceDigest == "" {
		t.Fatal("selection was not persisted")
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := s.PrepareCLIMigration(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got := s.migrationSnapshot(); got.Selected != original.Selected || got.SourceDigest != original.SourceDigest {
		t.Fatal("selection changed")
	}
	if err := s.executeMigration(ctx); err != nil {
		t.Fatal(err)
	}
	got := s.migrationSnapshot()
	if got.Status != "completed" || !got.Workers["stopped"].VerificationPending {
		t.Fatalf("migration=%+v", got)
	}
	f.runtime.mu.Lock()
	defer f.runtime.mu.Unlock()
	if !f.runtime.running["running"] || f.runtime.running["stopped"] {
		t.Fatalf("running=%v", f.runtime.running)
	}
	a, b := f.runtime.configured["running"], f.runtime.configured["stopped"]
	if a == nil || b == nil || a.ResourceID != b.ResourceID || a.ConfigDir != b.ConfigDir {
		t.Fatalf("projections=%v", f.runtime.configured)
	}
}
func TestFeishuCLIMigrationStopFailureBlocksConfiguration(t *testing.T) {
	s, f, _ := newCLIFixture(t)
	ctx := context.Background()
	f.runtime.running["worker"] = true
	f.runtime.fail = "stop:worker"
	if err := s.PrepareCLIMigration(ctx, []LegacyWorker{{AgentID: "worker", Running: true, ConfigFile: migrationInput(t)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.executeMigration(ctx); err == nil {
		t.Fatal("expected stop failure")
	}
	if len(f.calls) != 0 {
		t.Fatalf("CLI commands=%v", f.calls)
	}
	if got := s.migrationSnapshot(); got.Status != "waiting_for_legacy_callers" || got.Workers["worker"].Committed {
		t.Fatalf("migration=%+v", got)
	}
	if err := s.CheckCLIReady("worker"); err == nil {
		t.Fatal("startup was allowed")
	}
}
func TestFeishuCLIMigrationRestoresBeforeConfigurationCommit(t *testing.T) {
	s, f, _ := newCLIFixture(t)
	ctx := context.Background()
	f.runtime.running["worker"] = true
	f.runtime.fail = "configure:worker"
	if err := s.PrepareCLIMigration(ctx, []LegacyWorker{{AgentID: "worker", Running: true, ConfigFile: migrationInput(t)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.executeMigration(ctx); err == nil {
		t.Fatal("expected configuration failure")
	}
	got := s.migrationSnapshot().Workers["worker"]
	if got.Committed || got.Phase != "original" {
		t.Fatalf("worker=%+v", got)
	}
	if !f.runtime.running["worker"] || f.runtime.configured["worker"] != nil {
		t.Fatal("original configuration was not restored")
	}
	f.runtime.fail = ""
	if err := s.executeMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if got := s.migrationSnapshot(); got.Status != "completed" {
		t.Fatal(errors.New("migration retry did not complete"))
	}
}

func TestFeishuCLIMigrationDoesNotRecreateDeletedWorkerBinding(t *testing.T) {
	s, _, _ := newCLIFixture(t)
	ctx := context.Background()
	if err := s.PrepareCLIMigration(ctx, []LegacyWorker{{AgentID: "worker", ConfigFile: migrationInput(t)}}); err != nil {
		t.Fatal(err)
	}
	s.cliWorkMu.Lock()
	defer s.cliWorkMu.Unlock()
	migration := s.migrationSnapshot()
	bindingID := migration.Workers["worker"].BindingID
	if err := s.DeleteAgent(ctx, "worker"); err != nil {
		t.Fatal(err)
	}
	if err := s.ensureMigrationBinding("worker", bindingID, migration.ResourceID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ensure binding error=%v", err)
	}
	if _, err := s.Get(ctx, "worker", bindingID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("binding lookup error=%v", err)
	}
}

func TestMigrationViewWithNoWorkersReturnsEmptyArray(t *testing.T) {
	s := &Service{cliMigration: &cliMigration{ResourceID: "resource", Status: "completed"}}
	view := s.migrationViewLocked("resource")
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Workers json.RawMessage `json:"workers"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if string(decoded.Workers) != "[]" {
		t.Fatalf("workers = %s, want []", decoded.Workers)
	}
}

func TestFeishuCLIConnectRetriesFailedMigration(t *testing.T) {
	s, f, id := newCLIFixture(t)
	ctx := context.Background()
	f.runtime.running["worker"] = true
	f.runtime.fail = "stop:worker"
	if err := s.PrepareCLIMigration(ctx, []LegacyWorker{{AgentID: "worker", Running: true, ConfigFile: migrationInput(t)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.executeMigration(ctx); err == nil {
		t.Fatal("expected migration failure")
	}
	f.runtime.mu.Lock()
	f.runtime.fail = ""
	f.runtime.mu.Unlock()
	op := startCLITest(t, s, id, "connect", "retry-migration")
	if got := waitCLIOperation(t, s, id, op); got.Status != "completed" {
		t.Fatalf("operation=%+v", got)
	}
	if got := s.migrationSnapshot(); got.Status != "completed" {
		t.Fatalf("migration=%+v", got)
	}
	if err := s.CheckCLIReady("worker"); err != nil {
		t.Fatal(err)
	}
}
