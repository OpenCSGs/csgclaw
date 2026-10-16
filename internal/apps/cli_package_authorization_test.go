package apps

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/connectorcli"
)

func TestFeishuPackageAuthorizationPermissions(t *testing.T) {
	packages, err := loadPackages()
	if err != nil {
		t.Fatal(err)
	}
	spec := packages["feishu"].CLI
	var shown string
	workflow := connectorcli.Workflow{Client: connectorcli.Client{AuthorizationDomains: spec.AuthorizationDomains, URL: func(value string) { shown = value }, Runner: func(_ context.Context, c connectorcli.Client, _ io.Reader, args ...string) ([]byte, error) {
		if args[0] == "auth" {
			return []byte(`{"identities":{"bot":{"available":false}}}`), nil
		}
		if !slices.Equal(args, []string{"config", "init", "--new", "--brand", "feishu", "--lang", "zh"}) {
			t.Fatalf("init command=%v", args)
		}
		c.URL("https://open.feishu.cn/page/cli?user_code=test&from=cli&addons=old")
		return nil, nil
	}}}
	if err := workflow.Run(context.Background(), spec.Init); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(shown)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/page/launcher" || u.Query().Get("user_code") != "test" || u.Query().Get("tp") != "sdk" || u.Query().Get("from") != "sdk" {
		t.Fatal("application registration parameters changed")
	}
	compressed, err := base64.RawURLEncoding.DecodeString(u.Query().Get("addons"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var addons struct {
		Preset *bool               `json:"preset"`
		Scopes map[string][]string `json:"scopes"`
	}
	if err := json.NewDecoder(reader).Decode(&addons); err != nil {
		t.Fatal(err)
	}
	if addons.Preset == nil || *addons.Preset {
		t.Fatal("minimal application template not selected")
	}
	for identity, scopes := range addons.Scopes {
		if len(scopes) == 0 || !slices.IsSorted(scopes) || len(slices.Compact(slices.Clone(scopes))) != len(scopes) {
			t.Fatalf("invalid %s scope list", identity)
		}
		for _, scope := range []string{"calendar:calendar:read", "search:docs:read", "attendance:task:readonly", "mail:user_mailbox.message:send", "task:task:write", "task:tasklist:write", "vc:record:readonly", "vc:meeting.meetingevent:read"} {
			if slices.Contains(scopes, scope) {
				t.Errorf("%s requests reviewed permission %s", identity, scope)
			}
		}
	}
	tenant, user := addons.Scopes["tenant"], addons.Scopes["user"]
	for _, scope := range []string{"im:message.urgent", "mail:user_mailbox:readonly", "mail:user_mailbox.message:modify"} {
		if slices.Contains(tenant, scope) {
			t.Errorf("tenant requests reviewed permission %s", scope)
		}
	}
	if !slices.Contains(tenant, "application:application:self_manage") || !slices.Contains(tenant, "im:message:send_as_bot") || !slices.Contains(user, "docx:document:create") || !slices.Contains(user, "task:task:read") {
		t.Fatal("required permissions missing")
	}
	if strings.Join(user, " ") != spec.RequestedScopes {
		t.Fatal("personal consent differs from application permissions")
	}
	workflow.RequestedScopes = spec.RequestedScopes
	workflow.Client.Runner = func(_ context.Context, _ connectorcli.Client, _ io.Reader, args ...string) ([]byte, error) {
		switch strings.Join(args[:2], " ") {
		case "auth status":
			return []byte(`{"identities":{"user":{"available":true,"verified":true}}}`), nil
		case "auth login":
			if args[2] == "--no-wait" {
				if !slices.Equal(args, []string{"auth", "login", "--no-wait", "--json", "--scope", strings.Join(user, " ")}) {
					t.Fatal("personal consent arguments changed")
				}
				return []byte(`{"device_code":"private","verification_url":"https://open.feishu.cn/device","expires_in":60}`), nil
			}
		}
		return nil, nil
	}
	command := spec.Auth[0]
	command.Unless = nil
	if err := workflow.Run(context.Background(), connectorcli.Commands{command}); err != nil {
		t.Fatal(err)
	}
}
func TestCLIStatusCheckDoesNotAuthorize(t *testing.T) {
	s, f, id := newCLIFixture(t)
	op := startCLITest(t, s, id, "check", "status-only")
	if got := waitCLIOperation(t, s, id, op); got.Status != "completed" {
		t.Fatalf("operation=%+v", got)
	}
	for _, call := range f.calls {
		if !strings.HasPrefix(call, "auth status") {
			t.Fatalf("status invoked %s", call)
		}
	}
}
func TestCLIConfiguredUserStillAppliesIdentityPolicy(t *testing.T) {
	s, f, id := newCLIFixture(t)
	if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "connect", "first")); got.Status != "completed" {
		t.Fatal(got)
	}
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
	if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "connect", "policy")); got.Status != "completed" {
		t.Fatal(got)
	}
	for _, call := range f.calls {
		if strings.HasPrefix(call, "config init") || strings.HasPrefix(call, "auth login") {
			t.Fatalf("repeated authorization: %s", call)
		}
	}
	for _, command := range []string{"config strict-mode off --global", "config strict-mode off", "config default-as auto"} {
		if !slices.Contains(f.calls, command) {
			t.Fatalf("missing identity command %s", command)
		}
	}
}

func TestFeishuDisconnectAcceptsAlreadyRemovedConfiguration(t *testing.T) {
	packages, err := loadPackages()
	if err != nil {
		t.Fatal(err)
	}
	spec := packages["feishu"].CLI
	var calls []string
	workflow := connectorcli.Workflow{Client: connectorcli.Client{Runner: func(_ context.Context, _ connectorcli.Client, _ io.Reader, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return []byte(`{"error":{"subtype":"not_configured"}}`), fmt.Errorf("configuration already removed")
	}}}
	if err := workflow.Run(t.Context(), spec.UnAuth); err != nil {
		t.Fatal(err)
	}
	if err := workflow.Run(t.Context(), spec.Disconnect); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"auth logout", "config remove"}) {
		t.Fatalf("cleanup commands=%v", calls)
	}
}

func TestFeishuWorkflowRunsUnderCustomPackageName(t *testing.T) {
	source, fixture, _ := newCLIFixture(t)
	packages, err := loadPackages()
	if err != nil {
		t.Fatal(err)
	}
	files := packageFixture(KindCLI)
	spec := *packages["feishu"].CLI
	spec.Install.Skills = connectorcli.SkillArchive{}
	configuration, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	files["cli.json"].Data = configuration
	service := newTestService(t, Options{Packages: catalogFixture(files), Runtime: fixture.runtime, CLI: source.options.CLI})
	resource, err := service.Create(context.Background(), "", CreateRequest{AppID: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	for i, action := range []string{"connect", "check", "logout", "login", "disconnect"} {
		operation := startCLITest(t, service, resource.InstallationID, action, fmt.Sprintf("operation-%d", i))
		if got := waitCLIOperation(t, service, resource.InstallationID, operation); got.Status != "completed" {
			t.Fatalf("%s: %+v", action, got)
		}
	}
	resource, err = service.Get(context.Background(), "", resource.InstallationID)
	if err != nil || resource.Authorized || !resource.Disconnected {
		t.Fatalf("resource=%+v error=%v", resource, err)
	}
}

func TestCLICancelPendingResourcePreservesActiveAuthorization(t *testing.T) {
	for _, action := range []string{"cancel", "disconnect"} {
		t.Run(action, func(t *testing.T) {
			source, fixture, _ := newCLIFixture(t)
			service := newTestService(t, Options{Packages: catalogFixture(packageFixture(KindCLI)), Runtime: fixture.runtime, CLI: source.options.CLI})
			active, err := service.EnsureFeishuCLI(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			pending, err := service.Create(context.Background(), "", CreateRequest{AppID: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			fixture.wait = func(ctx context.Context) error {
				close(entered)
				select {
				case <-ctx.Done():
				case <-release:
				}
				return ctx.Err()
			}
			first := startCLITest(t, service, active.InstallationID, "connect", "active")
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("authorization did not start")
			}
			second := startCLITest(t, service, pending.InstallationID, "connect", "pending")
			var disconnect CLIOperation
			if action == "cancel" {
				if _, err := service.CancelCLI(context.Background(), pending.InstallationID, second.SessionID); err != nil {
					t.Fatal(err)
				}
			} else {
				disconnect = startCLITest(t, service, pending.InstallationID, "disconnect", "disconnect")
			}
			close(release)
			if got := waitCLIOperation(t, service, active.InstallationID, first); got.Status != "completed" {
				t.Fatalf("active authorization=%+v", got)
			}
			if got := waitCLIOperation(t, service, pending.InstallationID, second); got.Status != "cancelled" {
				t.Fatalf("pending authorization=%+v", got)
			}
			if action == "disconnect" {
				if got := waitCLIOperation(t, service, pending.InstallationID, disconnect); got.Status != "completed" {
					t.Fatalf("disconnect=%+v", got)
				}
			}
		})
	}
}

func TestFeishuPackageLogoutClearsExpiredCredentials(t *testing.T) {
	packages, err := loadPackages()
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"expired", "not_configured", "not_logged_in"} {
		t.Run(state, func(t *testing.T) {
			retained := state == "expired"
			calls := 0
			workflow := connectorcli.Workflow{Client: connectorcli.Client{Runner: func(_ context.Context, _ connectorcli.Client, _ io.Reader, args ...string) ([]byte, error) {
				switch strings.Join(args, " ") {
				case "auth status --json":
					if state == "not_configured" {
						return []byte(`{"error":{"subtype":"not_configured"}}`), &connectorcli.OperationError{Code: "cli_operation_failed"}
					}
					if state == "not_logged_in" {
						return []byte(`{"identities":{"user":{"available":false,"status":"missing"}}}`), nil
					}
					return []byte(`{"identities":{"user":{"available":false,"status":"missing","tokenStatus":"expired"}}}`), nil
				case "auth logout":
					calls++
					retained = false
					return nil, nil
				default:
					t.Fatalf("unexpected command: %v", args)
					return nil, nil
				}
			}}}
			if err := workflow.Run(context.Background(), packages["feishu"].CLI.UnAuth); err != nil {
				t.Fatal(err)
			}
			if retained || calls != 1 {
				t.Fatalf("retained credentials=%v, logout calls=%d", retained, calls)
			}
		})
	}
}

func TestFeishuCLIDisconnectClearsUnavailableConfiguration(t *testing.T) {
	for _, state := range []string{"bot_unavailable", "not_configured"} {
		t.Run(state, func(t *testing.T) {
			service, _, id := newCLIFixture(t)
			if got := waitCLIOperation(t, service, id, startCLITest(t, service, id, "connect", "connect")); got.Status != "completed" {
				t.Fatal(got)
			}
			snapshot, err := service.cliSnapshot(id)
			if err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(snapshot.CLI.ConfigDir, "config.json")
			if state == "not_configured" {
				if err := os.Remove(configPath); err != nil {
					t.Fatal(err)
				}
			}
			run := service.options.CLI.Run
			removals := 0
			service.options.CLI.Run = func(ctx context.Context, client connectorcli.Client, input io.Reader, args ...string) ([]byte, error) {
				if strings.Join(args, " ") == "config remove" {
					removals++
					if state == "not_configured" {
						return []byte(`{"error":{"subtype":"not_configured"}}`), &connectorcli.OperationError{Code: "cli_operation_failed"}
					}
				}
				data, err := run(ctx, client, input, args...)
				if err == nil && strings.HasPrefix(strings.Join(args, " "), "auth status") {
					var status map[string]any
					if err := json.Unmarshal(data, &status); err != nil {
						return nil, err
					}
					status["identities"].(map[string]any)["bot"] = map[string]any{"available": false, "status": "not_configured"}
					return json.Marshal(status)
				}
				return data, err
			}
			if got := waitCLIOperation(t, service, id, startCLITest(t, service, id, "disconnect", "disconnect")); got.Status != "completed" {
				t.Fatalf("disconnect=%+v", got)
			}
			current, err := service.Get(context.Background(), "", id)
			if err != nil || !current.Disconnected || current.Authorized {
				t.Fatalf("resource=%+v error=%v", current, err)
			}
			if _, err := os.Stat(configPath); !os.IsNotExist(err) {
				t.Fatalf("configuration remains: %v", err)
			}
			if removals != 1 {
				t.Fatalf("configuration removal calls=%d", removals)
			}
		})
	}
}

func TestCLIPublishesAuthorizationStagesBeforeCompletion(t *testing.T) {
	for _, cancelAtUser := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelAtUser), func(t *testing.T) {
			s, fixture, id := newCLIFixture(t)
			userEntered, policyEntered := make(chan struct{}), make(chan struct{})
			allowUser, allowPolicy := make(chan struct{}), make(chan struct{})
			s.options.CLI.Run = func(ctx context.Context, client connectorcli.Client, input io.Reader, args ...string) ([]byte, error) {
				var entered, release chan struct{}
				switch strings.Join(args, " ") {
				case "auth login --device-code private-code --json":
					entered, release = userEntered, allowUser
				case "config strict-mode off --global":
					entered, release = policyEntered, allowPolicy
				}
				if entered != nil {
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				return fixture.run(ctx, client, input, args...)
			}
			op := startCLITest(t, s, id, "connect", "stages")
			assertStage := func(entered chan struct{}, user bool) Installation {
				t.Helper()
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("authorization stage timeout")
				}
				item, err := s.Get(context.Background(), "", id)
				if err != nil {
					t.Fatal(err)
				}
				var status struct {
					Identities struct {
						Bot, User struct{ Available, Verified bool }
					}
				}
				if err := json.Unmarshal(item.AuthStatus, &status); err != nil {
					t.Fatal(err)
				}
				if !status.Identities.Bot.Available || !status.Identities.Bot.Verified || status.Identities.User.Verified != user {
					t.Fatalf("unexpected stage identities: %s", item.AuthStatus)
				}
				if item.Authorized || item.Status != "connecting" || item.Operation.Status != "running" || item.CheckedAt.IsZero() || item.Revision != op.Revision {
					t.Fatalf("stage prematurely committed: %+v", item)
				}
				return item
			}
			bot := assertStage(userEntered, false)
			if cancelAtUser {
				if _, err := s.CancelCLI(context.Background(), id, op.SessionID); err != nil {
					t.Fatal(err)
				}
				if got := waitCLIOperation(t, s, id, op); got.Status != "cancelled" {
					t.Fatalf("cancel=%+v", got)
				}
				return
			}
			close(allowUser)
			personal := assertStage(policyEntered, true)
			if !personal.CheckedAt.After(bot.CheckedAt) {
				t.Fatal("personal authorization did not refresh check time")
			}
			close(allowPolicy)
			if got := waitCLIOperation(t, s, id, op); got.Status != "completed" {
				t.Fatalf("completion=%+v", got)
			}
			item, err := s.Get(context.Background(), "", id)
			if err != nil || !item.Authorized || item.Status != "connected" {
				t.Fatalf("completed resource=%+v err=%v", item, err)
			}
		})
	}
}
