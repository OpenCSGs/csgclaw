package apps

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"csgclaw/internal/connectorcli"
	skill "csgclaw/internal/skill/state"
)

func packageFixture(kind string) fstest.MapFS {
	source := fstest.MapFS{
		"plugin.json":             {Data: []byte(`{"name":"fixture","version":"2.0.0","kind":"` + kind + `","` + kind + `":"./` + kind + `.json","skills":"./skills","interface":{"displayName":"Fixture"}}`)},
		"skills/fixture/SKILL.md": {Data: []byte("---\nname: fixture\ndescription: Use the fixture connector.\n---\nRun fixture commands.\n")},
	}
	if kind == KindMCP {
		source["mcp.json"] = &fstest.MapFile{Data: []byte(`{"mcpServers":{"fixture":{"transport":"http","url":"https://example.com/mcp","auth_mode":"none"}}}`)}
	} else {
		spec := connectorcli.Spec{Install: connectorcli.InstallSpec{Version: "2.0.0", Executable: "fixture", VersionArgs: []string{"--version"}, Releases: map[string]connectorcli.Archive{"test": {URL: "https://example.com/fixture.tar.gz", SHA256: strings.Repeat("0", 64)}}}, Environment: map[string]string{"FIXTURE_CONFIG": "${CONFIG_DIR}"}, Auth: connectorcli.Commands{{Args: []string{"auth"}}}, Status: []string{"status"}, StatusMatch: `^ready\s*$`, UnAuth: connectorcli.Commands{{Args: []string{"logout"}}}}
		data, _ := json.Marshal(spec)
		source["cli.json"] = &fstest.MapFile{Data: data}
	}
	return source
}
func catalogFixture(source fstest.MapFS) fstest.MapFS {
	catalog := fstest.MapFS{}
	for name, file := range source {
		catalog["fixture/"+name] = file
	}
	return catalog
}
func TestConnectorPackageValidation(t *testing.T) {
	for _, kind := range []string{KindMCP, KindCLI} {
		t.Run(kind, func(t *testing.T) {
			pkg, err := readPackage(packageFixture(kind))
			if err != nil {
				t.Fatal(err)
			}
			if pkg.Manifest.Kind != kind || len(pkg.SkillFiles) != 1 {
				t.Fatalf("package=%+v", pkg)
			}
		})
	}
	for _, tc := range []struct{ name, manifest string }{
		{"traversal", `{"name":"fixture","version":"1.0.0","kind":"mcp","mcp":"../outside.json"}`},
		{"backslash", `{"name":"fixture","version":"1.0.0","kind":"mcp","mcp":"..\\outside.json"}`},
		{"mixed", `{"name":"fixture","version":"1.0.0","kind":"mcp","mcp":"mcp.json","cli":"cli.json"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := packageFixture(KindMCP)
			source["plugin.json"] = &fstest.MapFile{Data: []byte(tc.manifest)}
			if _, err := readPackage(source); err == nil {
				t.Fatal("invalid package accepted")
			}
		})
	}
	legacy := fstest.MapFS{"plugin.json": {Data: []byte(`{"name":"example","apps":"./apps.json","interface":{"displayName":"Example"}}`)}, "apps.json": {Data: []byte(`{"apps":{"service":{"id":"example","required":true}}}`)}}
	pkg, err := readPackage(legacy)
	if err != nil || pkg.Manifest.Kind != KindMCP {
		t.Fatalf("legacy=%+v err=%v", pkg, err)
	}
}

// The runtime fixture records per-resource configuration and applies real Skill files.
type packageRuntime struct {
	cliRuntimeStub
	root      string
	resources map[string]*CLIProjection
}

func (r *packageRuntime) Configure(ctx context.Context, id, resourceID string, p *CLIProjection) error {
	if err := r.cliRuntimeStub.Configure(ctx, id, resourceID, p); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources[resourceID] = p
	return nil
}
func (r *packageRuntime) PlanSkills(id, bundle, version string, previous []skill.ManagedSkill, enabled bool) (skill.SkillPlan, error) {
	plan := skill.SkillPlan{Before: previous, Bundle: bundle}
	if enabled {
		entries, err := os.ReadDir(bundle)
		if err != nil {
			return plan, err
		}
		for _, entry := range entries {
			digest, err := skill.DirectoryDigest(filepath.Join(bundle, entry.Name()))
			if err != nil {
				return plan, err
			}
			plan.After = append(plan.After, skill.ManagedSkill{Name: entry.Name(), Version: version, Digest: digest, Managed: true})
		}
	}
	return plan, nil
}
func (r *packageRuntime) ApplySkills(_ context.Context, id string, plan skill.SkillPlan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running[id] {
		return os.ErrPermission
	}
	for _, previous := range plan.Before {
		if !previous.Managed {
			continue
		}
		if err := os.RemoveAll(filepath.Join(r.root, id, previous.Name)); err != nil {
			return err
		}
	}
	for _, next := range plan.After {
		data, err := os.ReadFile(filepath.Join(plan.Bundle, next.Name, "SKILL.md"))
		if err != nil {
			return err
		}
		if err := connectorcli.AtomicWrite(filepath.Join(r.root, id, next.Name, "SKILL.md"), data); err != nil {
			return err
		}
	}
	return nil
}
func newPackageRuntime(t *testing.T) *packageRuntime {
	return &packageRuntime{cliRuntimeStub: cliRuntimeStub{running: map[string]bool{"worker": true}, configured: map[string]*CLIProjection{}}, root: t.TempDir(), resources: map[string]*CLIProjection{}}
}
func waitPackageBinding(t *testing.T, s *Service, agentID, id string) Installation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		e := s.entries[id]
		pending := e != nil && e.record.CLIApply != nil && e.record.CLIApply.Pending
		s.mu.Unlock()
		item, err := s.Get(context.Background(), agentID, id)
		if err == nil && (!pending || item.LastErrorCode != "") {
			return item
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("package binding did not settle")
	return Installation{}
}
func TestMCPPackageDefaultsAndSkillLifecycle(t *testing.T) {
	rt := newPackageRuntime(t)
	s := newTestService(t, Options{Packages: catalogFixture(packageFixture(KindMCP)), Runtime: rt})
	ctx := context.Background()
	resource, err := s.Create(ctx, "", CreateRequest{AppID: "fixture", Name: "Custom MCP"})
	if err != nil {
		t.Fatal(err)
	}
	if resource.Kind != KindMCP || resource.Config.URL != "https://example.com/mcp" || resource.Config.AuthMode != "none" {
		t.Fatalf("resource=%+v", resource)
	}
	binding, err := s.Bind(ctx, "worker", BindRequest{ResourceID: resource.InstallationID})
	if err != nil {
		t.Fatal(err)
	}
	item := waitPackageBinding(t, s, "worker", binding.InstallationID)
	if item.LastErrorCode != "" || len(item.Skills) != 1 {
		t.Fatalf("binding=%+v", item)
	}
	target := filepath.Join(rt.root, "worker", "fixture", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
	disabled := false
	if _, err := s.Update(ctx, "worker", binding.InstallationID, UpdateRequest{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	waitPackageBinding(t, s, "worker", binding.InstallationID)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("disabled Skills still present: %v", err)
	}
	enabled := true
	if _, err := s.Update(ctx, "worker", binding.InstallationID, UpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	waitPackageBinding(t, s, "worker", binding.InstallationID)
	if err := s.Delete(ctx, "", resource.InstallationID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("deleted Skills still present: %v", err)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.running["worker"] {
		t.Fatal("worker was not resumed")
	}
}
func TestDeclarativeCLICommandsAndResourceBinding(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell executable fixture")
	}
	rt := newPackageRuntime(t)
	install := func(_ context.Context, root string, _ func(string)) (connectorcli.Package, error) {
		executable := filepath.Join(root, "bin", "fixture")
		script := `#!/bin/sh
case "$1" in
 --version) echo 'fixture 2.0.0';;
 auth) printf 'ready\n' > "$FIXTURE_CONFIG/status"; printf 'auth\n' >> "$FIXTURE_CONFIG/calls";;
 status) if [ -f "$FIXTURE_CONFIG/status" ]; then cat "$FIXTURE_CONFIG/status"; else echo unauthorized; fi;;
 logout) rm -f "$FIXTURE_CONFIG/status";;
esac
`
		if err := connectorcli.AtomicWrite(executable, []byte(script)); err != nil {
			return connectorcli.Package{}, err
		}
		if err := os.Chmod(executable, 0700); err != nil {
			return connectorcli.Package{}, err
		}
		bundle := filepath.Join(root, "skills")
		if err := os.MkdirAll(bundle, 0700); err != nil {
			return connectorcli.Package{}, err
		}
		return connectorcli.Package{Executable: executable, SkillsDir: bundle, Version: "2.0.0"}, nil
	}
	s := newTestService(t, Options{Packages: catalogFixture(packageFixture(KindCLI)), Runtime: rt, CLI: &CLIOptions{Install: install}})
	ctx := context.Background()
	resource, err := s.Create(ctx, "", CreateRequest{AppID: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := s.Bind(ctx, "worker", BindRequest{ResourceID: resource.InstallationID})
	if err != nil {
		t.Fatal(err)
	}
	waitPackageBinding(t, s, "worker", binding.InstallationID)
	op := startCLITest(t, s, resource.InstallationID, "connect", "connect")
	if got := waitCLIOperation(t, s, resource.InstallationID, op); got.Status != "completed" {
		t.Fatalf("operation=%+v", got)
	}
	item := waitPackageBinding(t, s, "worker", binding.InstallationID)
	if item.Status != "connected" || !item.Authorized || len(item.Skills) != 1 {
		t.Fatalf("binding=%+v", item)
	}
	p, _, err := s.CLIProjection("worker", resource.InstallationID)
	if err != nil || p == nil || p.ConnectorID != "fixture" || p.Environment["FIXTURE_CONFIG"] != p.ConfigDir {
		t.Fatalf("projection=%+v error=%v", p, err)
	}
	if other, _, err := s.CLIProjection("worker", "another-resource"); err != nil || other != nil {
		t.Fatalf("unrelated projection=%+v error=%v", other, err)
	}
	op = startCLITest(t, s, resource.InstallationID, "check", "check")
	if got := waitCLIOperation(t, s, resource.InstallationID, op); got.Status != "completed" {
		t.Fatalf("check=%+v", got)
	}
	calls, err := os.ReadFile(filepath.Join(p.ConfigDir, "calls"))
	if err != nil || string(calls) != "auth\n" {
		t.Fatalf("status started authorization: %q %v", calls, err)
	}
	op = startCLITest(t, s, resource.InstallationID, "disconnect", "disconnect")
	if got := waitCLIOperation(t, s, resource.InstallationID, op); got.Status != "completed" {
		t.Fatalf("disconnect=%+v", got)
	}
	item = waitPackageBinding(t, s, "worker", binding.InstallationID)
	if item.Authorized || item.Status == "connected" {
		t.Fatalf("disconnected=%+v", item)
	}
	if _, err := os.Stat(filepath.Join(rt.root, "worker", "fixture", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("CLI Skills still present: %v", err)
	}
}

func TestMCPResourcesShareManagedSkills(t *testing.T) {
	rt := newPackageRuntime(t)
	s := newTestService(t, Options{Packages: catalogFixture(packageFixture(KindMCP)), Runtime: rt})
	ctx := context.Background()
	var bindings []Installation
	for _, name := range []string{"one", "two"} {
		resource, err := s.Create(ctx, "", CreateRequest{AppID: "fixture", Name: name})
		if err != nil {
			t.Fatal(err)
		}
		binding, err := s.Bind(ctx, "worker", BindRequest{ResourceID: resource.InstallationID})
		if err != nil {
			t.Fatal(err)
		}
		if got := waitPackageBinding(t, s, "worker", binding.InstallationID); got.LastErrorCode != "" {
			t.Fatalf("binding=%+v", got)
		}
		bindings = append(bindings, binding)
	}
	if err := s.Delete(ctx, "worker", bindings[0].InstallationID); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(rt.root, "worker", "fixture", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("shared Skill removed: %v", err)
	}
	if err := s.Delete(ctx, "worker", bindings[1].InstallationID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("unreferenced Skill remains: %v", err)
	}
}

func TestPackageSnapshotContainsConfigurationAndRejectsNestedSymlinks(t *testing.T) {
	pkg, err := readPackage(packageFixture(KindMCP))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dirs, err := prepareProcessDirs(filepath.Join(root, "instance"), pkg)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := readPackage(os.DirFS(dirs.Plugin))
	if err != nil || loaded.MCP.config().URL != pkg.MCP.config().URL || len(loaded.SkillFiles) != 1 {
		t.Fatalf("snapshot=%+v error=%v", loaded, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.RemoveAll(filepath.Join(dirs.Plugin, "skills")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dirs.Plugin, "skills")); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareProcessDirs(filepath.Join(root, "instance"), pkg); err == nil {
		t.Fatal("followed nested package symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("wrote outside the package directory")
	}
}

func TestDeleteWorkerRemovesPendingMCPBindingWithoutRuntimeChanges(t *testing.T) {
	rt := newPackageRuntime(t)
	rt.fail = "stop:worker"
	s := newTestService(t, Options{Packages: catalogFixture(packageFixture(KindMCP)), Runtime: rt})
	s.cliWorkMu.Lock()
	defer s.cliWorkMu.Unlock()
	ctx := context.Background()
	resource, err := s.Create(ctx, "", CreateRequest{AppID: "fixture", Name: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := s.Bind(ctx, "worker", BindRequest{ResourceID: resource.InstallationID})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAgent(ctx, "worker"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "worker", binding.InstallationID); err != ErrNotFound {
		t.Fatalf("binding still exists: %v", err)
	}
	if _, err := s.Get(ctx, "", resource.InstallationID); err != nil {
		t.Fatal("shared resource was removed")
	}
}

func TestMCPPackageConnectsWithDeclaredEndpoint(t *testing.T) {
	upstream := upstreamServer(t)
	source := packageFixture(KindMCP)
	data, _ := json.Marshal(mcpPackage{Servers: map[string]Config{"fixture": {URL: upstream.URL, AuthMode: "bearer", TokenPrefix: "Bearer "}}})
	source["mcp.json"] = &fstest.MapFile{Data: data}
	s := newTestService(t, Options{Packages: catalogFixture(source), Runtime: newPackageRuntime(t)})
	item, err := s.Create(context.Background(), "worker", CreateRequest{AppID: "fixture", Name: "Fixture", Credentials: Credentials{Token: "fixture-token"}, Connect: true})
	if err != nil || len(item.Tools) != 1 {
		t.Fatalf("connection=%+v error=%v", item, err)
	}
	if item.Config.URL != upstream.URL || item.CredentialsSet["token"] != true {
		t.Fatalf("settings=%+v", item)
	}
}
