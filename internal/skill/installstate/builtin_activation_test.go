package installstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func prepareActivationForMutation(t *testing.T) (string, string, Record) {
	t.Helper()
	root := t.TempDir()
	record := Record{Source: SourceBuiltin, Version: "new", SHA256: "new"}
	if err := Update(root, "pdf", &record, nil); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, ".builtin-staging", ".builtin-stage-test")
	if err := os.MkdirAll(stage+".previous", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage+".previous", "SKILL.md"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteBuiltinActivation(root, "pdf", stage, "new"); err != nil {
		t.Fatal(err)
	}
	return root, stage, record
}

func TestFailedExplicitMutationRestoresActivation(t *testing.T) {
	root, stage, record := prepareActivationForMutation(t)
	failure := errors.New("replacement failed")
	err := Update(root, "pdf", nil, func() error {
		if _, err := os.Stat(BuiltinActivationPath(root, "pdf") + ".cancelled"); err != nil {
			t.Fatal("activation not cancelled before mutation")
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
	records, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if records["pdf"].Version != record.Version {
		t.Fatal("failed operation changed origin")
	}
	if _, err := os.Stat(BuiltinActivationPath(root, "pdf")); err != nil {
		t.Fatal("failed operation lost activation journal")
	}
	if _, err := os.Stat(stage + ".previous"); err != nil {
		t.Fatal("failed operation lost backup")
	}
}

func TestCancelledActivationSurvivesInterruptedDeletion(t *testing.T) {
	root, stage, _ := prepareActivationForMutation(t)
	// Model the durable cancellation marker left by a process exit after deletion.
	_, _, err := cancelActivations(root, map[string]*Record{"pdf": nil})
	if err != nil {
		t.Fatal(err)
	}
	_, commit, err := RecoverBuiltinActivation(root, "pdf", Record{})
	if err != nil {
		t.Fatal(err)
	}
	if commit {
		t.Fatal("cancelled transaction adopted as builtin")
	}
	if _, err := os.Stat(filepath.Join(root, "pdf")); !os.IsNotExist(err) {
		t.Fatal("deleted files resurrected")
	}
	if _, err := os.Stat(stage + ".previous"); !os.IsNotExist(err) {
		t.Fatal("obsolete backup not cleaned")
	}
}
