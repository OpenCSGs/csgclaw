package codexcli

import (
	"strings"
	"testing"
)

func TestWindowsBatchAppServerCommandLine(t *testing.T) {
	path := `C:\Users\Jane Doe\AppData\Roaming\npm & tools\codex.cmd`
	got, err := windowsBatchAppServerCommandLine(path)
	if err != nil {
		t.Fatalf("windowsBatchAppServerCommandLine() error = %v", err)
	}
	want := `/d /s /v:off /c ""C:\Users\Jane Doe\AppData\Roaming\npm & tools\codex.cmd" app-server --listen stdio://"`
	if got != want {
		t.Fatalf("windowsBatchAppServerCommandLine() = %q, want %q", got, want)
	}
}

func TestWindowsBatchAppServerCommandLineRejectsUnsafePath(t *testing.T) {
	for _, path := range []string{"", `C:\npm\%USER%\codex.cmd`, "C:\\npm\\codex.cmd\r\nwhoami"} {
		t.Run(strings.ReplaceAll(path, "\\", "_"), func(t *testing.T) {
			if _, err := windowsBatchAppServerCommandLine(path); err == nil {
				t.Fatalf("windowsBatchAppServerCommandLine(%q) error = nil, want error", path)
			}
		})
	}
}

func TestIsWindowsCommandShimPath(t *testing.T) {
	for _, path := range []string{`C:\npm\codex.cmd`, `C:\npm\CODEX.BAT`} {
		if !isWindowsCommandShimPath(path) {
			t.Fatalf("isWindowsCommandShimPath(%q) = false, want true", path)
		}
	}
	for _, path := range []string{`C:\npm\codex.exe`, `C:\npm\codex.ps1`, "/usr/bin/codex"} {
		if isWindowsCommandShimPath(path) {
			t.Fatalf("isWindowsCommandShimPath(%q) = true, want false", path)
		}
	}
}
