package apps

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/connectorcli"
	"csgclaw/internal/larkcli"
	skill "csgclaw/internal/skill/state"
)

type cliRuntimeStub struct {
	mu          sync.Mutex
	running     map[string]bool
	configured  map[string]*CLIProjection
	fail        string
	events      []string
	beforeApply func(context.Context) error
}

func (r *cliRuntimeStub) State(_ context.Context, id, kind string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[id], nil
}
func (r *cliRuntimeStub) Update(ctx context.Context, id string, resume bool, fn func(context.Context) error) error {
	r.mu.Lock()
	r.events = append(r.events, "stop:"+id)
	if r.fail == "stop:"+id {
		r.mu.Unlock()
		return errors.New("injected stop failure")
	}
	r.running[id] = false
	beforeApply := r.beforeApply
	r.mu.Unlock()
	if fn != nil {
		if beforeApply != nil {
			if err := beforeApply(ctx); err != nil {
				return err
			}
		}
		if err := fn(ctx); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail == "start:"+id && resume {
		return errors.New("injected start failure")
	}
	r.running[id] = resume
	if resume {
		r.events = append(r.events, "start:"+id)
	}
	return nil
}

func (r *cliRuntimeStub) Configure(_ context.Context, id, resourceID string, p *CLIProjection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running[id] {
		return errors.New("configuration changed while running")
	}
	if r.fail == "configure:"+id {
		return errors.New("injected configuration failure")
	}
	r.configured[id] = p
	return nil
}
func (r *cliRuntimeStub) PlanSkills(string, string, string, []skill.ManagedSkill, bool) (skill.SkillPlan, error) {
	return skill.SkillPlan{}, nil
}
func (r *cliRuntimeStub) ApplySkills(context.Context, string, skill.SkillPlan) error { return nil }

type cliFixture struct {
	mu      sync.Mutex
	user    bool
	calls   []string
	runtime *cliRuntimeStub
	wait    func(context.Context) error
}

func (f *cliFixture) run(ctx context.Context, c connectorcli.Client, _ io.Reader, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, strings.Join(args, " "))
	f.mu.Unlock()
	f.runtime.mu.Lock()
	for id, running := range f.runtime.running {
		if running && f.runtime.configured[id] != nil {
			f.runtime.mu.Unlock()
			return nil, errors.New("CLI command ran before workers stopped")
		}
	}
	f.runtime.mu.Unlock()
	switch strings.Join(args[:2], " ") {
	case "config init":
		return nil, larkcli.AtomicWrite(filepath.Join(c.ConfigDir, "config.json"), []byte(`{"apps":[{"appId":"cli_test"}]}`))
	case "config remove":
		return nil, os.Remove(filepath.Join(c.ConfigDir, "config.json"))
	case "auth status":
		if _, err := os.Stat(filepath.Join(c.ConfigDir, "config.json")); err != nil {
			return []byte(`{"error":{"subtype":"not_configured"}}`), err
		}
		verified := true
		s := larkcli.Status{AppID: "cli_test"}
		s.Identities.Bot = larkcli.Identity{Available: true, Verified: &verified}
		f.mu.Lock()
		user := f.user
		f.mu.Unlock()
		if user {
			s.Identities.User = larkcli.Identity{Available: true, Verified: &verified, OpenID: "ou_test", Scope: "test:scope"}
		}
		return json.Marshal(s)
	case "auth login":
		if args[2] == "--no-wait" {
			return []byte(`{"verification_url":"https://open.feishu.cn/device","device_code":"private-code","expires_in":60}`), nil
		}
		if f.wait != nil {
			if err := f.wait(ctx); err != nil {
				return nil, err
			}
		}
		f.mu.Lock()
		f.user = true
		f.mu.Unlock()
		return nil, nil
	case "auth logout":
		f.mu.Lock()
		f.user = false
		f.mu.Unlock()
		return nil, nil
	}
	return []byte(`{}`), nil
}
func newCLIFixture(t *testing.T) (*Service, *cliFixture, string) {
	t.Helper()
	rt := &cliRuntimeStub{running: map[string]bool{}, configured: map[string]*CLIProjection{}}
	f := &cliFixture{runtime: rt}
	path := filepath.Join(t.TempDir(), "state.json")
	options := CLIOptions{Run: f.run, Install: func(_ context.Context, root string, _ func(string)) (connectorcli.Package, error) {
		return connectorcli.Package{Executable: filepath.Join(root, "bin", "lark-cli"), SkillsDir: filepath.Join(root, "skills"), Version: "1.0.97"}, nil
	}}
	s, err := NewService(path, Options{Runtime: rt, CLI: &options})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	resource, err := s.EnsureFeishuCLI(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s, f, resource.InstallationID
}
func waitCLIOperation(t *testing.T, s *Service, id string, op CLIOperation) CLIOperation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, err := s.CLISession(context.Background(), id, op.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status != "running" && current.Status != "pending" {
			return current
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("operation timeout")
	return CLIOperation{}
}
func startCLITest(t *testing.T, s *Service, id, action, operation string) CLIOperation {
	t.Helper()
	resource, err := s.Get(context.Background(), "", id)
	if err != nil {
		t.Fatal(err)
	}
	op, err := s.StartCLI(context.Background(), id, CLIRequest{OperationID: operation, Revision: resource.Revision, Action: action})
	if err != nil {
		t.Fatal(err)
	}
	return op
}
func waitCLIBinding(t *testing.T, s *Service, id, worker string) Installation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		item, err := s.Get(context.Background(), worker, id)
		if err != nil {
			t.Fatal(err)
		}
		if item.Status != "applying" {
			return item
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("binding timeout")
	return Installation{}
}
func TestFeishuCLIGlobalConnectAndWorkerLifecycle(t *testing.T) {
	s, f, id := newCLIFixture(t)
	ctx := context.Background()
	binding, err := s.Bind(ctx, "worker", BindRequest{ResourceID: id})
	if err != nil {
		t.Fatal(err)
	}
	if got := waitCLIBinding(t, s, binding.InstallationID, "worker"); got.Status != "needs_configuration" {
		t.Fatalf("unconfigured=%+v", got)
	}
	f.runtime.mu.Lock()
	f.runtime.running["worker"] = true
	f.runtime.mu.Unlock()
	op := startCLITest(t, s, id, "connect", "connect-1")
	if got := waitCLIOperation(t, s, id, op); got.Status != "completed" {
		t.Fatalf("connect=%+v", got)
	}
	item, err := s.Get(ctx, "worker", binding.InstallationID)
	if err != nil || item.Status != "connected" || item.AppliedRevision != item.Revision {
		t.Fatalf("binding=%+v err=%v", item, err)
	}
	f.runtime.mu.Lock()
	if !f.runtime.running["worker"] || f.runtime.configured["worker"] == nil {
		t.Error("worker was not resumed with shared configuration")
	}
	f.runtime.mu.Unlock()
	duplicate, err := s.StartCLI(ctx, id, CLIRequest{OperationID: "connect-1", Revision: op.Revision, Action: "connect"})
	if err != nil || duplicate.SessionID != op.SessionID {
		t.Fatalf("duplicate=%+v err=%v", duplicate, err)
	}
	op = startCLITest(t, s, id, "disconnect", "disconnect-1")
	if got := waitCLIOperation(t, s, id, op); got.Status != "completed" {
		t.Fatalf("disconnect=%+v", got)
	}
	f.mu.Lock()
	if f.user {
		t.Error("disconnect retained personal authorization")
	}
	f.mu.Unlock()
	snapshot, err := s.cliSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(snapshot.CLI.ConfigDir, "config.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disconnect retained application configuration: %v", err)
	}
	item, _ = s.Get(ctx, "worker", binding.InstallationID)
	if !item.Enabled || item.Status != "needs_configuration" {
		t.Fatalf("disconnect binding=%+v", item)
	}
	f.runtime.mu.Lock()
	if !f.runtime.running["worker"] || f.runtime.configured["worker"] != nil {
		t.Error("disconnect did not resume worker with unconfigured environment")
	}
	f.runtime.mu.Unlock()
}
func TestFeishuCLIStopFailurePreventsSharedWrite(t *testing.T) {
	s, f, id := newCLIFixture(t)
	binding, _ := s.Bind(context.Background(), "worker", BindRequest{ResourceID: id})
	waitCLIBinding(t, s, binding.InstallationID, "worker")
	f.runtime.mu.Lock()
	f.runtime.running["worker"] = true
	f.runtime.fail = "stop:worker"
	f.runtime.mu.Unlock()
	op := startCLITest(t, s, id, "connect", "stop-failure")
	if got := waitCLIOperation(t, s, id, op); got.Status != "failed" {
		t.Fatalf("operation=%+v", got)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 0 {
		t.Fatalf("CLI ran after stop failure: %v", f.calls)
	}
}
func TestFeishuCLIConcurrentResourceAndBindingUnique(t *testing.T) {
	s, _, id := newCLIFixture(t)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			r, err := s.EnsureFeishuCLI(context.Background())
			if err != nil || r.InstallationID != id {
				t.Errorf("resource=%+v err=%v", r, err)
			}
			_, err = s.Bind(context.Background(), "worker", BindRequest{ResourceID: id})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	items, _ := s.List(context.Background(), "worker")
	if len(items) != 1 {
		t.Fatalf("bindings=%v", items)
	}
}

func TestFeishuCLIDisconnectCancelsAuthorizationAndRunsOnce(t *testing.T) {
	s, f, id := newCLIFixture(t)
	entered := make(chan struct{})
	f.wait = func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }
	first := startCLITest(t, s, id, "connect", "connect-pending")
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("authorization did not start")
	}
	next := startCLITest(t, s, id, "disconnect", "disconnect-queued")
	if again, err := s.StartCLI(context.Background(), id, CLIRequest{Action: "disconnect", OperationID: "disconnect-queued", Revision: next.Revision}); err != nil || again.SessionID != next.SessionID {
		t.Fatal("duplicate queued operation")
	}
	if got := waitCLIOperation(t, s, id, next); got.Status != "completed" {
		t.Fatalf("disconnect=%+v", got)
	}
	if got := waitCLIOperation(t, s, id, first); got.Status != "cancelled" {
		t.Fatalf("connect=%+v", got)
	}
	r, err := s.cliSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Disconnected || r.AuthStatus != nil {
		t.Fatal("resource remained configured")
	}
	if _, err := os.Stat(filepath.Join(r.CLI.ConfigDir, "config.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config remains: %v", err)
	}
}

func TestFeishuCLIPersonalDenialKeepsBotConfiguration(t *testing.T) {
	s, f, id := newCLIFixture(t)
	f.wait = func(context.Context) error { return &connectorcli.OperationError{Code: "authorization_denied"} }
	op := startCLITest(t, s, id, "connect", "denied-personal")
	if got := waitCLIOperation(t, s, id, op); got.Status != "denied" || got.ErrorCode != "authorization_denied" {
		t.Fatalf("operation=%+v", got)
	}
	r, err := s.cliSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	var status larkcli.Status
	_ = json.Unmarshal(r.AuthStatus, &status)
	if r.AuthStatus == nil || !status.Identities.Bot.Available || status.Identities.User.Available {
		t.Fatal("bot-only authorization was not preserved")
	}
	f.wait = nil
	retry := startCLITest(t, s, id, "connect", "retry-personal")
	if got := waitCLIOperation(t, s, id, retry); got.Status != "completed" {
		t.Fatalf("retry=%+v", got)
	}
	count := 0
	for _, call := range f.calls {
		if strings.HasPrefix(call, "config init") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("application initialization count=%d", count)
	}
}

func TestFeishuCLIRestartResumesPersistedAuthorization(t *testing.T) {
	s, f, id := newCLIFixture(t)
	entered := make(chan struct{})
	f.wait = func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }
	op := startCLITest(t, s, id, "connect", "restart-connect")
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("authorization did not start")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	f.wait = nil
	restored, err := NewService(s.path, s.options)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restored.scheduleConnectorWork()
	if got := waitCLIOperation(t, restored, id, op); got.Status != "completed" {
		t.Fatalf("restored=%+v", got)
	}
	initializations, authorizations := 0, 0
	for _, call := range f.calls {
		if strings.HasPrefix(call, "config init") {
			initializations++
		}
		if strings.HasPrefix(call, "auth login --no-wait") {
			authorizations++
		}
	}
	if initializations != 1 || authorizations != 1 {
		t.Fatalf("init=%d authorization=%d", initializations, authorizations)
	}
}

func TestFeishuCLINewOperationUsesCurrentWorkerState(t *testing.T) {
	s, f, id := newCLIFixture(t)
	binding, err := s.Bind(context.Background(), "worker", BindRequest{ResourceID: id})
	if err != nil {
		t.Fatal(err)
	}
	waitCLIBinding(t, s, binding.InstallationID, "worker")
	f.runtime.mu.Lock()
	f.runtime.running["worker"] = true
	f.runtime.mu.Unlock()
	if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "connect", "first")); got.Status != "completed" {
		t.Fatalf("connect=%+v", got)
	}
	f.runtime.mu.Lock()
	f.runtime.running["worker"] = false
	f.runtime.mu.Unlock()
	if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "check", "second")); got.Status != "completed" {
		t.Fatalf("check=%+v", got)
	}
	f.runtime.mu.Lock()
	defer f.runtime.mu.Unlock()
	if f.runtime.running["worker"] {
		t.Fatal("operation restarted a worker stopped after the previous operation")
	}
}

func TestFeishuCLIRecoveryDoesNotReuseAnotherUserVerification(t *testing.T) {
	s, _, id := newCLIFixture(t)
	if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "connect", "connect")); got.Status != "completed" {
		t.Fatalf("connect=%+v", got)
	}
	s.cliWorkMu.Lock()
	defer s.cliWorkMu.Unlock()
	s.options.CLI.Run = func(context.Context, connectorcli.Client, io.Reader, ...string) ([]byte, error) {
		return []byte(`{"appId":"cli_test","identities":{"bot":{"available":true},"user":{"available":true,"openId":"ou_other"}}}`), nil
	}
	if err := s.recoverCLIStatus(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	r, err := s.cliSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	var status larkcli.Status
	_ = json.Unmarshal(r.AuthStatus, &status)
	if status.Identities.User.Verified != nil || r.CLI.Ready {
		t.Fatal("recovery reused verification from a different user")
	}
}

type cliStateHookRuntime struct {
	*cliRuntimeStub
	state func(context.Context, string) (bool, error)
}

func (r cliStateHookRuntime) State(ctx context.Context, id, kind string) (bool, error) {
	return r.state(ctx, id)
}

func TestFeishuCLIDeletedWorkerBindingIsNotPersistedAgain(t *testing.T) {
	s, f, id := newCLIFixture(t)
	s.cliWorkMu.Lock()
	defer s.cliWorkMu.Unlock()
	binding, err := s.Bind(context.Background(), "worker", BindRequest{ResourceID: id})
	if err != nil {
		t.Fatal(err)
	}
	s.options.Runtime = cliStateHookRuntime{cliRuntimeStub: f.runtime, state: func(ctx context.Context, worker string) (bool, error) {
		return false, s.DeleteAgent(ctx, worker)
	}}
	_ = s.applyBinding(context.Background(), binding.InstallationID)
	data, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), binding.InstallationID) {
		t.Fatal("deleted binding was persisted again during configuration")
	}
}

func TestFeishuCLIDisconnectPreservesConfigWhenLogoutFails(t *testing.T) {
	s, _, id := newCLIFixture(t)
	op := startCLITest(t, s, id, "connect", "connect-before-logout-error")
	if got := waitCLIOperation(t, s, id, op); got.Status != "completed" {
		t.Fatalf("connect=%+v", got)
	}
	run := s.options.CLI.Run
	s.options.CLI.Run = func(ctx context.Context, client connectorcli.Client, input io.Reader, args ...string) ([]byte, error) {
		if strings.Join(args, " ") == "auth logout" {
			return nil, errors.New("logout failed")
		}
		return run(ctx, client, input, args...)
	}
	op = startCLITest(t, s, id, "disconnect", "disconnect-logout-error")
	if got := waitCLIOperation(t, s, id, op); got.Status != "failed" {
		t.Fatalf("disconnect=%+v", got)
	}
	r, err := s.cliSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Disconnected {
		t.Fatal("resource disconnected after logout failed")
	}
	if _, err := os.Stat(filepath.Join(r.CLI.ConfigDir, "config.json")); err != nil {
		t.Fatalf("configuration unavailable after logout failed: %v", err)
	}
}

func TestFeishuCLILoginSkipsDisabledWorker(t *testing.T) {
	s, f, id := newCLIFixture(t)
	ctx := context.Background()
	if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "connect", "connect")); got.Status != "completed" {
		t.Fatalf("connect=%+v", got)
	}
	binding, err := s.Bind(ctx, "worker", BindRequest{ResourceID: id})
	if err != nil {
		t.Fatal(err)
	}
	waitCLIBinding(t, s, binding.InstallationID, "worker")
	if _, err = s.SetCLIEnabled(ctx, "worker", binding.InstallationID, false); err != nil {
		t.Fatal(err)
	}
	if got := waitCLIBinding(t, s, binding.InstallationID, "worker"); got.Status != "disabled" {
		t.Fatalf("binding=%+v", got)
	}
	f.runtime.mu.Lock()
	f.runtime.running["worker"] = true
	f.runtime.events = nil
	f.runtime.mu.Unlock()
	f.wait = func(context.Context) error { return s.CheckCLIReady("worker") }
	if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "login", "login")); got.Status != "completed" {
		t.Fatalf("login=%+v", got)
	}
	f.runtime.mu.Lock()
	defer f.runtime.mu.Unlock()
	if !f.runtime.running["worker"] || f.runtime.configured["worker"] != nil || len(f.runtime.events) != 0 {
		t.Fatalf("running=%v configured=%v events=%v", f.runtime.running, f.runtime.configured, f.runtime.events)
	}
}

func TestFeishuCLILoginCompletesFailedDisable(t *testing.T) {
	s, f, id := newCLIFixture(t)
	ctx := context.Background()
	if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "connect", "connect")); got.Status != "completed" {
		t.Fatalf("connect=%+v", got)
	}
	binding, err := s.Bind(ctx, "worker", BindRequest{ResourceID: id})
	if err != nil {
		t.Fatal(err)
	}
	waitCLIBinding(t, s, binding.InstallationID, "worker")
	f.runtime.mu.Lock()
	f.runtime.running["worker"] = true
	f.runtime.fail = "stop:worker"
	f.runtime.mu.Unlock()
	if _, err = s.SetCLIEnabled(ctx, "worker", binding.InstallationID, false); err != nil {
		t.Fatal(err)
	}
	if got := waitCLIBinding(t, s, binding.InstallationID, "worker"); got.Status != "error" {
		t.Fatalf("binding=%+v", got)
	}
	if err := s.CheckCLIReady("worker"); err == nil {
		t.Fatal("unfinished disable allowed worker startup")
	}
	f.runtime.mu.Lock()
	f.runtime.fail = ""
	f.runtime.mu.Unlock()
	if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "login", "login")); got.Status != "completed" {
		t.Fatalf("login=%+v", got)
	}
	if got := waitCLIBinding(t, s, binding.InstallationID, "worker"); got.Status != "disabled" {
		t.Fatalf("binding=%+v", got)
	}
	f.runtime.mu.Lock()
	defer f.runtime.mu.Unlock()
	if !f.runtime.running["worker"] || f.runtime.configured["worker"] != nil {
		t.Fatalf("running=%v configured=%v", f.runtime.running, f.runtime.configured)
	}
}

func TestCLICancelAfterAuthorizationCommitRestoresWorkers(t *testing.T) {
	for _, scenario := range []struct{ action, failure string }{
		{action: "connect"},
		{action: "disconnect"},
		{action: "connect", failure: "configure:running"},
	} {
		t.Run(scenario.action+"/"+scenario.failure, func(t *testing.T) {
			s, f, id := newCLIFixture(t)
			for _, worker := range []string{"running", "stopped"} {
				binding, err := s.Bind(t.Context(), worker, BindRequest{ResourceID: id})
				if err != nil {
					t.Fatal(err)
				}
				waitCLIBinding(t, s, binding.InstallationID, worker)
			}
			if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "connect", "initial")); got.Status != "completed" {
				t.Fatalf("initial operation=%+v", got)
			}
			applying := make(chan struct{})
			var once sync.Once
			f.runtime.mu.Lock()
			f.runtime.running["running"] = true
			f.runtime.fail = scenario.failure
			f.runtime.beforeApply = func(ctx context.Context) (err error) {
				once.Do(func() {
					close(applying)
					<-ctx.Done()
					err = ctx.Err()
				})
				return err
			}
			f.runtime.mu.Unlock()
			op := startCLITest(t, s, id, scenario.action, "cancel-applying")
			select {
			case <-applying:
			case <-time.After(3 * time.Second):
				t.Fatal("worker configuration did not start")
			}
			if _, err := s.CancelCLI(t.Context(), id, op.SessionID); err != nil {
				t.Fatal(err)
			}
			got := waitCLIOperation(t, s, id, op)
			if scenario.failure != "" {
				if got.Status != "failed" || got.ErrorCode != "cleanup_pending" {
					t.Fatalf("failed recovery=%+v", got)
				}
				if err := s.CheckCLIReady("running"); err == nil {
					t.Fatal("worker startup was allowed after failed recovery")
				}
				f.runtime.mu.Lock()
				defer f.runtime.mu.Unlock()
				if f.runtime.running["running"] {
					t.Fatal("worker resumed after failed recovery")
				}
				return
			}
			if got.Status != "cancelled" || got.ErrorCode != "" {
				t.Fatalf("cancelled operation=%+v", got)
			}
			for _, worker := range []string{"running", "stopped"} {
				if err := s.CheckCLIReady(worker); err != nil {
					t.Fatalf("worker %s cannot start: %v", worker, err)
				}
			}
			f.runtime.mu.Lock()
			defer f.runtime.mu.Unlock()
			if !f.runtime.running["running"] || f.runtime.running["stopped"] {
				t.Fatalf("worker states=%v", f.runtime.running)
			}
			for _, worker := range []string{"running", "stopped"} {
				if configured := f.runtime.configured[worker] != nil; configured != (scenario.action == "connect") {
					t.Fatalf("worker %s configured=%v after %s", worker, configured, scenario.action)
				}
			}
		})
	}
}
