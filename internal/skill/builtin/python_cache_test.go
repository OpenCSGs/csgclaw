package builtin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/skill/installstate"
)

func TestUsePythonHelperThenUpgrade(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to exercise the bundled helper")
	}
	for _, userEdit := range []bool{false, true} {
		name := "cache_only"
		if userEdit {
			name = "cache_and_source_edit"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := Sync(root); err != nil {
				t.Fatal(err)
			}
			records, err := installstate.Read(root)
			if err != nil {
				t.Fatal(err)
			}
			current := records["xlsx"]
			dst := filepath.Join(root, "xlsx")
			skillFile := filepath.Join(dst, "SKILL.md")
			currentContent, err := os.ReadFile(skillFile)
			if err != nil {
				t.Fatal(err)
			}
			oldContent := append(append([]byte(nil), currentContent...), []byte("\nPrevious bundled revision.\n")...)
			if err := os.WriteFile(skillFile, oldContent, 0644); err != nil {
				t.Fatal(err)
			}
			oldHash, err := installstate.BundleSHA256(dst)
			if err != nil {
				t.Fatal(err)
			}
			old := installstate.Record{Source: installstate.SourceBuiltin, Version: oldHash, SHA256: oldHash}
			if err := installstate.Update(root, "xlsx", &old, nil); err != nil {
				t.Fatal(err)
			}
			// Import the actual bundled module, generating its normal bytecode cache.
			// Importing defines functions only; it does not run LibreOffice or install dependencies.
			cmd := exec.Command(python, "-c", `import office.soffice; from pathlib import Path; assert list(Path('office/__pycache__').glob('soffice.*.pyc'))`)
			cmd.Dir = filepath.Join(dst, "scripts")
			for _, env := range os.Environ() {
				key, _, _ := strings.Cut(env, "=")
				switch key {
				case "PYTHONDONTWRITEBYTECODE", "PYTHONPYCACHEPREFIX", "PYTHONPATH", "PYTHONHOME":
					continue
				}
				cmd.Env = append(cmd.Env, env)
			}
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("import bundled office.soffice: %v\n%s", err, output)
			}
			helper := filepath.Join(dst, "scripts", "office", "soffice.py")
			var edited []byte
			if userEdit {
				original, err := os.ReadFile(helper)
				if err != nil {
					t.Fatal(err)
				}
				edited = append(original, []byte("\n# User's source modification.\n")...)
				if err := os.WriteFile(helper, edited, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := Sync(root); err != nil {
				t.Fatal(err)
			}
			records, err = installstate.Read(root)
			if err != nil {
				t.Fatal(err)
			}
			if userEdit {
				if records["xlsx"].Version != oldHash {
					t.Fatal("real user edit was overwritten")
				}
				actual, err := os.ReadFile(helper)
				if err != nil || string(actual) != string(edited) {
					t.Fatalf("user source edit not preserved: %v", err)
				}
			} else {
				if records["xlsx"].Version != current.Version {
					t.Fatalf("Python cache blocked update: got version %s, want %s", records["xlsx"].Version, current.Version)
				}
				actual, err := os.ReadFile(skillFile)
				if err != nil || string(actual) != string(currentContent) {
					t.Fatalf("current bundled revision not installed: %v", err)
				}
			}
		})
	}
}
