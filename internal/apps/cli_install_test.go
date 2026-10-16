package apps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	skillstate "csgclaw/internal/skill/state"
)

func TestCLIConnectAndMigrationImportPreinstalledPackageOffline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell executable fixture")
	}
	for _, action := range []string{"connect", "migration"} {
		t.Run(action, func(t *testing.T) {
			var requests atomic.Int32
			previous := http.DefaultTransport
			http.DefaultTransport = platformRoundTripper(func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				return nil, errors.New("network disabled")
			})
			t.Cleanup(func() { http.DefaultTransport = previous })
			seed := t.TempDir()
			t.Setenv("CSGCLAW_CONNECTOR_PACKAGES_DIR", seed)
			s, _, id := newCLIFixture(t)
			s.options.CLI.Install = nil
			spec := s.packages["feishu"].CLI.Install
			root := filepath.Join(seed, spec.Executable, spec.Version)
			binary := []byte("#!/bin/sh\nprintf 'lark-cli " + spec.Version + "\\n'\n")
			files := map[string][]byte{
				"bin/lark-cli":              binary,
				"skills/lark-test/SKILL.md": []byte("---\nname: lark-test\ndescription: Test connector skill\n---\nTest instructions\n"),
			}
			for name, data := range files {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0700); err != nil {
					t.Fatal(err)
				}
			}
			digest, err := skillstate.DirectoryDigest(filepath.Join(root, "skills"))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(binary)
			manifest, _ := json.Marshal(map[string]string{"version": spec.Version, "skills_commit": spec.Skills.Version, "executable_sha256": hex.EncodeToString(sum[:]), "skills_sha256": digest})
			if err := os.WriteFile(filepath.Join(root, "package.json"), manifest, 0600); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if action == "connect" {
				op := startCLITest(t, s, id, "connect", "offline-connect")
				if got := waitCLIOperation(t, s, id, op); got.Status != "completed" {
					t.Fatalf("operation=%+v", got)
				}
			} else {
				if err := s.PrepareCLIMigration(ctx, []LegacyWorker{{AgentID: "worker", ConfigFile: migrationInput(t)}}); err != nil {
					t.Fatal(err)
				}
				if err := s.RestoreCLIMigration(ctx); err != nil {
					t.Fatal(err)
				}
			}
			r, err := s.cliSnapshot(id)
			if err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(s.dataRoot, id, "package", spec.Version, "bin", "lark-cli")
			if !r.CLI.Ready || r.CLI.Executable != want || requests.Load() != 0 {
				t.Fatalf("ready=%v executable=%s requests=%d", r.CLI.Ready, r.CLI.Executable, requests.Load())
			}
			if _, err := os.Stat(filepath.Join(r.CLI.SkillsDir, "lark-test", "SKILL.md")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
