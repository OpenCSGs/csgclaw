package larkcli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestLegacySnapshotSurvivesGenerationCleanupAndRetry(t *testing.T) {
	root := t.TempDir()
	generation := filepath.Join(root, "generation-original")
	config := filepath.Join(generation, "config", WorkspaceName, ConfigFileName)
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"apps":[{"appId":"cli_existing"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(generation, "source-helper")
	if err := os.WriteFile(executable, []byte("test executable"), 0700); err != nil {
		t.Fatal(err)
	}
	current := agentruntime.ExtensionProjection{Root: filepath.Base(generation), Generation: 2, SourceRevision: "original", Environment: map[string]string{"LARKSUITE_CLI_CONFIG_DIR": filepath.Join(generation, "config"), "LARK_CHANNEL_HOME": filepath.Join(generation, "source")}}
	destination := filepath.Join(root, "migration", "saved")
	first, err := SaveLegacyProjection(current, destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(generation); err != nil {
		t.Fatal(err)
	}
	second, err := SaveLegacyProjection(current, destination)
	if err != nil || second.Directory != first.Directory {
		t.Fatalf("retry=%+v error=%v", second, err)
	}
	if !strings.HasPrefix(second.Projection.Environment["LARK_CHANNEL_HOME"], destination) {
		t.Fatal("source still references removed generation")
	}
	if _, err := os.Stat(filepath.Join(destination, "config", WorkspaceName, ConfigFileName)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(destination, "source-helper"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0100 == 0 {
		t.Fatal("helper execution permission was removed")
	}
}
func TestHistoricalAttachmentInstructionsUseCurrentChat(t *testing.T) {
	got := HistoricalAttachmentInstructions()
	if !strings.Contains(got, "current_chat_id") || strings.Contains(got, "LARK_CHANNEL_HOME") {
		t.Fatal("unexpected shared connector instructions")
	}
}
