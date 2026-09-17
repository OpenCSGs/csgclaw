package apps

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stdioDirectoryFixture(t *testing.T) Config {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Config{Transport: "stdio", Command: executable, Args: []string{"-test.run=^TestAppStdioHelper$"}, AuthMode: "none", Env: map[string]string{"CSGCLAW_APP_TEST_HELPER": "1", "CSGCLAW_APP_PATHS_HELPER": "1", "HOME": "/untrusted-override", "PLUGIN_DATA": "/untrusted-data", "PLUGIN_ROOT": "/untrusted-root"}}
}

func directoryDescription(t *testing.T, item Installation) map[string]string {
	t.Helper()
	for _, tool := range item.Tools {
		if tool.Name == "directories" {
			var paths map[string]string
			if err := json.Unmarshal([]byte(tool.Description), &paths); err != nil {
				t.Fatal(err)
			}
			return paths
		}
	}
	t.Fatal("directory fixture did not expose its process paths")
	return nil
}

func TestAppInstanceDirectoriesArePrivatePersistentAndRemoved(t *testing.T) {
	s := newTestService(t, Options{})
	create := func(name string) Installation {
		item, err := s.Create(context.Background(), "agent", CreateRequest{AppID: "feishu", Name: name, Config: stdioDirectoryFixture(t), Connect: true})
		if err != nil || item.Status != "connected" {
			t.Fatalf("create: %v %s", err, item.LastError)
		}
		return item
	}
	one, two := create("One"), create("Two")
	paths := directoryDescription(t, one)
	other := directoryDescription(t, two)
	root := filepath.Join(filepath.Dir(s.path), "apps", one.InstallationID)
	for _, key := range []string{"home", "data", "plugin"} {
		expected := filepath.Join(root, key)
		if paths[key] != expected || paths[key] == other[key] {
			t.Fatalf("%s isolation: got %q, want %q", key, paths[key], expected)
		}
		info, err := os.Stat(paths[key])
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("%s not private: %v %v", key, info, err)
		}
	}
	actualData, _ := filepath.EvalSymlinks(paths["data"])
	if paths["cwd"] != actualData {
		t.Fatalf("default cwd is not instance data: %q", paths["cwd"])
	}
	for _, name := range []string{"plugin.json", "apps.json"} {
		if _, err := os.Stat(filepath.Join(paths["plugin"], name)); err != nil {
			t.Fatalf("missing package snapshot %s: %v", name, err)
		}
	}
	if files, err := os.ReadDir(paths["plugin"]); err != nil || len(files) != 2 {
		t.Fatal("package snapshot contains unexpected files")
	}
	marker := filepath.Join(paths["data"], "retained.txt")
	if err := os.WriteFile(marker, []byte("persistent app data"), 0600); err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	if _, err := s.Update(context.Background(), "agent", one.InstallationID, UpdateRequest{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Connect(context.Background(), "agent", one.InstallationID); err != nil {
		t.Fatal(err)
	}
	if value, err := os.ReadFile(marker); err != nil || string(value) != "persistent app data" {
		t.Fatal("reconnect lost instance data")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := NewService(s.path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	item, err := restored.Get(context.Background(), "agent", one.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if directoryDescription(t, item)["data"] != paths["data"] {
		t.Fatal("restart changed instance data path")
	}
	if err := restored.Delete(context.Background(), "agent", one.InstallationID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("deleted App retained its directory: %v", err)
	}
	if _, err := os.Stat(other["data"]); err != nil {
		t.Fatal("deleting one App removed another instance's data")
	}
	if err := restored.DeleteAgent(context.Background(), "agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(other["data"])); !os.IsNotExist(err) {
		t.Fatal("deleting Agent retained App data")
	}
}

func TestStdioProbeUsesTemporaryDirectoriesAndCleansThem(t *testing.T) {
	s := newTestService(t, Options{})
	result, err := s.Probe(context.Background(), "agent", ProbeRequest{AppID: "feishu", Config: stdioDirectoryFixture(t)})
	if err != nil {
		t.Fatal(err)
	}
	paths := directoryDescription(t, Installation{Tools: result.Tools})
	if !strings.Contains(filepath.Base(filepath.Dir(paths["home"])), "csgclaw-app-probe-") {
		t.Fatalf("probe did not use a temporary home: %q", paths["home"])
	}
	if _, err := os.Stat(filepath.Dir(paths["home"])); !os.IsNotExist(err) {
		t.Fatalf("probe retained temporary state: %v", err)
	}
	if _, err := os.Stat(s.dataRoot); !os.IsNotExist(err) {
		t.Fatal("stdio probe created persistent installation data")
	}
}

func TestHTTPProbeCreatesNoInstallationDirectories(t *testing.T) {
	s := newTestService(t, Options{})
	upstream := upstreamServer(t)
	if _, err := s.Probe(context.Background(), "agent", ProbeRequest{AppID: "gitlab", Config: Config{URL: upstream.URL}, Credentials: Credentials{Token: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.dataRoot); !os.IsNotExist(err) {
		t.Fatal("HTTP probe created installation directories")
	}
}
